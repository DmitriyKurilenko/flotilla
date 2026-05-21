package compose

import (
	"os"
	"strings"
	"testing"
)

// ─── parseComposeConfigJSON ─────────────────────────────────────────

func TestParseComposeConfigJSON_Minimal(t *testing.T) {
	in := []byte(`{
  "services": {
    "web": {
      "image": "nginx:1.27-alpine",
      "labels": {"flotilla.role": "ingress"},
      "networks": {"proxy": null},
      "ports": [
        {"mode":"ingress","target":80,"published":"80","protocol":"tcp"},
        {"mode":"ingress","target":443,"published":"443","protocol":"tcp"}
      ],
      "expose": ["8080"],
      "healthcheck": {
        "test": ["CMD","wget","-qO-","http://127.0.0.1/"],
        "interval": "30s",
        "timeout": "5s",
        "retries": 3,
        "start_period": "10s"
      }
    },
    "db": {
      "image": "postgres:17"
    }
  },
  "networks": {
    "proxy": {"external": true, "name": "proxy"},
    "backend": {}
  },
  "volumes": {
    "static_volume": {},
    "shared": {"external": {"name": "shared-data"}, "name": "shared-data"}
  }
}`)
	f, err := parseComposeConfigJSON(in)
	if err != nil {
		t.Fatalf("parseComposeConfigJSON: %v", err)
	}
	if len(f.Services) != 2 {
		t.Fatalf("services len = %d, want 2", len(f.Services))
	}
	web := f.Services["web"]
	if web.Image != "nginx:1.27-alpine" {
		t.Errorf("web.Image = %q", web.Image)
	}
	if got := web.Labels["flotilla.role"]; got != "ingress" {
		t.Errorf("web label flotilla.role = %q, want ingress", got)
	}
	if len(web.Ports) != 2 {
		t.Errorf("web.Ports len = %d, want 2", len(web.Ports))
	}
	if web.Ports[0].Target != 80 || web.Ports[0].Host != 80 {
		t.Errorf("web.Ports[0] = %+v", web.Ports[0])
	}
	if len(web.Expose) != 1 || web.Expose[0] != 8080 {
		t.Errorf("web.Expose = %v, want [8080]", web.Expose)
	}
	if !containsString(web.Networks, "proxy") {
		t.Errorf("web.Networks = %v, expected to contain 'proxy'", web.Networks)
	}
	if web.Healthcheck == nil {
		t.Fatal("web.Healthcheck nil")
	}
	if web.Healthcheck.Retries != 3 || web.Healthcheck.Interval != "30s" {
		t.Errorf("web.Healthcheck = %+v", web.Healthcheck)
	}

	if proxy, ok := f.Networks["proxy"]; !ok || !proxy.External {
		t.Errorf("networks[proxy] = %+v (external should be true)", proxy)
	}
	if shared, ok := f.Volumes["shared"]; !ok || !shared.External {
		t.Errorf("volumes[shared] = %+v (external should be true via object form)", shared)
	}
	if static, ok := f.Volumes["static_volume"]; !ok || static.External {
		t.Errorf("volumes[static_volume] = %+v (external should be false)", static)
	}
}

func TestParseComposeConfigJSON_PortsAsObjectStyle(t *testing.T) {
	// `docker compose config --format json` may emit `published` as a
	// number too (newer versions). Make sure we handle both.
	in := []byte(`{
  "services": {
    "web": {
      "image": "nginx",
      "ports": [
        {"target":80,"published":80,"protocol":"tcp"},
        {"target":443,"published":"443","protocol":"tcp","host_ip":"127.0.0.1"}
      ]
    }
  }
}`)
	f, err := parseComposeConfigJSON(in)
	if err != nil {
		t.Fatalf("parseComposeConfigJSON: %v", err)
	}
	web := f.Services["web"]
	if len(web.Ports) != 2 {
		t.Fatalf("Ports len = %d", len(web.Ports))
	}
	if web.Ports[0].Host != 80 || web.Ports[1].Host != 443 {
		t.Errorf("Host ports = %v / %v", web.Ports[0].Host, web.Ports[1].Host)
	}
	if web.Ports[1].Bind != "127.0.0.1" {
		t.Errorf("Bind = %q, want 127.0.0.1", web.Ports[1].Bind)
	}
}

func TestParseComposeConfigJSON_NetworksAsList(t *testing.T) {
	// Legacy form: `networks: [proxy, backend]` instead of object map.
	in := []byte(`{
  "services": {
    "web": {
      "image": "nginx",
      "networks": ["proxy","backend"]
    }
  }
}`)
	f, err := parseComposeConfigJSON(in)
	if err != nil {
		t.Fatalf("parseComposeConfigJSON: %v", err)
	}
	web := f.Services["web"]
	if !containsString(web.Networks, "proxy") || !containsString(web.Networks, "backend") {
		t.Errorf("Networks = %v, want both proxy and backend", web.Networks)
	}
}

func TestParseComposeConfigJSON_Empty(t *testing.T) {
	f, err := parseComposeConfigJSON([]byte(`{"services":{},"networks":{},"volumes":{}}`))
	if err != nil {
		t.Fatalf("parseComposeConfigJSON: %v", err)
	}
	if len(f.Services) != 0 || len(f.Networks) != 0 || len(f.Volumes) != 0 {
		t.Errorf("empty compose should produce empty maps")
	}
}

func TestParseComposeConfigJSON_Malformed(t *testing.T) {
	if _, err := parseComposeConfigJSON([]byte(`{not json`)); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// ─── parseComposePSJSON ────────────────────────────────────────────

func TestParseComposePSJSON_Array(t *testing.T) {
	in := []byte(`[
  {"Service":"web","Name":"crm-web-1","State":"running","Health":"healthy"},
  {"Service":"db","Name":"crm-db-1","State":"running","Health":""}
]`)
	got, err := parseComposePSJSON(in)
	if err != nil {
		t.Fatalf("parseComposePSJSON: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Service != "web" || got[0].Health != "healthy" {
		t.Errorf("got[0] = %+v", got[0])
	}
}

func TestParseComposePSJSON_NDJSON(t *testing.T) {
	in := []byte(`{"Service":"web","Name":"crm-web-1","State":"running","Health":"healthy"}
{"Service":"db","Name":"crm-db-1","State":"running","Health":""}`)
	got, err := parseComposePSJSON(in)
	if err != nil {
		t.Fatalf("parseComposePSJSON: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[1].Service != "db" {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestParseComposePSJSON_Empty(t *testing.T) {
	got, err := parseComposePSJSON(nil)
	if err != nil {
		t.Fatalf("nil input: %v", err)
	}
	if got != nil {
		t.Errorf("nil input → got %v, want nil", got)
	}
	got, err = parseComposePSJSON([]byte("  \n\n  "))
	if err != nil {
		t.Fatalf("whitespace input: %v", err)
	}
	if got != nil {
		t.Errorf("whitespace input → got %v, want nil", got)
	}
}

// ─── IsValidYAML ───────────────────────────────────────────────────

func TestIsValidYAML(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid services block", "services:\n  web:\n    image: nginx\n", false},
		{"empty", "", true},
		{"whitespace only", "   \n\n", true},
		{"malformed indent", "services:\n  web:\n badly indented\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidYAML([]byte(tt.input))
			if tt.wantErr && err == nil {
				t.Errorf("want error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// ─── envFileArgs ───────────────────────────────────────────────────

func TestEnvFileArgs(t *testing.T) {
	if got := envFileArgs(""); got != nil {
		t.Errorf("empty path → %v, want nil", got)
	}
	if got := envFileArgs("/no/such/.env"); got != nil {
		t.Errorf("missing file → %v, want nil (compose must not be forced --env-file)", got)
	}
	dir := t.TempDir()
	p := dir + "/.env"
	if err := os.WriteFile(p, []byte("X=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := envFileArgs(p)
	if len(got) != 2 || got[0] != "--env-file" || got[1] != p {
		t.Errorf("existing file → %v, want [--env-file %s]", got, p)
	}
}

// ─── helpers ───────────────────────────────────────────────────────

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// (kept for hand-debugging large JSON blobs; not used by current tests)
var _ = strings.TrimSpace
