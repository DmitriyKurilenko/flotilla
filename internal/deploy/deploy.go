// Package deploy is the deploy-pipeline state machine described in
// docs/ARCHITECTURE.md §5.
//
// Each step is fail-stop: the first error aborts the pipeline, no
// automatic rollback in v0.1. The `--dry` mode stops after StepEnv —
// no filesystem mutation, no docker calls.
//
// Note on lint input (see docs/ARCHITECTURE.md §5 step 2): lint
// validates the operator's authored compose.yml, NOT the merged
// autocert override. Linting flotilla's own generated override would
// make rule L008 (https_entrypoint vs manual labels) misfire on every
// auto-cert project. The override is known-good by construction; it is
// only relevant to the actual `compose up`.
package deploy

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DmitriyKurilenko/flotilla/internal/autocert"
	"github.com/DmitriyKurilenko/flotilla/internal/compose"
	"github.com/DmitriyKurilenko/flotilla/internal/contract"
	"github.com/DmitriyKurilenko/flotilla/internal/envcheck"
	"github.com/DmitriyKurilenko/flotilla/internal/project"
	"github.com/DmitriyKurilenko/flotilla/internal/state"
	"github.com/DmitriyKurilenko/flotilla/internal/traefik"
)

// Step identifies one pipeline stage. The numeric values mirror
// docs/ARCHITECTURE.md §5 (0..8).
type Step int

const (
	StepAutocert Step = iota
	StepParse
	StepLint
	StepEnv
	StepSymlinks
	StepComposeUp
	StepWaitRunning
	StepTraefikDiscover
	StepSmoke
)

// String returns the short name of the step, matching the table column
// in docs/ARCHITECTURE.md §5.
func (s Step) String() string {
	switch s {
	case StepAutocert:
		return "autocert"
	case StepParse:
		return "parse"
	case StepLint:
		return "lint"
	case StepEnv:
		return "env"
	case StepSymlinks:
		return "symlinks"
	case StepComposeUp:
		return "compose-up"
	case StepWaitRunning:
		return "wait-running"
	case StepTraefikDiscover:
		return "traefik-discover"
	case StepSmoke:
		return "smoke"
	default:
		return "?"
	}
}

// LastDryStep is the last step executed in `--dry` mode.
const LastDryStep = StepEnv

// Docker-touching seams. Production wiring is the real compose package;
// tests swap these to exercise the pipeline without a Docker daemon.
var (
	composeLoad = compose.Load
	composeUp   = compose.Up
	composePS   = compose.PS
)

// Tunable timeouts. Exposed as vars (not consts) so tests can shrink
// them; production uses the docs/ARCHITECTURE.md §5 values.
var (
	WaitRunningTimeout = 180 * time.Second
	TraefikTimeout     = 30 * time.Second
	SmokeRetries       = 12
	SmokeInterval      = 10 * time.Second
	pollInterval       = 3 * time.Second
)

// Options control the deploy pipeline.
type Options struct {
	// ProjectDir is the absolute path to the directory containing
	// project.yml.
	ProjectDir string
	// Dry stops the pipeline after StepEnv. No filesystem mutation,
	// no docker calls.
	Dry bool
	// Logger receives structured progress events. nil → discard.
	Logger *slog.Logger
	// TraefikAddr overrides the Traefik API address (tests). Empty →
	// traefik.DefaultAddr.
	TraefikAddr string
}

// Outcome describes what happened. Run always returns a non-nil
// *Outcome; callers inspect LastStep and Err.
type Outcome struct {
	LastStep Step
	Err      error
	Dry      bool
}

func done(step Step, err error, dry bool) *Outcome {
	return &Outcome{LastStep: step, Err: err, Dry: dry}
}

// Run executes the pipeline. See package doc and docs/ARCHITECTURE.md §5.
func Run(ctx context.Context, opts Options) *Outcome {
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	pd := opts.ProjectDir
	projPath := filepath.Join(pd, "project.yml")
	composePath := filepath.Join(pd, "compose.yml")
	envPath := filepath.Join(pd, ".env")
	envExample := filepath.Join(pd, ".env.example")

	// ── Step: parse ────────────────────────────────────────────────
	proj, err := project.Load(projPath)
	if err != nil {
		return done(StepParse, err, opts.Dry)
	}
	log.Info("parsed project.yml", "name", proj.Name, "domain", proj.Domain)

	// ── Step: template substitution ────────────────────────────────
	// Replace ${domain} placeholder in compose.yml with the actual domain.
	// This lets manual-labels projects reference the project's domain
	// without duplicating it in .env.
	rawCompose, err := os.ReadFile(composePath)
	if err != nil {
		return done(StepParse, fmt.Errorf("read compose.yml: %w", err), opts.Dry)
	}
	renderedCompose := strings.ReplaceAll(string(rawCompose), "${domain}", proj.Domain)
	if err := state.EnsureDir(pd); err != nil {
		return done(StepParse, err, opts.Dry)
	}
	renderedComposePath := filepath.Join(state.Dir(pd), "compose.yml")
	if err := os.WriteFile(renderedComposePath, []byte(renderedCompose), 0o644); err != nil {
		return done(StepParse, fmt.Errorf("write rendered compose: %w", err), opts.Dry)
	}
	if !opts.Dry {
		log.Info("rendered compose.yml", "path", renderedComposePath)
	}

	// ── Step: autocert ─────────────────────────────────────────────
	var overridePath string
	if proj.HTTPSEntrypoint != nil {
		compForPort, cerr := composeLoad(ctx, renderedComposePath, envPath)
		if cerr != nil {
			return done(StepAutocert, fmt.Errorf("autocert needs a parseable compose.yml: %w", cerr), opts.Dry)
		}
		overrideYAML, rerr := autocert.Render(proj, compForPort)
		if rerr != nil {
			return done(StepAutocert, rerr, opts.Dry)
		}
		if !opts.Dry {
			if err := state.EnsureDir(pd); err != nil {
				return done(StepAutocert, err, false)
			}
			overridePath = filepath.Join(state.Dir(pd), "compose.override.yml")
			if err := os.WriteFile(overridePath, overrideYAML, 0o644); err != nil {
				return done(StepAutocert, fmt.Errorf("write override: %w", err), false)
			}
			log.Info("rendered autocert override", "path", overridePath)
		} else {
			log.Info("autocert override rendered in-memory (dry run)")
		}
	}

	// ── Step: lint ─────────────────────────────────────────────────
	// Lint validates the rendered compose.yml (with ${domain} substituted),
	// not the merged override.
	authoredCompose, _ := composeLoad(ctx, renderedComposePath, envPath)
	lintCtx := contract.Context{
		Project:         proj,
		Compose:         authoredCompose, // may be nil → L001 fails
		RawCompose:      []byte(renderedCompose),
		DockerfilePaths: findDockerfiles(pd),
		EnvPath:         envPath,
		EnvExamplePath:  envExample,
	}
	results := contract.Run(lintCtx)
	for _, r := range results {
		switch r.Verdict {
		case contract.Fail:
			log.Error("lint", "rule", r.Rule.ID(), "verdict", "fail", "msg", r.Message)
		case contract.Warn:
			log.Warn("lint", "rule", r.Rule.ID(), "verdict", "warn", "msg", r.Message)
		default:
			log.Info("lint", "rule", r.Rule.ID(), "verdict", "pass")
		}
	}
	if contract.HasFailures(results) {
		return done(StepLint, fmt.Errorf("lint failed: %s", failSummary(results)), opts.Dry)
	}

	// ── Step: env ──────────────────────────────────────────────────
	refs, err := envcheck.ScanCompose([]byte(renderedCompose))
	if err != nil {
		return done(StepEnv, fmt.Errorf("scan compose env refs: %w", err), opts.Dry)
	}
	envRes, err := envcheck.Check(refs, envPath)
	if err != nil {
		return done(StepEnv, err, opts.Dry)
	}
	if envRes.HasFailures() {
		return done(StepEnv, fmt.Errorf("env check failed: missing=%v empty=%v",
			envRes.Missing, envRes.Empty), opts.Dry)
	}
	for _, u := range envRes.Unused {
		log.Warn("env", "unused_in_compose", u)
	}

	if opts.Dry {
		log.Info("dry run complete — validation passed, no changes made")
		return done(LastDryStep, nil, true)
	}

	// ── Step: symlinks (ensure .flotilla workdir) ──────────────────
	// In flotilla's model the project directory IS the deploy target;
	// the only layout flotilla owns here is the .flotilla/ workdir
	// (state.json + compose.override.yml). The repo-symlink topology
	// from §5 step 4 applies only when a separate checkout feeds a
	// deploy target, which v0.1 discovery does not create.
	if err := state.EnsureDir(pd); err != nil {
		return done(StepSymlinks, err, false)
	}

	// ── Step: compose up ───────────────────────────────────────────
	composeFiles := []string{renderedComposePath}
	if overridePath != "" {
		composeFiles = append(composeFiles, overridePath)
	}
	var upBuf bytes.Buffer
	if err := composeUp(ctx, pd, composeFiles, envPath, &upBuf, &upBuf); err != nil {
		return done(StepComposeUp, fmt.Errorf("%w\n%s", err, tail(upBuf.String(), 40)), false)
	}
	log.Info("compose up complete")

	// ── Step: wait running ─────────────────────────────────────────
	if err := waitRunning(ctx, pd, composeFiles, WaitRunningTimeout); err != nil {
		return done(StepWaitRunning, err, false)
	}
	log.Info("all containers running")

	// ── Step: traefik discover ─────────────────────────────────────
	addr := opts.TraefikAddr
	if addr == "" {
		addr = traefik.DefaultAddr
	}
	if err := waitTraefik(ctx, traefik.New(addr), proj.Domain, TraefikTimeout); err != nil {
		return done(StepTraefikDiscover, err, false)
	}
	log.Info("traefik registered router", "domain", proj.Domain)

	// ── Step: smoke ────────────────────────────────────────────────
	if err := smoke(ctx, proj.Domain, SmokeRetries, SmokeInterval); err != nil {
		return done(StepSmoke, err, false)
	}
	log.Info("smoke ok", "url", "https://"+proj.Domain+"/")

	// Record state (best-effort; a failed write is not a deploy failure).
	if werr := state.Write(pd, &state.State{
		SHA:     gitSHA(pd),
		Summary: "deploy ok",
	}); werr != nil {
		log.Warn("could not write .flotilla/state.json", "err", werr)
	}

	return done(StepSmoke, nil, false)
}

// ─── helpers ─────────────────────────────────────────────────────────
//nolint:gocyclo // the pipeline is intentionally a linear sequence of steps

func findDockerfiles(dir string) []string {
	var out []string
	for _, pat := range []string{"Dockerfile", "Dockerfile.*", "*.Dockerfile"} {
		ms, _ := filepath.Glob(filepath.Join(dir, pat))
		out = append(out, ms...)
	}
	return out
}

func failSummary(results []contract.Result) string {
	var parts []string
	for _, r := range results {
		if r.Verdict == contract.Fail {
			parts = append(parts, fmt.Sprintf("%s (%s)", r.Rule.ID(), r.Message))
		}
	}
	return strings.Join(parts, "; ")
}

func tail(s string, lines int) string {
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(parts) <= lines {
		return s
	}
	return strings.Join(parts[len(parts)-lines:], "\n")
}

// waitRunning polls compose ps until every service is running and any
// declared healthcheck reports healthy, or the timeout elapses.
func waitRunning(ctx context.Context, projectDir string, composeFiles []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		statuses, err := composePS(ctx, projectDir, composeFiles)
		if err == nil && allReady(statuses) {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("containers not ready within %s: %w", timeout, err)
			}
			return fmt.Errorf("containers not ready within %s: %s", timeout, summarize(mustStatuses(ctx, projectDir, composeFiles)))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func mustStatuses(ctx context.Context, dir string, files []string) []compose.ContainerStatus {
	s, _ := composePS(ctx, dir, files)
	return s
}

func allReady(statuses []compose.ContainerStatus) bool {
	if len(statuses) == 0 {
		return false
	}
	for _, s := range statuses {
		if s.State != "running" {
			return false
		}
		if s.Health != "" && s.Health != "healthy" {
			return false
		}
	}
	return true
}

func summarize(statuses []compose.ContainerStatus) string {
	var parts []string
	for _, s := range statuses {
		h := s.Health
		if h == "" {
			h = "no-healthcheck"
		}
		parts = append(parts, fmt.Sprintf("%s=%s/%s", s.Service, s.State, h))
	}
	return strings.Join(parts, " ")
}

// waitTraefik polls the Traefik API until a router for the domain
// appears, or the timeout elapses. A common cause of timeout here is
// «Traefik isn't running» — see docs/ARCHITECTURE.md §5 step 7.
func waitTraefik(ctx context.Context, tc *traefik.Client, domain string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		routers, err := tc.FindByHost(ctx, domain)
		if err == nil && len(routers) > 0 {
			return nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("no Traefik router for Host(`%s`) within %s (Traefik API error: %w)", domain, timeout, lastErr)
			}
			return fmt.Errorf("no Traefik router for Host(`%s`) within %s — is the container healthy? `docker logs flotilla-traefik`", domain, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// smoke issues GET https://<domain>/ and accepts any status < 500.
// Retries `retries` times with `interval` between attempts. TLS
// verification is skipped — the cert may still be issuing and that is
// the acme probe's concern, not the smoke test's.
func smoke(ctx context.Context, domain string, retries int, interval time.Duration) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // post-deploy reachability probe only
		},
	}
	url := "https://" + domain + "/"
	var lastErr error
	for attempt := 1; attempt <= retries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			code := resp.StatusCode
			resp.Body.Close()
			if code < 500 {
				return nil
			}
			lastErr = fmt.Errorf("status %d", code)
		} else {
			lastErr = err
		}
		if attempt < retries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
		}
	}
	return fmt.Errorf("smoke %s failed after %d attempts: %w", url, retries, lastErr)
}

func gitSHA(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
