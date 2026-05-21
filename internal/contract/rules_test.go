package contract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DmitriyKurilenko/flotilla/internal/compose"
	"github.com/DmitriyKurilenko/flotilla/internal/project"
)

// findResult returns the Result for ruleID from a Run() slice.
func runRule(t *testing.T, ruleID string, ctx Context) Result {
	t.Helper()
	for _, res := range Run(ctx) {
		if res.Rule.ID() == ruleID {
			return res
		}
	}
	t.Fatalf("rule %s not found in Run() output", ruleID)
	return Result{}
}

func assertVerdict(t *testing.T, ruleID string, ctx Context, want Verdict) {
	t.Helper()
	res := runRule(t, ruleID, ctx)
	if res.Verdict != want {
		t.Errorf("%s verdict = %s, want %s (msg: %s)", ruleID, res.Verdict, want, res.Message)
	}
}

// ─── L001 ───────────────────────────────────────────────────────────

func TestL001_ValidYAMLAndLoadedCompose(t *testing.T) {
	ctx := Context{
		RawCompose: []byte("services:\n  web:\n    image: nginx\n"),
		Compose:    &compose.File{Services: map[string]compose.Service{"web": {Name: "web"}}},
	}
	assertVerdict(t, "L001", ctx, Pass)
}

func TestL001_InvalidYAML(t *testing.T) {
	ctx := Context{RawCompose: []byte("services:\n  web:\n bad: indent\n")}
	assertVerdict(t, "L001", ctx, Fail)
}

func TestL001_ComposeNilEvenIfYAMLValid(t *testing.T) {
	ctx := Context{RawCompose: []byte("services:\n  web:\n    image: nginx\n"), Compose: nil}
	assertVerdict(t, "L001", ctx, Fail)
}

// ─── L002 ───────────────────────────────────────────────────────────

func TestL002_EnabledServiceProperlyAttached(t *testing.T) {
	ctx := Context{Compose: &compose.File{Services: map[string]compose.Service{
		"web": {
			Name:     "web",
			Labels:   map[string]string{"traefik.enable": "true", "traefik.docker.network": "proxy"},
			Networks: []string{"proxy", "backend"},
		},
	}}}
	assertVerdict(t, "L002", ctx, Pass)
}

func TestL002_MissingDockerNetworkLabel(t *testing.T) {
	ctx := Context{Compose: &compose.File{Services: map[string]compose.Service{
		"web": {
			Name:     "web",
			Labels:   map[string]string{"traefik.enable": "true"},
			Networks: []string{"proxy"},
		},
	}}}
	assertVerdict(t, "L002", ctx, Fail)
}

func TestL002_NotAttachedToProxyNetwork(t *testing.T) {
	ctx := Context{Compose: &compose.File{Services: map[string]compose.Service{
		"web": {
			Name:     "web",
			Labels:   map[string]string{"traefik.enable": "true", "traefik.docker.network": "proxy"},
			Networks: []string{"backend"},
		},
	}}}
	assertVerdict(t, "L002", ctx, Fail)
}

func TestL002_DisabledServiceIgnored(t *testing.T) {
	ctx := Context{Compose: &compose.File{Services: map[string]compose.Service{
		"celery": {Name: "celery", Labels: map[string]string{"traefik.enable": "false"}},
		"db":     {Name: "db"},
	}}}
	assertVerdict(t, "L002", ctx, Pass)
}

// ─── L003 ───────────────────────────────────────────────────────────

func TestL003_SingleRouterNoPriorityNeeded(t *testing.T) {
	ctx := Context{Compose: &compose.File{Services: map[string]compose.Service{
		"web": {Name: "web", Labels: map[string]string{
			"traefik.http.routers.web.rule": "Host(`example.com`)",
		}},
	}}}
	assertVerdict(t, "L003", ctx, Pass)
}

func TestL003_MultipleRoutersSameHostWithPriorities(t *testing.T) {
	ctx := Context{Compose: &compose.File{Services: map[string]compose.Service{
		"web": {Name: "web", Labels: map[string]string{
			"traefik.http.routers.api.rule":     "Host(`example.com`) && PathPrefix(`/api`)",
			"traefik.http.routers.api.priority": "100",
			"traefik.http.routers.spa.rule":     "Host(`example.com`)",
			"traefik.http.routers.spa.priority": "1",
		}},
	}}}
	assertVerdict(t, "L003", ctx, Pass)
}

func TestL003_MultipleRoutersSameHostMissingPriority(t *testing.T) {
	ctx := Context{Compose: &compose.File{Services: map[string]compose.Service{
		"web": {Name: "web", Labels: map[string]string{
			"traefik.http.routers.api.rule":     "Host(`example.com`) && PathPrefix(`/api`)",
			"traefik.http.routers.api.priority": "100",
			"traefik.http.routers.spa.rule":     "Host(`example.com`)",
			// spa has no priority → ambiguous
		}},
	}}}
	assertVerdict(t, "L003", ctx, Fail)
}

// ─── L004 ───────────────────────────────────────────────────────────

func TestL004_NoHealthcheckInDockerfile(t *testing.T) {
	dir := t.TempDir()
	df := filepath.Join(dir, "Dockerfile")
	_ = os.WriteFile(df, []byte("FROM nginx\nCOPY . /app\n"), 0o644)
	ctx := Context{DockerfilePaths: []string{df}}
	assertVerdict(t, "L004", ctx, Pass)
}

func TestL004_HealthcheckInDockerfileWarns(t *testing.T) {
	dir := t.TempDir()
	df := filepath.Join(dir, "Dockerfile")
	_ = os.WriteFile(df, []byte("FROM nginx\nHEALTHCHECK CMD curl -f http://localhost/ || exit 1\n"), 0o644)
	ctx := Context{DockerfilePaths: []string{df}}
	assertVerdict(t, "L004", ctx, Warn)
}

func TestL004_MissingDockerfileIgnored(t *testing.T) {
	ctx := Context{DockerfilePaths: []string{"/nonexistent/Dockerfile"}}
	assertVerdict(t, "L004", ctx, Pass)
}

// ─── L005 ───────────────────────────────────────────────────────────

func TestL005_HTTPSEntrypointSatisfiesByConstruction(t *testing.T) {
	ctx := Context{
		Project: &project.File{Domain: "example.com", HTTPSEntrypoint: &project.HTTPSEntrypoint{Service: "web"}},
		Compose: &compose.File{Services: map[string]compose.Service{"web": {Name: "web"}}},
	}
	assertVerdict(t, "L005", ctx, Pass)
}

func TestL005_ManualLabelMatchesDomain(t *testing.T) {
	ctx := Context{
		Project: &project.File{Domain: "example.com"},
		Compose: &compose.File{Services: map[string]compose.Service{
			"web": {Name: "web", Labels: map[string]string{
				"traefik.http.routers.web.rule": "Host(`example.com`)",
			}},
		}},
	}
	assertVerdict(t, "L005", ctx, Pass)
}

func TestL005_DomainDrift(t *testing.T) {
	ctx := Context{
		Project: &project.File{Domain: "new.example.com"},
		Compose: &compose.File{Services: map[string]compose.Service{
			"web": {Name: "web", Labels: map[string]string{
				"traefik.http.routers.web.rule": "Host(`old.example.com`)",
			}},
		}},
	}
	assertVerdict(t, "L005", ctx, Fail)
}

// ─── L006 ───────────────────────────────────────────────────────────

func TestL006_HealthcheckOn127001(t *testing.T) {
	ctx := Context{Compose: &compose.File{Services: map[string]compose.Service{
		"web": {Name: "web", Healthcheck: &compose.Healthcheck{
			Test: []string{"CMD-SHELL", "wget -qO- http://127.0.0.1/ || exit 1"},
		}},
	}}}
	assertVerdict(t, "L006", ctx, Pass)
}

func TestL006_HealthcheckOnLocalhostWarns(t *testing.T) {
	ctx := Context{Compose: &compose.File{Services: map[string]compose.Service{
		"web": {Name: "web", Healthcheck: &compose.Healthcheck{
			Test: []string{"CMD-SHELL", "curl -sf http://localhost:8000/healthz || exit 1"},
		}},
	}}}
	assertVerdict(t, "L006", ctx, Warn)
}

func TestL006_DisabledHealthcheckIgnored(t *testing.T) {
	ctx := Context{Compose: &compose.File{Services: map[string]compose.Service{
		"web": {Name: "web", Healthcheck: &compose.Healthcheck{
			Test:    []string{"CMD-SHELL", "curl http://localhost/"},
			Disable: true,
		}},
	}}}
	assertVerdict(t, "L006", ctx, Pass)
}

// ─── L007 ───────────────────────────────────────────────────────────

func TestL007_AllRequiredDocumented(t *testing.T) {
	dir := t.TempDir()
	example := filepath.Join(dir, ".env.example")
	_ = os.WriteFile(example, []byte("DB_PASSWORD=changeme\nSECRET_KEY=changeme\n"), 0o644)

	ctx := Context{
		RawCompose: []byte("services:\n  web:\n    environment:\n" +
			"      - DB=${DB_PASSWORD}\n      - SK=${SECRET_KEY}\n      - OPT=${OPTIONAL:-x}\n"),
		EnvExamplePath: example,
	}
	assertVerdict(t, "L007", ctx, Pass)
}

func TestL007_UndocumentedRequiredVarFails(t *testing.T) {
	dir := t.TempDir()
	example := filepath.Join(dir, ".env.example")
	_ = os.WriteFile(example, []byte("DB_PASSWORD=changeme\n"), 0o644)

	ctx := Context{
		RawCompose: []byte("services:\n  web:\n    environment:\n" +
			"      - DB=${DB_PASSWORD}\n      - SK=${SECRET_KEY}\n"),
		EnvExamplePath: example,
	}
	assertVerdict(t, "L007", ctx, Fail)
}

func TestL007_NoRequiredVarsPasses(t *testing.T) {
	ctx := Context{
		RawCompose:     []byte("services:\n  web:\n    image: nginx\n"),
		EnvExamplePath: "/does/not/matter",
	}
	assertVerdict(t, "L007", ctx, Pass)
}

// ─── L008 ───────────────────────────────────────────────────────────

func TestL008_NoHTTPSEntrypointNoConflict(t *testing.T) {
	ctx := Context{
		Project: &project.File{Domain: "example.com"},
		Compose: &compose.File{Services: map[string]compose.Service{
			"web": {Name: "web", Labels: map[string]string{
				"traefik.http.routers.web.rule": "Host(`example.com`)",
			}},
		}},
	}
	assertVerdict(t, "L008", ctx, Pass)
}

func TestL008_HTTPSEntrypointCleanService(t *testing.T) {
	ctx := Context{
		Project: &project.File{HTTPSEntrypoint: &project.HTTPSEntrypoint{Service: "web"}},
		Compose: &compose.File{Services: map[string]compose.Service{
			"web": {Name: "web"}, // no traefik.* labels
		}},
	}
	assertVerdict(t, "L008", ctx, Pass)
}

func TestL008_HTTPSEntrypointWithManualLabelsConflicts(t *testing.T) {
	ctx := Context{
		Project: &project.File{HTTPSEntrypoint: &project.HTTPSEntrypoint{Service: "web"}},
		Compose: &compose.File{Services: map[string]compose.Service{
			"web": {Name: "web", Labels: map[string]string{
				"traefik.enable":                "true",
				"traefik.http.routers.web.rule": "Host(`example.com`)",
			}},
		}},
	}
	assertVerdict(t, "L008", ctx, Fail)
}

func TestL008_HTTPSEntrypointPointsAtMissingService(t *testing.T) {
	ctx := Context{
		Project: &project.File{HTTPSEntrypoint: &project.HTTPSEntrypoint{Service: "nope"}},
		Compose: &compose.File{Services: map[string]compose.Service{"web": {Name: "web"}}},
	}
	assertVerdict(t, "L008", ctx, Fail)
}
