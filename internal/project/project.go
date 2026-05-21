// Package project parses and validates project.yml files against the
// v1 JSON schema embedded at internal/project/v1.schema.json.
//
// The parsed *File is the canonical in-memory representation; callers
// never touch raw YAML directly. See docs/ARCHITECTURE.md §3 for the
// rationale of each field.
package project

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

// File is a parsed and validated project.yml v1.
type File struct {
	Version         int
	Name            string
	Domain          string
	Path            string
	Description     string           // "" when omitted
	HTTPSEntrypoint *HTTPSEntrypoint // nil when omitted

	// Source records where this File came from on disk. Empty for
	// in-memory inputs (FromBytes). Used in error messages and by the
	// symlinks step at deploy time.
	Source string
}

// HTTPSEntrypoint is the normalized form of the project.yml field.
// The YAML union (string vs object) is collapsed at parse time:
//
//	https_entrypoint: web
//	https_entrypoint: { service: web, port: 8000 }
//
// both produce a non-nil *HTTPSEntrypoint with Service set; Port is 0
// in the shortcut form (meaning «infer from compose»).
type HTTPSEntrypoint struct {
	Service string
	Port    int // 0 means "infer from compose"
}

// RouterName returns the Traefik router/service base-name derived from
// File.Name. Underscores are replaced with hyphens (the Traefik label
// grammar does not allow underscores in router names).
//
//	"crm_prvms"  → "crm-prvms"
//	"my-project" → "my-project"
func (f *File) RouterName() string {
	if f == nil {
		return ""
	}
	return strings.ReplaceAll(f.Name, "_", "-")
}

// String is a one-line human summary, used in logs and status output.
func (f *File) String() string {
	if f == nil {
		return "<nil project>"
	}
	return fmt.Sprintf("%s (%s) at %s", f.Name, f.Domain, f.Path)
}

// ─── Schema embedding ────────────────────────────────────────────────

//go:embed v1.schema.json
var v1SchemaBytes []byte

// schemaURL is the in-memory identifier for the embedded schema. It
// matches the schema's own $id so error messages reference a real URL.
const schemaURL = "https://raw.githubusercontent.com/DmitriyKurilenko/flotilla/main/internal/project/v1.schema.json"

var compiledV1 *jsonschema.Schema

func init() {
	c := jsonschema.NewCompiler()
	if err := c.AddResource(schemaURL, bytes.NewReader(v1SchemaBytes)); err != nil {
		panic(fmt.Sprintf("project: register embedded schema: %v", err))
	}
	sch, err := c.Compile(schemaURL)
	if err != nil {
		panic(fmt.Sprintf("project: compile embedded schema: %v", err))
	}
	compiledV1 = sch
}

// ─── Parsing ─────────────────────────────────────────────────────────

// Load reads project.yml from path, parses it, and validates against
// the v1 JSON schema. The File.Source field is set to path.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	f, err := FromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	f.Source = path
	return f, nil
}

// FromBytes parses and validates a project.yml document given as raw
// bytes. Used by tests and by callers that already have the YAML in
// memory.
func FromBytes(data []byte) (*File, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("project.yml is empty")
	}

	// Pass 1: parse YAML into a generic structure for schema validation.
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	// yaml.v3 already produces map[string]any for string-keyed mappings,
	// but we normalize defensively for non-string keys (which never
	// appear in valid project.yml but produce confusing errors otherwise).
	normalized, err := normalizeYAML(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	if err := compiledV1.Validate(normalized); err != nil {
		return nil, fmt.Errorf("schema validation: %w", err)
	}

	// Pass 2: decode into typed struct now that the shape is known good.
	var doc yamlDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		// Should not happen — schema validation passed, so the YAML is
		// well-formed and shaped correctly. Treat as internal error.
		return nil, fmt.Errorf("decode after validation passed: %w", err)
	}

	return doc.toFile(), nil
}

// yamlDoc is the typed YAML decoding target.
type yamlDoc struct {
	Version         int             `yaml:"version"`
	Name            string          `yaml:"name"`
	Domain          string          `yaml:"domain"`
	Path            string          `yaml:"path"`
	Description     string          `yaml:"description"`
	HTTPSEntrypoint *yamlEntrypoint `yaml:"https_entrypoint"`
}

// yamlEntrypoint handles the union (string OR {service, port}) at decode
// time. The schema validator has already accepted the shape; this code
// only translates the accepted shapes into a uniform Service/Port pair.
type yamlEntrypoint struct {
	Service string
	Port    int
}

// UnmarshalYAML accepts either a bare scalar (shortcut form) or a
// mapping with service/port keys (explicit form).
func (e *yamlEntrypoint) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		e.Service = n.Value
		return nil
	case yaml.MappingNode:
		var explicit struct {
			Service string `yaml:"service"`
			Port    int    `yaml:"port"`
		}
		if err := n.Decode(&explicit); err != nil {
			return err
		}
		e.Service = explicit.Service
		e.Port = explicit.Port
		return nil
	default:
		return fmt.Errorf("https_entrypoint: unexpected YAML node kind %d (line %d)", n.Kind, n.Line)
	}
}

func (d yamlDoc) toFile() *File {
	f := &File{
		Version:     d.Version,
		Name:        d.Name,
		Domain:      d.Domain,
		Path:        d.Path,
		Description: d.Description,
	}
	if d.HTTPSEntrypoint != nil {
		f.HTTPSEntrypoint = &HTTPSEntrypoint{
			Service: d.HTTPSEntrypoint.Service,
			Port:    d.HTTPSEntrypoint.Port,
		}
	}
	return f
}

// normalizeYAML walks a generic YAML-decoded value and rewrites every
// map[any]any to map[string]any. yaml.v3 produces the latter directly
// for string-keyed mappings; this function exists as a defensive layer
// against unusual decodings (e.g. nested merge keys) so that the
// jsonschema validator never sees a non-string map key.
func normalizeYAML(v any) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			n, err := normalizeYAML(val)
			if err != nil {
				return nil, err
			}
			out[k] = n
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			ks, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("non-string key in mapping: %v", k)
			}
			n, err := normalizeYAML(val)
			if err != nil {
				return nil, err
			}
			out[ks] = n
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			n, err := normalizeYAML(val)
			if err != nil {
				return nil, err
			}
			out[i] = n
		}
		return out, nil
	default:
		return v, nil
	}
}
