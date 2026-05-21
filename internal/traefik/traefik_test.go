package traefik

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleRoutersJSON = `[
  {
    "name": "crm-prvms@docker",
    "rule": "Host(` + "`crm.prvms.ru`" + `)",
    "service": "crm-prvms",
    "entryPoints": ["websecure"],
    "tls": {"certResolver": "le"},
    "priority": 1,
    "status": "enabled"
  },
  {
    "name": "dashboard@internal",
    "rule": "PathPrefix(` + "`/api`" + `)",
    "service": "api@internal",
    "entryPoints": ["traefik"],
    "priority": 2147483646,
    "status": "enabled"
  },
  {
    "name": "other@docker",
    "rule": "Host(` + "`other.example.com`" + `)",
    "service": "other",
    "entryPoints": ["websecure"],
    "tls": null,
    "status": "enabled"
  }
]`

func newTestServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http/routers" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestNew_DefaultAddr(t *testing.T) {
	c := New("")
	if c.Addr() != DefaultAddr {
		t.Errorf("Addr() = %q, want %q", c.Addr(), DefaultAddr)
	}
	c = New("http://127.0.0.1:9999/")
	if c.Addr() != "http://127.0.0.1:9999" {
		t.Errorf("trailing slash should be trimmed, got %q", c.Addr())
	}
}

func TestHTTPRouters_ParsesAndMapsTLS(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, sampleRoutersJSON)
	defer srv.Close()

	routers, err := New(srv.URL).HTTPRouters(context.Background())
	if err != nil {
		t.Fatalf("HTTPRouters: %v", err)
	}
	if len(routers) != 3 {
		t.Fatalf("len = %d, want 3", len(routers))
	}

	crm := routers[0]
	if crm.Name != "crm-prvms@docker" || crm.Service != "crm-prvms" {
		t.Errorf("router[0] = %+v", crm)
	}
	if !crm.TLS {
		t.Errorf("crm router should have TLS=true (tls object present)")
	}
	if crm.Priority != 1 {
		t.Errorf("crm priority = %d, want 1", crm.Priority)
	}

	// tls: null → TLS false.
	if routers[2].TLS {
		t.Errorf("router with tls:null should have TLS=false")
	}
	// tls absent → TLS false.
	if routers[1].TLS {
		t.Errorf("router with no tls field should have TLS=false")
	}
}

func TestFindByHost(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, sampleRoutersJSON)
	defer srv.Close()

	c := New(srv.URL)
	got, err := c.FindByHost(context.Background(), "crm.prvms.ru")
	if err != nil {
		t.Fatalf("FindByHost: %v", err)
	}
	if len(got) != 1 || got[0].Name != "crm-prvms@docker" {
		t.Errorf("FindByHost(crm.prvms.ru) = %+v, want the crm router only", got)
	}

	none, err := c.FindByHost(context.Background(), "absent.example.com")
	if err != nil {
		t.Fatalf("FindByHost: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("FindByHost(absent) = %+v, want empty", none)
	}
}

func TestHTTPRouters_Non200(t *testing.T) {
	srv := newTestServer(t, http.StatusInternalServerError, "boom")
	defer srv.Close()
	if _, err := New(srv.URL).HTTPRouters(context.Background()); err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestHTTPRouters_BadJSON(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, "{not an array")
	defer srv.Close()
	if _, err := New(srv.URL).HTTPRouters(context.Background()); err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}

func TestHTTPRouters_Unreachable(t *testing.T) {
	// Port 1 is not listening; expect a connection error, not a panic.
	c := New("http://127.0.0.1:1")
	if _, err := c.HTTPRouters(context.Background()); err == nil {
		t.Fatal("expected error for unreachable Traefik")
	}
}
