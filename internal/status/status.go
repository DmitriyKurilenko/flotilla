// Package status implements `flotilla status`. It combines compose
// container state, Traefik router registration, ACME storage state,
// and an external HTTPS probe into one Report.
//
// status is informational: every probe degrades gracefully. The only
// hard error is «project.yml could not be loaded» — without it there
// is nothing to report.
//
// See docs/ARCHITECTURE.md §6.1 for the rendered output.
package status

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"time"

	"github.com/DmitriyKurilenko/flotilla/internal/acme"
	"github.com/DmitriyKurilenko/flotilla/internal/compose"
	"github.com/DmitriyKurilenko/flotilla/internal/project"
	"github.com/DmitriyKurilenko/flotilla/internal/state"
	"github.com/DmitriyKurilenko/flotilla/internal/traefik"
)

// Report is the rendered status of one project.
type Report struct {
	Name           string
	Domain         string
	Description    string
	Containers     []ContainerLine
	TraefikRouters []string // router names whose rule contains Host(<domain>)
	CertSubject    string   // e.g. "crm.prvms.ru"; empty if no cert found
	CertExpiresAt  string   // RFC3339; empty if no cert
	HTTPStatus     int      // status code of GET https://<domain>/; 0 if the request failed
	HTTPLatencyMS  int
	LastDeploySHA  string
	LastDeployAt   string // RFC3339, from .flotilla/state.json
}

// ContainerLine is one row of the "containers" section.
type ContainerLine struct {
	Service string
	State   string // running / exited / created / restarting / ...
	Health  string // healthy / unhealthy / starting / "" (no healthcheck)
}

// Options tunes Collect's external probes. The zero value uses
// production defaults; tests inject overrides.
type Options struct {
	TraefikAddr string // default traefik.DefaultAddr
	ACMEPath    string // default acme.DefaultPath
	HTTPClient  *http.Client
	HTTPScheme  string // default "https"
}

func (o Options) traefikAddr() string {
	if o.TraefikAddr == "" {
		return traefik.DefaultAddr
	}
	return o.TraefikAddr
}

func (o Options) acmePath() string {
	if o.ACMEPath == "" {
		return acme.DefaultPath
	}
	return o.ACMEPath
}

func (o Options) httpScheme() string {
	if o.HTTPScheme == "" {
		return "https"
	}
	return o.HTTPScheme
}

func (o Options) httpClient() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{
		Timeout: 5 * time.Second,
		// During cert issuance the TLS cert may be Traefik's default
		// self-signed one. We only care about the HTTP status reachable
		// at the domain, not cert validity (that's the acme probe's job).
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // status probe only
		},
	}
}

// Collect builds a Report for the project at projectDir. projectDir
// must contain a project.yml.
func Collect(ctx context.Context, projectDir string, opts Options) (*Report, error) {
	proj, err := project.Load(filepath.Join(projectDir, "project.yml"))
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}

	rep := &Report{
		Name:        proj.Name,
		Domain:      proj.Domain,
		Description: proj.Description,
	}

	// ── containers (best-effort) ──
	if cs, err := compose.PS(ctx, projectDir, []string{filepath.Join(projectDir, "compose.yml")}); err == nil {
		for _, c := range cs {
			rep.Containers = append(rep.Containers, ContainerLine{
				Service: c.Service, State: c.State, Health: c.Health,
			})
		}
		sort.Slice(rep.Containers, func(i, j int) bool {
			return rep.Containers[i].Service < rep.Containers[j].Service
		})
	}

	// ── traefik routers (best-effort) ──
	tc := traefik.New(opts.traefikAddr())
	if routers, err := tc.FindByHost(ctx, proj.Domain); err == nil {
		for _, r := range routers {
			rep.TraefikRouters = append(rep.TraefikRouters, r.Name)
		}
		sort.Strings(rep.TraefikRouters)
	}

	// ── cert (best-effort) ──
	if c, err := acme.FindByDomain(opts.acmePath(), proj.Domain); err == nil && c != nil {
		rep.CertSubject = c.Domain
		rep.CertExpiresAt = c.NotAfter.UTC().Format(time.RFC3339)
	}

	// ── http probe (best-effort) ──
	code, ms := probeHTTP(ctx, opts.httpClient(), opts.httpScheme()+"://"+proj.Domain+"/")
	rep.HTTPStatus = code
	rep.HTTPLatencyMS = ms

	// ── last deploy (best-effort) ──
	if st, err := state.Read(projectDir); err == nil && st != nil {
		rep.LastDeploySHA = st.SHA
		rep.LastDeployAt = st.DeployedAt
	}

	return rep, nil
}

// probeHTTP issues a GET and returns (statusCode, latencyMillis).
// On any transport error it returns (0, latencyMillis) — a zero status
// means «did not get an HTTP response».
func probeHTTP(ctx context.Context, client *http.Client, url string) (int, int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0
	}
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := int(time.Since(start).Milliseconds())
	if err != nil {
		return 0, elapsed
	}
	defer resp.Body.Close()
	return resp.StatusCode, elapsed
}
