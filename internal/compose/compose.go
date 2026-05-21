// Package compose wraps the `docker compose` CLI and parses compose
// configurations into the structural view flotilla needs.
//
// flotilla does not model the full compose schema: only services,
// networks, volumes, labels, ports, and healthchecks. Everything else
// the compose file declares is the project's business, not flotilla's.
//
// We invoke `docker compose` via os/exec rather than depending on the
// Docker Engine SDK; the compose CLI is the stable contract. See
// docs/ARCHITECTURE.md §8.1.
package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// File is a structural view of a compose project.
type File struct {
	RawPath  string // absolute path to the primary compose.yml
	Services map[string]Service
	Networks map[string]Network
	Volumes  map[string]Volume
}

// Service is one compose service after merge + interpolation by
// `docker compose config`.
type Service struct {
	Name        string
	Image       string
	Ports       []Port
	Expose      []int
	Networks    []string
	Labels      map[string]string
	Healthcheck *Healthcheck
}

// Port mirrors a compose `ports:` entry, normalized to the long form.
// `docker compose config --format json` emits ports as one entry per
// host port; the Target is the container-side port.
type Port struct {
	Target int    // container-side port (always set)
	Host   int    // host-side port (0 if not published)
	Bind   string // host bind interface (e.g. "127.0.0.1"), "" for all interfaces
}

// Network is one compose `networks:` entry.
type Network struct {
	Name     string
	External bool
}

// Volume is one compose `volumes:` entry.
type Volume struct {
	Name     string
	External bool
}

// Healthcheck mirrors compose's `healthcheck:` block.
type Healthcheck struct {
	Test        []string
	Interval    string
	Timeout     string
	Retries     int
	StartPeriod string
	Disable     bool
}

// ─── Compose CLI wrapper ─────────────────────────────────────────────

// dockerBinary is exec'd as `docker compose ...`. Override for testing.
var dockerBinary = "docker"

// Load runs `docker compose -f path [--env-file envFile] config --format json`
// and parses the result into a *File.
//
// envFile may be empty (compose then uses its own default lookup).
func Load(ctx context.Context, path, envFile string) (*File, error) {
	return LoadWithOverride(ctx, path, envFile, nil)
}

// LoadWithOverride is Load with an additional in-memory override file.
// Used by `flotilla deploy --dry` so the autocert override can be fed
// into linting without being persisted to disk.
//
// The override bytes are written to a temp file that is removed before
// the function returns.
func LoadWithOverride(ctx context.Context, path, envFile string, overrideYAML []byte) (*File, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", path, err)
	}

	files := []string{absPath}
	if len(overrideYAML) > 0 {
		tmp, err := writeTempOverride(overrideYAML)
		if err != nil {
			return nil, err
		}
		defer os.Remove(tmp)
		files = append(files, tmp)
	}

	out, err := composeConfig(ctx, filepath.Dir(absPath), files, envFile)
	if err != nil {
		return nil, err
	}

	file, err := parseComposeConfigJSON(out)
	if err != nil {
		return nil, err
	}
	file.RawPath = absPath
	return file, nil
}

func writeTempOverride(data []byte) (string, error) {
	f, err := os.CreateTemp("", "flotilla-override-*.yml")
	if err != nil {
		return "", fmt.Errorf("create temp override: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("write temp override: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("close temp override: %w", err)
	}
	return f.Name(), nil
}

// envFileArgs returns the `--env-file PATH` flag only when PATH is set
// and the file actually exists. `docker compose --env-file X` errors
// with "couldn't find env file" if X is missing, so a project that
// declares no environment variables (and therefore ships no .env)
// must not have the flag forced on it. Compose's own default .env
// lookup still applies when the flag is absent.
func envFileArgs(envFile string) []string {
	if envFile == "" {
		return nil
	}
	if _, err := os.Stat(envFile); err != nil {
		return nil
	}
	return []string{"--env-file", envFile}
}

func composeConfig(ctx context.Context, workdir string, composeFiles []string, envFile string) ([]byte, error) {
	args := []string{"compose"}
	for _, f := range composeFiles {
		args = append(args, "-f", f)
	}
	args = append(args, envFileArgs(envFile)...)
	args = append(args, "config", "--format", "json")

	cmd := exec.CommandContext(ctx, dockerBinary, args...)
	cmd.Dir = workdir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker compose config: %w; stderr: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Up runs
//
//	docker compose [-f ...]... [--env-file ...] up -d --remove-orphans --force-recreate
//
// in projectDir. Streams stdout/stderr to the provided writers.
func Up(ctx context.Context, projectDir string, composeFiles []string, envFile string, stdout, stderr io.Writer) error {
	args := []string{"compose"}
	for _, f := range composeFiles {
		args = append(args, "-f", f)
	}
	args = append(args, envFileArgs(envFile)...)
	args = append(args, "up", "-d", "--remove-orphans", "--force-recreate")

	cmd := exec.CommandContext(ctx, dockerBinary, args...)
	cmd.Dir = projectDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	return nil
}

// ContainerStatus is one row from `docker compose ps --format json`.
type ContainerStatus struct {
	Service string
	Name    string
	State   string // running / exited / created / restarting / ...
	Health  string // healthy / unhealthy / starting / "" (no healthcheck)
}

// PS returns the current state of every container in the compose project.
func PS(ctx context.Context, projectDir string, composeFiles []string) ([]ContainerStatus, error) {
	args := []string{"compose"}
	for _, f := range composeFiles {
		args = append(args, "-f", f)
	}
	args = append(args, "ps", "--format", "json", "--all")

	cmd := exec.CommandContext(ctx, dockerBinary, args...)
	cmd.Dir = projectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker compose ps: %w; stderr: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parseComposePSJSON(stdout.Bytes())
}

// ─── JSON → File parsing ─────────────────────────────────────────────

// parseComposeConfigJSON parses the output of `docker compose config --format json`
// into a *File.
func parseComposeConfigJSON(data []byte) (*File, error) {
	var raw struct {
		Services map[string]json.RawMessage `json:"services"`
		Networks map[string]json.RawMessage `json:"networks"`
		Volumes  map[string]json.RawMessage `json:"volumes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse compose config JSON: %w", err)
	}

	f := &File{
		Services: make(map[string]Service, len(raw.Services)),
		Networks: make(map[string]Network, len(raw.Networks)),
		Volumes:  make(map[string]Volume, len(raw.Volumes)),
	}

	for name, blob := range raw.Services {
		svc, err := parseService(name, blob)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", name, err)
		}
		f.Services[name] = svc
	}
	for name, blob := range raw.Networks {
		nw, err := parseNetwork(name, blob)
		if err != nil {
			return nil, fmt.Errorf("network %q: %w", name, err)
		}
		f.Networks[name] = nw
	}
	for name, blob := range raw.Volumes {
		vol, err := parseVolume(name, blob)
		if err != nil {
			return nil, fmt.Errorf("volume %q: %w", name, err)
		}
		f.Volumes[name] = vol
	}
	return f, nil
}

func parseService(name string, blob json.RawMessage) (Service, error) {
	// `docker compose config --format json` always emits services in long
	// form. Labels arrive as a map; ports as objects; healthcheck as an
	// object. Some fields may be absent.
	var raw struct {
		Image       string            `json:"image"`
		Labels      map[string]string `json:"labels"`
		Networks    json.RawMessage   `json:"networks"`
		Ports       []rawPort         `json:"ports"`
		Expose      []json.Number     `json:"expose"`
		Healthcheck *rawHealthcheck   `json:"healthcheck"`
	}
	if err := json.Unmarshal(blob, &raw); err != nil {
		return Service{}, err
	}
	svc := Service{
		Name:   name,
		Image:  raw.Image,
		Labels: raw.Labels,
	}
	for _, p := range raw.Ports {
		svc.Ports = append(svc.Ports, p.toPort())
	}
	for _, e := range raw.Expose {
		n, err := strconv.Atoi(e.String())
		if err != nil {
			return Service{}, fmt.Errorf("expose entry %q: %w", e.String(), err)
		}
		svc.Expose = append(svc.Expose, n)
	}
	nets, err := parseServiceNetworks(raw.Networks)
	if err != nil {
		return Service{}, fmt.Errorf("networks: %w", err)
	}
	svc.Networks = nets
	if raw.Healthcheck != nil {
		hc := raw.Healthcheck.toHealthcheck()
		svc.Healthcheck = &hc
	}
	return svc, nil
}

// rawPort matches the long form emitted by `compose config --format json`:
//
//	{"mode":"ingress","target":80,"published":"8080","protocol":"tcp","host_ip":"127.0.0.1"}
//
// `published` may be a number or a string (depending on whether the user
// wrote `8080` or `"8080"` in YAML), so we use json.Number for tolerance.
type rawPort struct {
	Target    json.Number `json:"target"`
	Published json.Number `json:"published"`
	HostIP    string      `json:"host_ip"`
}

func (p rawPort) toPort() Port {
	target, _ := strconv.Atoi(p.Target.String())
	host, _ := strconv.Atoi(p.Published.String())
	return Port{Target: target, Host: host, Bind: p.HostIP}
}

// parseServiceNetworks handles both forms compose emits for the
// per-service `networks:` field: a list of names (legacy) or a map of
// name → config (current default for `compose config`).
func parseServiceNetworks(blob json.RawMessage) ([]string, error) {
	if len(blob) == 0 {
		return nil, nil
	}
	// Try map first (most common with config output).
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(blob, &asMap); err == nil {
		out := make([]string, 0, len(asMap))
		for k := range asMap {
			out = append(out, k)
		}
		return out, nil
	}
	// Fallback: list.
	var asList []string
	if err := json.Unmarshal(blob, &asList); err == nil {
		return asList, nil
	}
	return nil, fmt.Errorf("unsupported networks form: %s", string(blob))
}

// rawHealthcheck mirrors the JSON shape of compose's healthcheck block.
type rawHealthcheck struct {
	Test        []string `json:"test"`
	Interval    string   `json:"interval"`
	Timeout     string   `json:"timeout"`
	Retries     int      `json:"retries"`
	StartPeriod string   `json:"start_period"`
	Disable     bool     `json:"disable"`
}

func (h rawHealthcheck) toHealthcheck() Healthcheck {
	return Healthcheck{
		Test:        h.Test,
		Interval:    h.Interval,
		Timeout:     h.Timeout,
		Retries:     h.Retries,
		StartPeriod: h.StartPeriod,
		Disable:     h.Disable,
	}
}

func parseNetwork(name string, blob json.RawMessage) (Network, error) {
	var raw struct {
		External any    `json:"external"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal(blob, &raw); err != nil {
		return Network{}, err
	}
	nw := Network{Name: raw.Name}
	if nw.Name == "" {
		nw.Name = name
	}
	nw.External = boolish(raw.External)
	return nw, nil
}

func parseVolume(name string, blob json.RawMessage) (Volume, error) {
	var raw struct {
		External any    `json:"external"`
		Name     string `json:"name"`
	}
	if err := json.Unmarshal(blob, &raw); err != nil {
		return Volume{}, err
	}
	vol := Volume{Name: raw.Name}
	if vol.Name == "" {
		vol.Name = name
	}
	vol.External = boolish(raw.External)
	return vol, nil
}

// boolish accepts the two ways compose serializes external markers:
//
//	external: true                ← scalar bool
//	external: {name: my-network}  ← object means «external with custom name»
//
// Both indicate «external = true» from flotilla's perspective.
func boolish(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case map[string]any:
		return true
	default:
		return false
	}
}

// parseComposePSJSON parses `docker compose ps --format json`.
//
// Compose emits one JSON object per line (NDJSON) for ps, not a JSON
// array; we handle both forms defensively.
func parseComposePSJSON(data []byte) ([]ContainerStatus, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}
	// Try JSON array first.
	if trimmed[0] == '[' {
		var arr []psRecord
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, fmt.Errorf("parse ps array: %w", err)
		}
		return psToStatus(arr), nil
	}
	// Fallback: NDJSON.
	var out []psRecord
	for _, line := range bytes.Split(trimmed, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var rec psRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("parse ps line %q: %w", string(line), err)
		}
		out = append(out, rec)
	}
	return psToStatus(out), nil
}

type psRecord struct {
	Service string `json:"Service"`
	Name    string `json:"Name"`
	State   string `json:"State"`
	Health  string `json:"Health"`
}

func psToStatus(in []psRecord) []ContainerStatus {
	out := make([]ContainerStatus, len(in))
	for i, r := range in {
		out[i] = ContainerStatus{
			Service: r.Service,
			Name:    r.Name,
			State:   r.State,
			Health:  r.Health,
		}
	}
	return out
}

// ─── YAML pre-parsing (for envcheck) ─────────────────────────────────

// IsValidYAML is a cheap sanity check: does the byte slice parse as
// YAML at all? Returns nil if so, or the parse error.
//
// Used by lint rule L001 (Compose syntax) before we shell out to the
// heavier `docker compose config`.
func IsValidYAML(data []byte) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("compose file is empty")
	}
	var v any
	return yaml.Unmarshal(data, &v)
}
