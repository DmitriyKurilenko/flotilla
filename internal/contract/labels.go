package contract

import (
	"regexp"
	"strings"
)

// Helpers for parsing Traefik labels out of a compose service's
// `labels:` map. We extract only the fields the v0.1 lint rules need:
//
//   - which router names exist for this service
//   - each router's rule string (the `Host(...)` text)
//   - each router's explicit priority (if any)
//   - whether the service has `traefik.enable=true`
//   - whether the service has `traefik.docker.network=...`
//
// We do not validate the rules themselves; Traefik is the canonical
// validator. We only need enough structure to enforce the lint rules.

// routerKeyRE captures the router name from a label key like
// "traefik.http.routers.<name>.<attr>".
var routerKeyRE = regexp.MustCompile(`^traefik\.http\.routers\.([^.]+)\.([a-zA-Z0-9_.]+)$`)

// routerInfo is the parsed subset of one router's labels.
type routerInfo struct {
	name     string
	rule     string // empty if not set
	priority string // empty if not set; we keep it as a string for L003
}

// extractRouters walks a label map and returns one routerInfo per router
// name discovered.
func extractRouters(labels map[string]string) []routerInfo {
	if len(labels) == 0 {
		return nil
	}
	byName := make(map[string]*routerInfo)
	for k, v := range labels {
		m := routerKeyRE.FindStringSubmatch(k)
		if m == nil {
			continue
		}
		name, attr := m[1], m[2]
		ri, ok := byName[name]
		if !ok {
			ri = &routerInfo{name: name}
			byName[name] = ri
		}
		switch attr {
		case "rule":
			ri.rule = v
		case "priority":
			ri.priority = v
		}
	}
	out := make([]routerInfo, 0, len(byName))
	for _, ri := range byName {
		out = append(out, *ri)
	}
	return out
}

// traefikEnabled returns true if labels contain `traefik.enable=true`
// (case-insensitive on the value, per Traefik's own parsing).
func traefikEnabled(labels map[string]string) bool {
	v, ok := labels["traefik.enable"]
	return ok && strings.EqualFold(strings.TrimSpace(v), "true")
}

// traefikDockerNetwork returns the value of `traefik.docker.network`,
// or "" if not set.
func traefikDockerNetwork(labels map[string]string) string {
	return strings.TrimSpace(labels["traefik.docker.network"])
}

// hasTraefikLabel reports whether the service has ANY label whose key
// begins with `traefik.`. Used by L008 to detect manual labels.
func hasTraefikLabel(labels map[string]string) bool {
	for k := range labels {
		if strings.HasPrefix(k, "traefik.") {
			return true
		}
	}
	return false
}

// hostRE matches the Traefik `Host(`example.com`)` rule fragment and
// captures the host string. Permissive about whitespace and quoting:
// Traefik itself accepts only backticks, but we tolerate both single
// and double quotes to be forgiving to operator typos that Traefik
// would have rejected at a different layer.
var hostRE = regexp.MustCompile("(?i)Host\\(\\s*[`'\"]([^`'\"]+)[`'\"]\\s*\\)")

// extractHosts returns every Host(...) hostname from a single Traefik
// rule string. A rule may contain multiple Host(...) clauses joined by
// `||`; we return them all.
func extractHosts(rule string) []string {
	if rule == "" {
		return nil
	}
	ms := hostRE.FindAllStringSubmatch(rule, -1)
	if len(ms) == 0 {
		return nil
	}
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m[1])
	}
	return out
}
