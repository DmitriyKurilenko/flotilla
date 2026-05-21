// Package envcheck scans compose YAML for ${VAR} references and
// verifies they're satisfied by .env on disk.
//
// This is the implementation behind deploy pipeline step 3 (`env`) and
// behind half of lint rule L007. See docs/ARCHITECTURE.md §3.2 («What
// is derived from compose.yml»).
package envcheck

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Reference is one ${VAR} or ${VAR:-default} found in compose YAML.
type Reference struct {
	Name       string
	HasDefault bool   // true for ${VAR:-default} form
	Default    string // "" when HasDefault is false
}

// Result is the outcome of comparing references against a .env file.
type Result struct {
	Missing []string // required references not present in .env
	Empty   []string // required references present but empty
	Unused  []string // .env entries that are never referenced (warn-only)
}

// HasFailures reports whether there are missing or empty required vars.
// Unused vars are informational and do not fail deploy.
func (r *Result) HasFailures() bool {
	if r == nil {
		return false
	}
	return len(r.Missing) > 0 || len(r.Empty) > 0
}

// ─── ScanCompose ──────────────────────────────────────────────────

// referenceRE matches both ${VAR} and ${VAR:-default} (and the rarer
// ${VAR:?error} / ${VAR-default} variants compose supports). We capture
// the name, the separator (if any), and the default.
//
//	${FOO}                    → name=FOO, sep="", default=""
//	${FOO:-bar}               → name=FOO, sep=":-", default="bar"
//	${FOO-bar}                → name=FOO, sep="-",  default="bar"
//	${FOO:?error message}     → name=FOO, sep=":?", default="error message"
//	${FOO?error message}      → name=FOO, sep="?",  default="error message"
//
// flotilla treats any of the default/error forms as «has a default» for
// the purpose of «is this var required?», because in all those forms
// compose can resolve a value without the operator setting the var
// (either it falls back to the default, or compose hard-errors with the
// custom message — either way it's not a silent «empty value» problem
// that flotilla should pre-check).
var referenceRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:?[?-])?([^}]*)\}`)

// ScanCompose finds every ${VAR} reference in the raw compose bytes.
// References are de-duplicated by Name; if the same VAR appears with
// both a default and without, the «no default» wins (it's required).
func ScanCompose(rawCompose []byte) ([]Reference, error) {
	if len(rawCompose) == 0 {
		return nil, nil
	}
	matches := referenceRE.FindAllSubmatch(rawCompose, -1)
	if len(matches) == 0 {
		return nil, nil
	}

	dedup := make(map[string]Reference, len(matches))
	for _, m := range matches {
		name := string(m[1])
		sep := string(m[2])
		def := string(m[3])
		hasDefault := sep != ""

		// First sighting → record. Subsequent sightings: prefer "no
		// default" if any reference of this name is unguarded.
		prev, seen := dedup[name]
		if !seen {
			dedup[name] = Reference{Name: name, HasDefault: hasDefault, Default: def}
			continue
		}
		if !hasDefault {
			// downgrade to required regardless of prior default.
			dedup[name] = Reference{Name: name, HasDefault: false}
		}
		_ = prev
	}

	out := make([]Reference, 0, len(dedup))
	for _, r := range dedup {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ─── LoadEnv ──────────────────────────────────────────────────────

// LoadEnv parses a dotenv-style file into a map. A missing file returns
// an empty map without error; the caller decides if that's fatal.
//
// Syntax accepted (matching `docker compose --env-file` behavior):
//
//   - Blank lines: skipped.
//   - Lines starting with # (after whitespace): comments, skipped.
//   - `KEY=value`: literal value (no shell expansion).
//   - `KEY="value"` / `KEY='value'`: quoted value, surrounding quotes
//     stripped (one pair only — no escape processing inside).
//   - `KEY=` (empty RHS): key with empty value.
//
// This is a strict, predictable subset. Compose's actual rules are
// looser; we match the strict subset that is unambiguous and well-
// documented to avoid surprise.
func LoadEnv(path string) (map[string]string, error) {
	if path == "" {
		return map[string]string{}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	out := make(map[string]string)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 1 {
			return nil, fmt.Errorf("%s:%d: not a KEY=VALUE line: %q", path, lineNo, line)
		}
		key := strings.TrimSpace(line[:eq])
		if !validKey(key) {
			return nil, fmt.Errorf("%s:%d: invalid key %q", path, lineNo, key)
		}
		val := strings.TrimSpace(line[eq+1:])
		val = unquote(val)
		out[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return out, nil
}

var keyRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validKey(s string) bool {
	return keyRE.MatchString(s)
}

func unquote(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// ─── Check ────────────────────────────────────────────────────────

// Check compares the references against the environment loaded from
// envPath and reports the differences.
//
// A reference is «required» if it has no default in compose
// (Reference.HasDefault == false). Required references that are absent
// from .env land in Missing; required references present but empty
// land in Empty.
//
// References *with* defaults are not validated at all — compose will
// fill in the default and the deploy succeeds.
//
// Vars in .env that are never referenced by compose are reported in
// Unused (warn-only; deploy proceeds).
func Check(refs []Reference, envPath string) (*Result, error) {
	env, err := LoadEnv(envPath)
	if err != nil {
		return nil, err
	}

	result := &Result{}
	referenced := make(map[string]struct{}, len(refs))
	for _, r := range refs {
		referenced[r.Name] = struct{}{}
		if r.HasDefault {
			continue
		}
		val, present := env[r.Name]
		if !present {
			result.Missing = append(result.Missing, r.Name)
			continue
		}
		if val == "" {
			result.Empty = append(result.Empty, r.Name)
		}
	}
	for k := range env {
		if _, ok := referenced[k]; !ok {
			result.Unused = append(result.Unused, k)
		}
	}
	sort.Strings(result.Missing)
	sort.Strings(result.Empty)
	sort.Strings(result.Unused)
	return result, nil
}
