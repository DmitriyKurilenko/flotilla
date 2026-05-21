package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadMissingReturnsNilNil(t *testing.T) {
	dir := t.TempDir()
	s, err := Read(dir)
	if err != nil {
		t.Fatalf("Read missing: unexpected error %v", err)
	}
	if s != nil {
		t.Errorf("Read missing: got %+v, want nil", s)
	}
}

func TestWriteThenRead(t *testing.T) {
	dir := t.TempDir()
	in := &State{SHA: "abc123", Summary: "deploy ok"}
	if err := Write(dir, in); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if in.DeployedAt == "" {
		t.Error("Write should set DeployedAt when empty")
	}

	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got == nil {
		t.Fatal("Read returned nil after Write")
	}
	if got.SHA != "abc123" || got.Summary != "deploy ok" || got.DeployedAt != in.DeployedAt {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, in)
	}

	if _, err := os.Stat(filepath.Join(dir, DirName, FileName)); err != nil {
		t.Errorf("state file not at expected path: %v", err)
	}
}

func TestWriteNil(t *testing.T) {
	if err := Write(t.TempDir(), nil); err == nil {
		t.Fatal("Write(nil) should error")
	}
}

func TestReadCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(dir), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err == nil {
		t.Fatal("Read corrupt json should error")
	}
}
