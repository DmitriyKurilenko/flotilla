//go:build integration

// Package integration exercises flotilla against a real Docker daemon.
//
// These tests are gated behind the `integration` build tag and are NOT
// part of `go test ./...`. Run them explicitly on a host (or CI job)
// that has Docker:
//
//	go test -tags=integration ./tests/integration/...
//
// What is covered here (reliable, no external dependencies):
//
//   - `flotilla deploy --dry` runs the full validation pipeline
//     (steps 0-3) against a real project and exits 0.
//   - `flotilla deploy` (no Traefik on the host) drives real
//     `docker compose up` + `wait-running` and then fails
//     deterministically at the traefik-discover step. This proves the
//     docker-touching steps (5-6) work end to end.
//
// What is intentionally NOT covered here: a full Traefik + Let's
// Encrypt round trip. Real ACME needs a public domain and DNS; that
// belongs in a manual/staging job, not unit-or-integration CI. See
// docs/ARCHITECTURE.md §11.
package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not in PATH; skipping integration test")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not responding; skipping integration test")
	}
}

// buildFlotilla compiles the CLI once and returns the binary path.
func buildFlotilla(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "flotilla")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/flotilla")
	cmd.Dir = repoRoot
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build flotilla: %v\n%s", err, out.String())
	}
	return bin
}

// writeProject lays out a project dir with an nginx service and the
// given project.yml body.
func writeProject(t *testing.T, projectYML string) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "project.yml"), projectYML)
	mustWrite(t, filepath.Join(dir, "compose.yml"), `services:
  web:
    image: nginx:1.27-alpine
    expose:
      - "80"
`)
	return dir
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, bin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return out.String(), code
}

func TestDeployDry_RealPipeline(t *testing.T) {
	requireDocker(t)
	bin := buildFlotilla(t)

	dir := writeProject(t, `version: 1
name: flotilla-it-dry
domain: dry.flotilla.test
path: `+"/opt/flotilla-it-dry"+`
https_entrypoint: web
`)

	out, code := run(t, bin, "deploy", dir, "--dry")
	if code != 0 {
		t.Fatalf("deploy --dry exit %d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, "dry run passed") {
		t.Errorf("expected dry-run success message, got:\n%s", out)
	}
	// Dry must not have written the override.
	if _, err := os.Stat(filepath.Join(dir, ".flotilla", "compose.override.yml")); !os.IsNotExist(err) {
		t.Errorf("dry run must not write compose.override.yml")
	}
}

func TestDeploy_RealComposeUp_FailsAtTraefikDiscover(t *testing.T) {
	requireDocker(t)
	bin := buildFlotilla(t)

	name := "flotilla-it-" + time.Now().Format("150405")
	dir := writeProject(t, `version: 1
name: `+name+`
domain: nonexistent.flotilla.test
path: /opt/`+name+`
https_entrypoint: web
`)

	// Ensure cleanup of whatever compose brought up.
	t.Cleanup(func() {
		c := exec.Command("docker", "compose",
			"-f", filepath.Join(dir, "compose.yml"),
			"-f", filepath.Join(dir, ".flotilla", "compose.override.yml"),
			"down", "-v", "--remove-orphans")
		c.Dir = dir
		_ = c.Run()
	})

	out, code := run(t, bin, "deploy", dir)
	// No Traefik on this host → the pipeline must reach compose-up +
	// wait-running successfully and then fail at traefik-discover.
	if code == 0 {
		t.Fatalf("expected non-zero exit (no Traefik present)\n%s", out)
	}
	if !strings.Contains(out, "traefik-discover") {
		t.Errorf("expected failure at traefik-discover step, got:\n%s", out)
	}
	// Sanity: the override WAS written (non-dry path).
	if _, err := os.Stat(filepath.Join(dir, ".flotilla", "compose.override.yml")); err != nil {
		t.Errorf("non-dry deploy should have written compose.override.yml: %v", err)
	}
}
