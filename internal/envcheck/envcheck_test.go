package envcheck

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// ─── ScanCompose ──────────────────────────────────────────────────

func TestScanCompose_BasicForms(t *testing.T) {
	compose := []byte(`
services:
  web:
    image: registry/${IMAGE_NAME}:${IMAGE_TAG:-latest}
    environment:
      - SECRET_KEY=${SECRET_KEY}
      - DB_PASSWORD=${DB_PASSWORD}
      - REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379/0
      - OPTIONAL=${OPTIONAL_VAR:-fallback-value}
      - DASH_DEFAULT=${SOME_VAR-just-dash}
`)
	refs, err := ScanCompose(compose)
	if err != nil {
		t.Fatalf("ScanCompose: %v", err)
	}
	got := refMap(refs)

	wantRequired := []string{"IMAGE_NAME", "SECRET_KEY", "DB_PASSWORD", "REDIS_PASSWORD"}
	wantOptional := []string{"IMAGE_TAG", "OPTIONAL_VAR", "SOME_VAR"}

	for _, name := range wantRequired {
		r, ok := got[name]
		if !ok {
			t.Errorf("missing reference %q", name)
			continue
		}
		if r.HasDefault {
			t.Errorf("%s should be required (no default), got HasDefault=true", name)
		}
	}
	for _, name := range wantOptional {
		r, ok := got[name]
		if !ok {
			t.Errorf("missing reference %q", name)
			continue
		}
		if !r.HasDefault {
			t.Errorf("%s should be optional (has default), got HasDefault=false", name)
		}
	}
}

func TestScanCompose_DedupRequiredWinsOverDefault(t *testing.T) {
	// If the same VAR appears once with a default and once without, the
	// unguarded reference makes it required.
	compose := []byte(`
services:
  web:
    image: ${TAG:-latest}
    environment:
      - SECOND=${TAG}
`)
	refs, err := ScanCompose(compose)
	if err != nil {
		t.Fatalf("ScanCompose: %v", err)
	}
	r := refMap(refs)["TAG"]
	if r.HasDefault {
		t.Errorf("expected TAG to be required (one unguarded reference), got HasDefault=true")
	}
}

func TestScanCompose_Empty(t *testing.T) {
	refs, err := ScanCompose(nil)
	if err != nil || refs != nil {
		t.Errorf("nil input → (%v, %v), want (nil, nil)", refs, err)
	}
	refs, err = ScanCompose([]byte(`services:\n  web:\n    image: nginx\n`))
	if err != nil {
		t.Fatalf("no refs input: %v", err)
	}
	if refs != nil {
		t.Errorf("no refs → got %v, want nil", refs)
	}
}

func TestScanCompose_IgnoreNonVariableDollarBraces(t *testing.T) {
	// Things like ${0} shell positional args would never appear in
	// compose; we don't have to handle them, but ensure the regex
	// doesn't crash on weird inputs.
	compose := []byte(`
services:
  web:
    command: echo "${FOO} and $$LITERAL_DOLLAR and \${ESCAPED}"
    environment:
      - REAL=${REAL_VAR}
`)
	refs, err := ScanCompose(compose)
	if err != nil {
		t.Fatalf("ScanCompose: %v", err)
	}
	if _, ok := refMap(refs)["REAL_VAR"]; !ok {
		t.Errorf("REAL_VAR should be captured")
	}
	if _, ok := refMap(refs)["FOO"]; !ok {
		t.Errorf("FOO should be captured")
	}
}

// ─── LoadEnv ──────────────────────────────────────────────────────

func TestLoadEnv_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	body := `
# top-level comment
KEY1=value1
KEY2="quoted value"
KEY3='single quoted'
KEY4=

# another comment
KEY5=value with spaces and = signs
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadEnv(path)
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	want := map[string]string{
		"KEY1": "value1",
		"KEY2": "quoted value",
		"KEY3": "single quoted",
		"KEY4": "",
		"KEY5": "value with spaces and = signs",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadEnv = %#v\nwant %#v", got, want)
	}
}

func TestLoadEnv_MissingFile(t *testing.T) {
	got, err := LoadEnv(filepath.Join(t.TempDir(), "nope.env"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("missing file → got %v, want empty map", got)
	}
}

func TestLoadEnv_EmptyPath(t *testing.T) {
	got, err := LoadEnv("")
	if err != nil {
		t.Fatalf("empty path: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty path → got %v, want empty map", got)
	}
}

func TestLoadEnv_BadLines(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		body string
	}{
		{"no equals", "KEY1\nKEY2=ok\n"},
		{"bad key with dash", "BAD-KEY=value\n"},
		{"empty key", "=value\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name+".env")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadEnv(path); err == nil {
				t.Errorf("expected error for %q", tt.body)
			}
		})
	}
}

// ─── Check ────────────────────────────────────────────────────────

func TestCheck_AllRequiredPresent(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	_ = os.WriteFile(envPath, []byte("DB_PASSWORD=secret\nSECRET_KEY=abc\n"), 0o644)

	refs := []Reference{
		{Name: "DB_PASSWORD"},
		{Name: "SECRET_KEY"},
	}
	res, err := Check(refs, envPath)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.HasFailures() {
		t.Errorf("expected no failures, got %+v", res)
	}
}

func TestCheck_MissingRequired(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	_ = os.WriteFile(envPath, []byte("OTHER=x\n"), 0o644)

	refs := []Reference{{Name: "DB_PASSWORD"}, {Name: "SECRET_KEY"}}
	res, err := Check(refs, envPath)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !res.HasFailures() {
		t.Errorf("expected failures, got %+v", res)
	}
	got := append([]string{}, res.Missing...)
	sort.Strings(got)
	want := []string{"DB_PASSWORD", "SECRET_KEY"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Missing = %v, want %v", got, want)
	}
	if !containsString(res.Unused, "OTHER") {
		t.Errorf("Unused should contain OTHER, got %v", res.Unused)
	}
}

func TestCheck_EmptyRequired(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	_ = os.WriteFile(envPath, []byte("DB_PASSWORD=\nSECRET_KEY=set\n"), 0o644)

	refs := []Reference{{Name: "DB_PASSWORD"}, {Name: "SECRET_KEY"}}
	res, err := Check(refs, envPath)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !res.HasFailures() {
		t.Errorf("expected failures")
	}
	if !containsString(res.Empty, "DB_PASSWORD") {
		t.Errorf("Empty should contain DB_PASSWORD, got %v", res.Empty)
	}
}

func TestCheck_DefaultsAreNotRequired(t *testing.T) {
	// Reference has a default → not validated.
	refs := []Reference{
		{Name: "REQUIRED_VAR"},
		{Name: "OPTIONAL_VAR", HasDefault: true, Default: "fallback"},
	}
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	_ = os.WriteFile(envPath, []byte("REQUIRED_VAR=set\n"), 0o644)

	res, err := Check(refs, envPath)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.HasFailures() {
		t.Errorf("optional vars should not cause failures, got %+v", res)
	}
}

// ─── helpers ──────────────────────────────────────────────────────

func refMap(refs []Reference) map[string]Reference {
	m := make(map[string]Reference, len(refs))
	for _, r := range refs {
		m[r.Name] = r
	}
	return m
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
