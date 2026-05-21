package deploy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DmitriyKurilenko/flotilla/internal/compose"
)

func TestStepString(t *testing.T) {
	tests := []struct {
		s    Step
		want string
	}{
		{StepAutocert, "autocert"},
		{StepParse, "parse"},
		{StepLint, "lint"},
		{StepEnv, "env"},
		{StepSymlinks, "symlinks"},
		{StepComposeUp, "compose-up"},
		{StepWaitRunning, "wait-running"},
		{StepTraefikDiscover, "traefik-discover"},
		{StepSmoke, "smoke"},
		{Step(99), "?"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("Step(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestLastDryStep(t *testing.T) {
	if LastDryStep != StepEnv {
		t.Errorf("LastDryStep = %v, expected StepEnv per ARCHITECTURE.md §5.1", LastDryStep)
	}
}

// stubComposeLoad swaps the docker seam for the duration of a test.
func stubComposeLoad(t *testing.T, fn func() (*compose.File, error)) {
	t.Helper()
	orig := composeLoad
	composeLoad = func(_ context.Context, _, _ string) (*compose.File, error) { return fn() }
	t.Cleanup(func() { composeLoad = orig })
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newProject lays out a minimal project dir and returns its path.
func newProject(t *testing.T, projectYML, composeYML, env, envExample string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "project.yml"), projectYML)
	if composeYML != "" {
		writeFile(t, filepath.Join(dir, "compose.yml"), composeYML)
	}
	if env != "" {
		writeFile(t, filepath.Join(dir, ".env"), env)
	}
	if envExample != "" {
		writeFile(t, filepath.Join(dir, ".env.example"), envExample)
	}
	return dir
}

func TestRun_ParseFailure(t *testing.T) {
	dir := t.TempDir() // no project.yml
	out := Run(context.Background(), Options{ProjectDir: dir, Dry: true})
	if out.Err == nil {
		t.Fatal("expected parse error")
	}
	if out.LastStep != StepParse {
		t.Errorf("LastStep = %v, want StepParse", out.LastStep)
	}
}

func TestRun_DrySuccess_HTTPSEntrypoint(t *testing.T) {
	// Authored compose has NO traefik labels; project uses
	// https_entrypoint, so L005/L008 pass and autocert renders cleanly.
	dir := newProject(t,
		"version: 1\nname: demo\ndomain: demo.example.com\npath: "+"/opt/demo"+"\nhttps_entrypoint: web\n",
		"services:\n  web:\n    image: nginx\n",
		"",
		"")

	stubComposeLoad(t, func() (*compose.File, error) {
		return &compose.File{Services: map[string]compose.Service{
			"web": {Name: "web", Expose: []int{8000}},
		}}, nil
	})

	out := Run(context.Background(), Options{ProjectDir: dir, Dry: true})
	if out.Err != nil {
		t.Fatalf("dry run should pass, got err at %v: %v", out.LastStep, out.Err)
	}
	if out.LastStep != LastDryStep {
		t.Errorf("LastStep = %v, want %v", out.LastStep, LastDryStep)
	}
	if !out.Dry {
		t.Error("Outcome.Dry should be true")
	}
	// Dry must not write the override to disk.
	if _, err := os.Stat(filepath.Join(dir, ".flotilla", "compose.override.yml")); !os.IsNotExist(err) {
		t.Errorf("dry run must not write compose.override.yml (stat err=%v)", err)
	}
}

func TestRun_DrySuccess_ManualLabels(t *testing.T) {
	dir := newProject(t,
		"version: 1\nname: demo\ndomain: demo.example.com\npath: /opt/demo\n",
		"services:\n  web:\n    image: nginx\n", "", "")

	stubComposeLoad(t, func() (*compose.File, error) {
		return &compose.File{Services: map[string]compose.Service{
			"web": {
				Name:     "web",
				Networks: []string{"proxy"},
				Labels: map[string]string{
					"traefik.enable":                "true",
					"traefik.docker.network":        "proxy",
					"traefik.http.routers.web.rule": "Host(`demo.example.com`)",
				},
			},
		}}, nil
	})

	out := Run(context.Background(), Options{ProjectDir: dir, Dry: true})
	if out.Err != nil {
		t.Fatalf("dry run should pass, got err at %v: %v", out.LastStep, out.Err)
	}
	if out.LastStep != LastDryStep {
		t.Errorf("LastStep = %v, want %v", out.LastStep, LastDryStep)
	}
}

func TestRun_LintFailure_DomainDrift(t *testing.T) {
	dir := newProject(t,
		"version: 1\nname: demo\ndomain: new.example.com\npath: /opt/demo\n",
		"services:\n  web:\n    image: nginx\n", "", "")

	stubComposeLoad(t, func() (*compose.File, error) {
		return &compose.File{Services: map[string]compose.Service{
			"web": {Name: "web", Labels: map[string]string{
				"traefik.http.routers.web.rule": "Host(`old.example.com`)",
			}},
		}}, nil
	})

	out := Run(context.Background(), Options{ProjectDir: dir, Dry: true})
	if out.Err == nil {
		t.Fatal("expected lint failure (L005 domain drift)")
	}
	if out.LastStep != StepLint {
		t.Errorf("LastStep = %v, want StepLint", out.LastStep)
	}
}

func TestRun_LintFailure_ComposeUnparseable(t *testing.T) {
	dir := newProject(t,
		"version: 1\nname: demo\ndomain: demo.example.com\npath: /opt/demo\n",
		"this: is: not: valid: compose", "", "")

	// composeLoad returns error → authoredCompose nil → L001 fails.
	stubComposeLoad(t, func() (*compose.File, error) {
		return nil, context.DeadlineExceeded
	})

	out := Run(context.Background(), Options{ProjectDir: dir, Dry: true})
	if out.Err == nil {
		t.Fatal("expected lint failure (L001)")
	}
	if out.LastStep != StepLint {
		t.Errorf("LastStep = %v, want StepLint", out.LastStep)
	}
}

func TestRun_EnvFailure_MissingRequired(t *testing.T) {
	dir := newProject(t,
		"version: 1\nname: demo\ndomain: demo.example.com\npath: /opt/demo\n",
		"services:\n  web:\n    image: nginx\n    environment:\n      - SK=${SECRET_KEY}\n",
		"OTHER=x\n",             // .env present but SECRET_KEY missing
		"SECRET_KEY=changeme\n", // documented in .env.example so L007 passes
	)

	stubComposeLoad(t, func() (*compose.File, error) {
		return &compose.File{Services: map[string]compose.Service{
			"web": {
				Name:     "web",
				Networks: []string{"proxy"},
				Labels: map[string]string{
					"traefik.enable":                "true",
					"traefik.docker.network":        "proxy",
					"traefik.http.routers.web.rule": "Host(`demo.example.com`)",
				},
			},
		}}, nil
	})

	out := Run(context.Background(), Options{ProjectDir: dir, Dry: true})
	if out.Err == nil {
		t.Fatal("expected env failure (SECRET_KEY missing)")
	}
	if out.LastStep != StepEnv {
		t.Errorf("LastStep = %v, want StepEnv (err: %v)", out.LastStep, out.Err)
	}
}

func TestRun_AutocertPortNotInferable(t *testing.T) {
	dir := newProject(t,
		"version: 1\nname: demo\ndomain: demo.example.com\npath: /opt/demo\nhttps_entrypoint: web\n",
		"services:\n  web:\n    image: nginx\n", "", "")

	// Service has no expose/ports and project gave no explicit port →
	// autocert.Render must fail at StepAutocert.
	stubComposeLoad(t, func() (*compose.File, error) {
		return &compose.File{Services: map[string]compose.Service{
			"web": {Name: "web"},
		}}, nil
	})

	out := Run(context.Background(), Options{ProjectDir: dir, Dry: true})
	if out.Err == nil {
		t.Fatal("expected autocert failure (port not inferable)")
	}
	if out.LastStep != StepAutocert {
		t.Errorf("LastStep = %v, want StepAutocert", out.LastStep)
	}
}
