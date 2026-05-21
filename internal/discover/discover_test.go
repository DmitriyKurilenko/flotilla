package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProject(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "version: 1\nname: " + name + "\ndomain: " + name + ".example.com\npath: " + dir + "\n"
	if err := os.WriteFile(filepath.Join(dir, "project.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover_SortsByName(t *testing.T) {
	root := t.TempDir()
	writeProject(t, filepath.Join(root, "z-app"), "zebra")
	writeProject(t, filepath.Join(root, "a-app"), "alpha")
	writeProject(t, filepath.Join(root, "m-app"), "mike")

	got, err := Discover([]string{filepath.Join(root, "*", "project.yml")})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	wantOrder := []string{"alpha", "mike", "zebra"}
	for i, f := range got {
		if f.Name != wantOrder[i] {
			t.Errorf("position %d: Name = %q, want %q", i, f.Name, wantOrder[i])
		}
		if filepath.Base(f.Path) != "project.yml" {
			t.Errorf("Path should end in project.yml, got %q", f.Path)
		}
		if f.Dir != filepath.Dir(f.Path) {
			t.Errorf("Dir = %q, want %q", f.Dir, filepath.Dir(f.Path))
		}
	}
}

func TestDiscover_SkipsInvalidProject(t *testing.T) {
	root := t.TempDir()
	writeProject(t, filepath.Join(root, "good"), "good")

	bad := filepath.Join(root, "bad")
	_ = os.MkdirAll(bad, 0o755)
	_ = os.WriteFile(filepath.Join(bad, "project.yml"), []byte("not: a: valid: project"), 0o644)

	got, err := Discover([]string{filepath.Join(root, "*", "project.yml")})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 || got[0].Name != "good" {
		t.Errorf("expected only the good project, got %+v", got)
	}
}

func TestDiscover_NoMatches(t *testing.T) {
	got, err := Discover([]string{filepath.Join(t.TempDir(), "*", "project.yml")})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %+v", got)
	}
}

func TestDiscover_DedupsAcrossGlobs(t *testing.T) {
	root := t.TempDir()
	writeProject(t, filepath.Join(root, "app"), "app")
	glob := filepath.Join(root, "*", "project.yml")

	got, err := Discover([]string{glob, glob}) // same glob twice
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected dedup to 1, got %d", len(got))
	}
}

func TestDiscover_BadPattern(t *testing.T) {
	if _, err := Discover([]string{"[invalid"}); err == nil {
		t.Fatal("expected ErrBadPattern")
	}
}
