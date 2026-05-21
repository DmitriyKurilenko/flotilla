package project_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DmitriyKurilenko/flotilla/internal/project"
)

// ─── RouterName / String (pure-Go helpers) ──────────────────────────

func TestRouterName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"crm_prvms", "crm-prvms"},
		{"my-project", "my-project"},
		{"a_b_c_d", "a-b-c-d"},
		{"single", "single"},
		{"", ""},
	}
	for _, tt := range tests {
		f := &project.File{Name: tt.in}
		if got := f.RouterName(); got != tt.want {
			t.Errorf("RouterName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRouterName_NilReceiver(t *testing.T) {
	var f *project.File
	if got := f.RouterName(); got != "" {
		t.Errorf("RouterName on nil *File = %q, want \"\"", got)
	}
}

func TestString(t *testing.T) {
	f := &project.File{Name: "crm_prvms", Domain: "crm.prvms.ru", Path: "/opt/crm_prvms"}
	want := "crm_prvms (crm.prvms.ru) at /opt/crm_prvms"
	if got := f.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestString_NilReceiver(t *testing.T) {
	var f *project.File
	if got := f.String(); got != "<nil project>" {
		t.Errorf("String on nil *File = %q, want \"<nil project>\"", got)
	}
}

// ─── Load / FromBytes — happy paths ─────────────────────────────────

func TestLoad_ValidMinimal(t *testing.T) {
	path := filepath.Join("testdata", "valid_minimal.yaml")
	f, err := project.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Version != 1 {
		t.Errorf("Version = %d, want 1", f.Version)
	}
	if f.Name != "crm_prvms" {
		t.Errorf("Name = %q, want crm_prvms", f.Name)
	}
	if f.Domain != "crm.prvms.ru" {
		t.Errorf("Domain = %q, want crm.prvms.ru", f.Domain)
	}
	if f.Path != "/opt/crm_prvms" {
		t.Errorf("Path = %q, want /opt/crm_prvms", f.Path)
	}
	if f.Description != "" {
		t.Errorf("Description = %q, want empty", f.Description)
	}
	if f.HTTPSEntrypoint != nil {
		t.Errorf("HTTPSEntrypoint = %+v, want nil", f.HTTPSEntrypoint)
	}
	if !strings.HasSuffix(f.Source, "valid_minimal.yaml") {
		t.Errorf("Source = %q, expected path ending with testdata/valid_minimal.yaml", f.Source)
	}
}

func TestFromBytes_ShortcutEntrypoint(t *testing.T) {
	data := mustReadFixture(t, "valid_full_shortcut.yaml")
	f, err := project.FromBytes(data)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	if f.Description == "" {
		t.Error("Description should be parsed for full fixture")
	}
	if f.HTTPSEntrypoint == nil {
		t.Fatal("HTTPSEntrypoint should be set for shortcut form")
	}
	if f.HTTPSEntrypoint.Service != "web" {
		t.Errorf("Service = %q, want web", f.HTTPSEntrypoint.Service)
	}
	if f.HTTPSEntrypoint.Port != 0 {
		t.Errorf("Port = %d, want 0 (shortcut form infers from compose)", f.HTTPSEntrypoint.Port)
	}
	if f.Source != "" {
		t.Errorf("FromBytes should leave Source empty, got %q", f.Source)
	}
}

func TestFromBytes_ExplicitEntrypoint(t *testing.T) {
	data := mustReadFixture(t, "valid_full_object.yaml")
	f, err := project.FromBytes(data)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	if f.HTTPSEntrypoint == nil {
		t.Fatal("HTTPSEntrypoint should be set")
	}
	if f.HTTPSEntrypoint.Service != "web" {
		t.Errorf("Service = %q, want web", f.HTTPSEntrypoint.Service)
	}
	if f.HTTPSEntrypoint.Port != 8000 {
		t.Errorf("Port = %d, want 8000", f.HTTPSEntrypoint.Port)
	}
}

// ─── Load / FromBytes — invalid inputs ──────────────────────────────

func TestFromBytes_Invalid(t *testing.T) {
	// We only assert that an error happens; we don't pin specific
	// message substrings, because the upstream jsonschema library
	// phrasing isn't part of our public contract.
	fixtures := []string{
		"invalid_missing_name.yaml",
		"invalid_bad_name.yaml",
		"invalid_bad_domain.yaml",
		"invalid_unknown_field.yaml",
		"invalid_wrong_version.yaml",
		"invalid_bad_path.yaml",
		"invalid_entrypoint_no_service.yaml",
	}
	for _, fx := range fixtures {
		t.Run(fx, func(t *testing.T) {
			data := mustReadFixture(t, fx)
			if _, err := project.FromBytes(data); err == nil {
				t.Fatalf("expected error for %s, got nil", fx)
			}
		})
	}
}

func TestFromBytes_EmptyInput(t *testing.T) {
	if _, err := project.FromBytes(nil); err == nil {
		t.Fatal("expected error for nil input")
	}
	if _, err := project.FromBytes([]byte("   \n\t\n")); err == nil {
		t.Fatal("expected error for whitespace-only input")
	}
}

func TestFromBytes_MalformedYAML(t *testing.T) {
	data := []byte(`version: 1
name: crm
  badly: indented`)
	if _, err := project.FromBytes(data); err == nil {
		t.Fatal("expected YAML parse error")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	if _, err := project.Load("does/not/exist.yaml"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ─── helpers ────────────────────────────────────────────────────────

func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}
