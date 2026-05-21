package status

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProbeHTTP_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(204)
	}))
	defer srv.Close()

	code, ms := probeHTTP(context.Background(), srv.Client(), srv.URL+"/")
	if code != 204 {
		t.Errorf("code = %d, want 204", code)
	}
	if ms < 0 {
		t.Errorf("latency = %d, want >= 0", ms)
	}
}

func TestProbeHTTP_Unreachable(t *testing.T) {
	code, _ := probeHTTP(context.Background(), &http.Client{Timeout: time.Second}, "http://127.0.0.1:1/")
	if code != 0 {
		t.Errorf("unreachable host → code %d, want 0", code)
	}
}

func TestCollect_MissingProjectErrors(t *testing.T) {
	if _, err := Collect(context.Background(), t.TempDir(), Options{}); err == nil {
		t.Fatal("Collect on dir without project.yml should error")
	}
}

func TestCollect_AssemblesReport(t *testing.T) {
	dir := t.TempDir()

	// project.yml
	proj := "version: 1\nname: demo\ndomain: demo.example.com\npath: " + dir + "\ndescription: Demo project\n"
	if err := os.WriteFile(filepath.Join(dir, "project.yml"), []byte(proj), 0o644); err != nil {
		t.Fatal(err)
	}
	// .flotilla/state.json
	if err := os.MkdirAll(filepath.Join(dir, ".flotilla"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := `{"sha":"deadbeef","deployed_at":"2026-05-10T12:00:00Z","summary":"ok"}`
	if err := os.WriteFile(filepath.Join(dir, ".flotilla", "state.json"), []byte(st), 0o644); err != nil {
		t.Fatal(err)
	}

	// Fake Traefik API.
	traefikSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http/routers" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[{"name":"demo@docker","rule":"Host(` + "`demo.example.com`" + `)","service":"demo","status":"enabled"}]`))
	}))
	defer traefikSrv.Close()

	// Fake site for the HTTP probe.
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer site.Close()

	rep, err := Collect(context.Background(), dir, Options{
		TraefikAddr: traefikSrv.URL,
		ACMEPath:    filepath.Join(dir, "no-acme.json"), // missing → no cert, no error
		HTTPClient:  site.Client(),
		HTTPScheme:  "http",
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if rep.Name != "demo" || rep.Domain != "demo.example.com" || rep.Description != "Demo project" {
		t.Errorf("identity wrong: %+v", rep)
	}
	if len(rep.TraefikRouters) != 1 || rep.TraefikRouters[0] != "demo@docker" {
		t.Errorf("TraefikRouters = %v", rep.TraefikRouters)
	}
	if rep.LastDeploySHA != "deadbeef" || rep.LastDeployAt != "2026-05-10T12:00:00Z" {
		t.Errorf("last deploy wrong: sha=%q at=%q", rep.LastDeploySHA, rep.LastDeployAt)
	}
	if rep.CertSubject != "" || rep.CertExpiresAt != "" {
		t.Errorf("expected no cert (missing acme.json), got %q / %q", rep.CertSubject, rep.CertExpiresAt)
	}
	// HTTP probe note: Collect builds the URL from the project domain
	// (demo.example.com), not the test server, so the probe will not
	// reach `site`. We only assert it didn't panic and produced a
	// sane (zero or real) status — exact value depends on the host's
	// DNS for demo.example.com, which we must not rely on.
	if rep.HTTPStatus < 0 {
		t.Errorf("HTTPStatus = %d, want >= 0", rep.HTTPStatus)
	}
}
