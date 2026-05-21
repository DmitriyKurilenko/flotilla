package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRun_NoArgs_UsageExit2(t *testing.T) {
	if code := run(context.Background(), nil); code != 2 {
		t.Errorf("no args → exit %d, want 2", code)
	}
}

func TestRun_Help(t *testing.T) {
	if code := run(context.Background(), []string{"--help"}); code != 0 {
		t.Errorf("--help → exit %d, want 0", code)
	}
	if code := run(context.Background(), []string{"help"}); code != 0 {
		t.Errorf("help → exit %d, want 0", code)
	}
}

func TestRun_Version(t *testing.T) {
	if code := run(context.Background(), []string{"--version"}); code != 0 {
		t.Errorf("--version → exit %d, want 0", code)
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	if code := run(context.Background(), []string{"frobnicate"}); code != 2 {
		t.Errorf("unknown command → exit %d, want 2", code)
	}
}

func TestStatus_MissingProject_Exit1(t *testing.T) {
	dir := t.TempDir() // no project.yml
	code := cmdStatus(context.Background(), []string{dir, "--quiet"})
	if code != 1 {
		t.Errorf("status on dir without project.yml → exit %d, want 1", code)
	}
}

func TestDeploy_ParseFailure_Exit1(t *testing.T) {
	dir := t.TempDir() // no project.yml → parse step fails
	code := cmdDeploy(context.Background(), []string{dir, "--dry", "--quiet"})
	if code != 1 {
		t.Errorf("deploy --dry on dir without project.yml → exit %d, want 1", code)
	}
}

func TestDeploy_AllWithPathArg_Exit2(t *testing.T) {
	code := cmdDeploy(context.Background(), []string{"/somewhere", "--all"})
	if code != 2 {
		t.Errorf("--all with a path → exit %d, want 2", code)
	}
}

func TestResolveTargets_DefaultsToCWD(t *testing.T) {
	got, err := resolveTargets(nil, false)
	if err != nil {
		t.Fatalf("resolveTargets: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 target, got %v", got)
	}
	if !filepath.IsAbs(got[0]) {
		t.Errorf("target should be absolute, got %q", got[0])
	}
}

func TestResolveTargets_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveTargets([]string{dir}, false)
	if err != nil {
		t.Fatalf("resolveTargets: %v", err)
	}
	want, _ := filepath.Abs(dir)
	if len(got) != 1 || got[0] != want {
		t.Errorf("resolveTargets(%q) = %v, want [%q]", dir, got, want)
	}
}

func TestResolveTargets_TooManyPaths(t *testing.T) {
	if _, err := resolveTargets([]string{"a", "b"}, false); err == nil {
		t.Error("expected error for >1 path")
	}
}

func TestResolveTargets_AllWithPath(t *testing.T) {
	if _, err := resolveTargets([]string{"x"}, true); err == nil {
		t.Error("expected error: --all takes no path")
	}
}

// Ensure --version path doesn't depend on ldflags being set.
func TestPrintVersion(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "v")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	printVersion(f)
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 256)
	n, _ := f.Read(buf)
	if n == 0 {
		t.Error("printVersion wrote nothing")
	}
}
