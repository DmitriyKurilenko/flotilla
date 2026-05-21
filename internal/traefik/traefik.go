// Package traefik is a read-only client for Traefik's HTTP API,
// typically reachable at http://127.0.0.1:8080 on a flotilla-managed
// VPS (install.sh binds the dashboard to localhost only).
//
// flotilla never reconfigures Traefik via this client — Traefik's
// configuration is its own (in /opt/traefik) plus the labels in each
// project's compose. This client exists for `flotilla status` and for
// deploy pipeline step 7 («is my project's router registered?»).
package traefik

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DefaultAddr is where install.sh exposes the Traefik dashboard.
const DefaultAddr = "http://127.0.0.1:8080"

// Client is a Traefik HTTP API client.
type Client struct {
	addr string
	http *http.Client
}

// New returns a Client targeting addr. Use DefaultAddr when in doubt.
// A nil-safe default HTTP client with a short timeout is used; Traefik
// is local so requests should be fast or fail fast.
func New(addr string) *Client {
	if addr == "" {
		addr = DefaultAddr
	}
	return &Client{
		addr: strings.TrimRight(addr, "/"),
		http: &http.Client{Timeout: 5 * time.Second},
	}
}

// Addr returns the configured base address. Useful for log messages.
func (c *Client) Addr() string { return c.addr }

// Router is the subset of /api/http/routers we care about.
type Router struct {
	Name        string
	Rule        string
	Service     string
	Entrypoints []string
	TLS         bool
	Priority    int
	Status      string // "enabled" / "disabled" / "warning"
}

// apiRouter mirrors the JSON shape Traefik returns. The `tls` field is
// an object when TLS is configured and absent otherwise, so we decode
// it as a raw message and treat presence as «TLS on».
type apiRouter struct {
	Name        string          `json:"name"`
	Rule        string          `json:"rule"`
	Service     string          `json:"service"`
	EntryPoints []string        `json:"entryPoints"`
	TLS         json.RawMessage `json:"tls"`
	Priority    int             `json:"priority"`
	Status      string          `json:"status"`
}

// HTTPRouters returns every HTTP router currently registered with
// Traefik (GET /api/http/routers).
func (c *Client) HTTPRouters(ctx context.Context) ([]Router, error) {
	url := c.addr + "/api/http/routers"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("traefik: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("traefik: GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("traefik: GET %s: unexpected status %s", url, resp.Status)
	}

	var raw []apiRouter
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("traefik: decode routers: %w", err)
	}

	out := make([]Router, 0, len(raw))
	for _, r := range raw {
		out = append(out, Router{
			Name:        r.Name,
			Rule:        r.Rule,
			Service:     r.Service,
			Entrypoints: r.EntryPoints,
			TLS:         len(r.TLS) > 0 && string(r.TLS) != "null",
			Priority:    r.Priority,
			Status:      r.Status,
		})
	}
	return out, nil
}

// FindByHost returns the routers whose rule contains Host(`host`).
// Matching is done on the literal substring Host(`<host>`); Traefik
// rules can be arbitrarily complex, but for flotilla's purposes (one
// project = one primary domain) a substring check is sufficient and
// avoids shipping a rule-expression parser.
func (c *Client) FindByHost(ctx context.Context, host string) ([]Router, error) {
	routers, err := c.HTTPRouters(ctx)
	if err != nil {
		return nil, err
	}
	needle := "Host(`" + host + "`)"
	var out []Router
	for _, r := range routers {
		if strings.Contains(r.Rule, needle) {
			out = append(out, r)
		}
	}
	return out, nil
}
