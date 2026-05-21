package contract

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/DmitriyKurilenko/flotilla/internal/compose"
	"github.com/DmitriyKurilenko/flotilla/internal/envcheck"
)

// This file holds the closed v0.1 rule set (L001-L008). Each rule is a
// tiny struct implementing Rule. Registry() returns them in declared
// order. See docs/ARCHITECTURE.md §7.
//
// Adding a rule here requires: a documented incident in CHANGELOG.md, a
// row in ARCHITECTURE.md §7, and positive+negative tests.

// Registry returns the closed v0.1 rule set in declared order.
func Registry() []Rule {
	return []Rule{
		l001{}, l002{}, l003{}, l004{}, l005{}, l006{}, l007{}, l008{},
	}
}

// pass / warn / fail are small constructors that keep Check bodies terse.
func pass(r Rule) Result { return Result{Rule: r, Verdict: Pass} }

func warn(r Rule, msg, hint string) Result {
	return Result{Rule: r, Verdict: Warn, Message: msg, Hint: hint}
}

func fail(r Rule, msg, hint string) Result {
	return Result{Rule: r, Verdict: Fail, Message: msg, Hint: hint}
}

// ─── L001 — Compose syntax ──────────────────────────────────────────

type l001 struct{}

func (l001) ID() string    { return "L001" }
func (l001) Title() string { return "Compose syntax" }

func (r l001) Check(ctx Context) Result {
	if err := compose.IsValidYAML(ctx.RawCompose); err != nil {
		return fail(r,
			fmt.Sprintf("compose.yml is not valid YAML: %v", err),
			"Fix the YAML structure; run `docker compose config` locally to see the parser's view.")
	}
	if ctx.Compose == nil {
		return fail(r,
			"`docker compose config` rejected compose.yml",
			"Run `docker compose -f compose.yml config` locally to see the full error.")
	}
	return pass(r)
}

// ─── L002 — Traefik network attachment ──────────────────────────────

type l002 struct{}

func (l002) ID() string    { return "L002" }
func (l002) Title() string { return "Traefik network attachment" }

func (r l002) Check(ctx Context) Result {
	if ctx.Compose == nil {
		return pass(r) // L001 already failed; nothing to check.
	}
	names := sortedServiceNames(ctx.Compose)
	for _, name := range names {
		svc := ctx.Compose.Services[name]
		if !traefikEnabled(svc.Labels) {
			continue
		}
		if net := traefikDockerNetwork(svc.Labels); net != "proxy" {
			return fail(r,
				fmt.Sprintf("service %q has traefik.enable=true but traefik.docker.network is %q (want \"proxy\")", name, net),
				"Add label: traefik.docker.network=proxy")
		}
		if !containsStr(svc.Networks, "proxy") {
			return fail(r,
				fmt.Sprintf("service %q is Traefik-enabled but not attached to the `proxy` network", name),
				"Add `proxy` to the service's networks: and declare it as an external network.")
		}
	}
	return pass(r)
}

// ─── L003 — Router priority ─────────────────────────────────────────

type l003 struct{}

func (l003) ID() string    { return "L003" }
func (l003) Title() string { return "Router priority" }

func (r l003) Check(ctx Context) Result {
	if ctx.Compose == nil {
		return pass(r)
	}
	// Collect every router across all services, grouped by the set of
	// hosts its rule targets. If two routers share a host, each must
	// have an explicit priority.
	type routerRef struct {
		service string
		ri      routerInfo
	}
	hostRouters := map[string][]routerRef{}
	for _, name := range sortedServiceNames(ctx.Compose) {
		svc := ctx.Compose.Services[name]
		for _, ri := range extractRouters(svc.Labels) {
			for _, h := range extractHosts(ri.rule) {
				hostRouters[h] = append(hostRouters[h], routerRef{service: name, ri: ri})
			}
		}
	}
	hosts := make([]string, 0, len(hostRouters))
	for h := range hostRouters {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	for _, h := range hosts {
		refs := hostRouters[h]
		if len(refs) < 2 {
			continue
		}
		for _, ref := range refs {
			if strings.TrimSpace(ref.ri.priority) == "" {
				return fail(r,
					fmt.Sprintf("host %q is served by %d routers but router %q (service %q) has no explicit priority", h, len(refs), ref.ri.name, ref.service),
					"Set traefik.http.routers.<name>.priority on every router that shares a host, so routing is deterministic.")
			}
		}
	}
	return pass(r)
}

// ─── L004 — No HEALTHCHECK in Dockerfile ────────────────────────────

type l004 struct{}

func (l004) ID() string    { return "L004" }
func (l004) Title() string { return "No HEALTHCHECK in Dockerfile" }

var dockerfileHealthcheckRE = regexp.MustCompile(`(?im)^\s*HEALTHCHECK\b`)

func (r l004) Check(ctx Context) Result {
	var offenders []string
	for _, p := range ctx.DockerfilePaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue // missing/unreadable Dockerfile is not this rule's concern
		}
		if dockerfileHealthcheckRE.Match(data) {
			offenders = append(offenders, p)
		}
	}
	if len(offenders) > 0 {
		return warn(r,
			fmt.Sprintf("HEALTHCHECK directive found in: %s", strings.Join(offenders, ", ")),
			"Move healthchecks into compose.yml where they can be overridden or disabled. "+
				"An image-level HEALTHCHECK that fails makes Traefik filter the container's routers.")
	}
	return pass(r)
}

// ─── L005 — Domain matches Traefik labels ───────────────────────────

type l005 struct{}

func (l005) ID() string    { return "L005" }
func (l005) Title() string { return "Domain matches Traefik labels" }

func (r l005) Check(ctx Context) Result {
	if ctx.Project == nil {
		return pass(r)
	}
	// When https_entrypoint is set, flotilla generates a
	// Host(<domain>) router at deploy time (autocert). Nothing for the
	// operator to declare; this rule is satisfied by construction.
	if ctx.Project.HTTPSEntrypoint != nil {
		return pass(r)
	}
	if ctx.Compose == nil {
		return pass(r) // L001 handles the «no compose» case.
	}
	domain := ctx.Project.Domain
	for _, name := range sortedServiceNames(ctx.Compose) {
		svc := ctx.Compose.Services[name]
		for _, ri := range extractRouters(svc.Labels) {
			for _, h := range extractHosts(ri.rule) {
				if h == domain {
					return pass(r)
				}
			}
		}
	}
	return fail(r,
		fmt.Sprintf("no Traefik router rule contains Host(`%s`); project.yml domain and compose labels have drifted", domain),
		"Either add a router with Host(`"+domain+"`) to compose.yml, or set `https_entrypoint:` in project.yml and let flotilla generate the labels.")
}

// ─── L006 — No localhost healthchecks ───────────────────────────────

type l006 struct{}

func (l006) ID() string    { return "L006" }
func (l006) Title() string { return "No localhost healthchecks" }

func (r l006) Check(ctx Context) Result {
	if ctx.Compose == nil {
		return pass(r)
	}
	var offenders []string
	for _, name := range sortedServiceNames(ctx.Compose) {
		svc := ctx.Compose.Services[name]
		if svc.Healthcheck == nil || svc.Healthcheck.Disable {
			continue
		}
		joined := strings.ToLower(strings.Join(svc.Healthcheck.Test, " "))
		if strings.Contains(joined, "localhost") || strings.Contains(joined, "0.0.0.0") {
			offenders = append(offenders, name)
		}
	}
	if len(offenders) > 0 {
		sort.Strings(offenders)
		return warn(r,
			fmt.Sprintf("healthcheck(s) target localhost/0.0.0.0 in service(s): %s", strings.Join(offenders, ", ")),
			"Use 127.0.0.1 (busybox wget does not fall back IPv6→IPv4) or the project's domain with an explicit Host header. "+
				"localhost-based probes caused the crm_prvms/rent_django HTTPS outage.")
	}
	return pass(r)
}

// ─── L007 — Required env not in .env.example ────────────────────────

type l007 struct{}

func (l007) ID() string    { return "L007" }
func (l007) Title() string { return "Required env documented in .env.example" }

func (r l007) Check(ctx Context) Result {
	refs, err := envcheck.ScanCompose(ctx.RawCompose)
	if err != nil {
		return pass(r) // L001 owns «compose unparseable».
	}
	var required []string
	for _, ref := range refs {
		if !ref.HasDefault {
			required = append(required, ref.Name)
		}
	}
	if len(required) == 0 {
		return pass(r)
	}
	example, err := envcheck.LoadEnv(ctx.EnvExamplePath)
	if err != nil {
		return fail(r,
			fmt.Sprintf("could not read .env.example: %v", err),
			"Create .env.example and list every required variable (value can be a placeholder).")
	}
	var undocumented []string
	for _, name := range required {
		if _, ok := example[name]; !ok {
			undocumented = append(undocumented, name)
		}
	}
	if len(undocumented) > 0 {
		sort.Strings(undocumented)
		return fail(r,
			fmt.Sprintf("required vars missing from .env.example: %s", strings.Join(undocumented, ", ")),
			"Add each variable to .env.example so a fresh-server operator knows it exists.")
	}
	return pass(r)
}

// ─── L008 — https_entrypoint vs manual Traefik labels ───────────────

type l008 struct{}

func (l008) ID() string    { return "L008" }
func (l008) Title() string { return "https_entrypoint vs manual Traefik labels" }

func (r l008) Check(ctx Context) Result {
	if ctx.Project == nil || ctx.Project.HTTPSEntrypoint == nil {
		return pass(r) // no auto-cert requested → no conflict possible
	}
	if ctx.Compose == nil {
		return pass(r)
	}
	target := ctx.Project.HTTPSEntrypoint.Service
	svc, ok := ctx.Compose.Services[target]
	if !ok {
		return fail(r,
			fmt.Sprintf("project.yml https_entrypoint points at compose service %q, which does not exist", target),
			"Set https_entrypoint to a service name that exists in compose.yml.")
	}
	if hasTraefikLabel(svc.Labels) {
		return fail(r,
			fmt.Sprintf("service %q has both https_entrypoint (project.yml) and hand-written traefik.* labels (compose.yml)", target),
			"Pick one ownership model: remove the traefik.* labels and let flotilla generate them, "+
				"or remove https_entrypoint and own the labels in compose.")
	}
	return pass(r)
}

// ─── shared helpers ─────────────────────────────────────────────────

func sortedServiceNames(c *compose.File) []string {
	names := make([]string, 0, len(c.Services))
	for n := range c.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func containsStr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
