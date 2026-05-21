// Package state reads and writes .flotilla/state.json — the only
// persistent state flotilla keeps for a project. See
// docs/ARCHITECTURE.md §8.3.
//
// The file records the last successful deploy: git sha, timestamp, and
// a one-line summary. It is non-critical (regenerated on next deploy)
// and lost on disk wipe by design.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DirName is the per-project workdir flotilla owns.
const DirName = ".flotilla"

// FileName is the state file within DirName.
const FileName = "state.json"

// State is the JSON shape of .flotilla/state.json.
type State struct {
	SHA        string `json:"sha"`
	DeployedAt string `json:"deployed_at"` // RFC3339
	Summary    string `json:"summary"`
}

// Dir returns the absolute path to projectDir/.flotilla.
func Dir(projectDir string) string {
	return filepath.Join(projectDir, DirName)
}

// Path returns the absolute path to projectDir/.flotilla/state.json.
func Path(projectDir string) string {
	return filepath.Join(Dir(projectDir), FileName)
}

// EnsureDir creates projectDir/.flotilla if it does not exist.
func EnsureDir(projectDir string) error {
	d := Dir(projectDir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", d, err)
	}
	return nil
}

// Read loads the state file. A missing file returns (nil, nil) — that
// is the normal «never deployed» case, not an error.
func Read(projectDir string) (*State, error) {
	data, err := os.ReadFile(Path(projectDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state.json: %w", err)
	}
	return &s, nil
}

// Write atomically writes the state file, creating .flotilla/ if
// needed. DeployedAt is set to now (RFC3339) if empty.
func Write(projectDir string, s *State) error {
	if s == nil {
		return errors.New("state.Write: nil state")
	}
	if s.DeployedAt == "" {
		s.DeployedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := EnsureDir(projectDir); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	data = append(data, '\n')

	final := Path(projectDir)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp state: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit state: %w", err)
	}
	return nil
}
