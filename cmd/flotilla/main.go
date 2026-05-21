// Command flotilla is the CLI described in docs/ARCHITECTURE.md §6.
// v0.1 ships two subcommands: status and deploy.
//
// install.sh handles the VPS bootstrap (Docker + Traefik + the proxy
// network); the CLI is invoked after install.sh has placed it in
// /usr/local/bin/flotilla.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/DmitriyKurilenko/flotilla/internal/deploy"
	"github.com/DmitriyKurilenko/flotilla/internal/discover"
	flog "github.com/DmitriyKurilenko/flotilla/internal/log"
	"github.com/DmitriyKurilenko/flotilla/internal/status"
)

// Build-time variables, set via -ldflags. See Makefile and .goreleaser.yml.
var (
	version = "dev"
	commit  = "unknown"
	date    = ""
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	code := run(ctx, os.Args[1:])
	os.Exit(code)
}

// run returns the process exit code: 0 ok, 1 failure, 2 usage error.
func run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return 2
	}

	switch args[0] {
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return 0
	case "-v", "--version":
		printVersion(os.Stdout)
		return 0
	case "status":
		return cmdStatus(ctx, args[1:])
	case "deploy":
		return cmdDeploy(ctx, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "flotilla: unknown command %q\n\n", args[0])
		printUsage(os.Stderr)
		return 2
	}
}

// ─── common flag wiring ──────────────────────────────────────────────

type commonFlags struct {
	all   bool
	quiet bool
	json  bool
}

func bindCommon(fs *flag.FlagSet) *commonFlags {
	c := &commonFlags{}
	fs.BoolVar(&c.all, "all", false, "operate on every project found at /opt/*/project.yml")
	fs.BoolVar(&c.quiet, "quiet", false, "errors only; suppress progress")
	fs.BoolVar(&c.quiet, "q", false, "shorthand for --quiet")
	fs.BoolVar(&c.json, "json", false, "machine-readable JSON output")
	return c
}

// splitArgs partitions args into flag-like (start with "-") and
// positional. The stdlib flag package stops parsing at the first
// positional, so `flotilla deploy /opt/foo --dry` would otherwise
// treat --dry as a path. Every flotilla flag is boolean (none consume
// a following value), so this partition is unambiguous and lets flags
// and the path appear in any order.
func splitArgs(args []string) (flagArgs, positional []string) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
		} else {
			positional = append(positional, a)
		}
	}
	return flagArgs, positional
}

// resolveTargets turns the positional path + --all into a list of
// project directories to act on.
func resolveTargets(positional []string, all bool) ([]string, error) {
	if all {
		if len(positional) > 0 {
			return nil, errors.New("--all takes no path argument")
		}
		found, err := discover.DiscoverDefault()
		if err != nil {
			return nil, fmt.Errorf("discover projects: %w", err)
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("no projects found at %s", discover.DefaultGlob)
		}
		dirs := make([]string, len(found))
		for i, f := range found {
			dirs[i] = f.Dir
		}
		return dirs, nil
	}

	path := "."
	if len(positional) == 1 {
		path = positional[0]
	} else if len(positional) > 1 {
		return nil, errors.New("expected at most one path argument")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", path, err)
	}
	return []string{abs}, nil
}

// ─── status ──────────────────────────────────────────────────────────

func cmdStatus(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: flotilla status [path] [--all] [--quiet] [--json]")
		fs.PrintDefaults()
	}
	common := bindCommon(fs)
	flagArgs, posArgs := splitArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}

	targets, err := resolveTargets(posArgs, common.all)
	if err != nil {
		fmt.Fprintln(os.Stderr, "flotilla status:", err)
		return 2
	}

	type item struct {
		Dir    string         `json:"dir"`
		Report *status.Report `json:"report,omitempty"`
		Error  string         `json:"error,omitempty"`
	}
	var items []item
	failed := false

	for _, dir := range targets {
		rep, err := status.Collect(ctx, dir, status.Options{})
		if err != nil {
			failed = true
			items = append(items, item{Dir: dir, Error: err.Error()})
			continue
		}
		items = append(items, item{Dir: dir, Report: rep})
	}

	if common.json {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(items)
	} else if !common.quiet {
		for _, it := range items {
			if it.Error != "" {
				fmt.Fprintf(os.Stderr, "✗ %s: %s\n", it.Dir, it.Error)
				continue
			}
			renderReport(os.Stdout, it.Report)
		}
	}

	if failed {
		return 1
	}
	return 0
}

func renderReport(w io.Writer, r *status.Report) {
	fmt.Fprintf(w, "%s · %s\n", r.Name, r.Domain)
	if r.Description != "" {
		fmt.Fprintf(w, "  %s\n", r.Description)
	}
	running := 0
	for _, c := range r.Containers {
		if c.State == "running" {
			running++
		}
	}
	fmt.Fprintf(w, "  containers     %d/%d running\n", running, len(r.Containers))
	for _, c := range r.Containers {
		h := c.Health
		if h == "" {
			h = "no-healthcheck"
		}
		fmt.Fprintf(w, "                 - %s: %s/%s\n", c.Service, c.State, h)
	}
	if len(r.TraefikRouters) > 0 {
		fmt.Fprintf(w, "  traefik        %d router(s): %v\n", len(r.TraefikRouters), r.TraefikRouters)
	} else {
		fmt.Fprintf(w, "  traefik        no router for %s\n", r.Domain)
	}
	if r.CertExpiresAt != "" {
		fmt.Fprintf(w, "  cert           %s, expires %s\n", r.CertSubject, r.CertExpiresAt)
	} else {
		fmt.Fprintf(w, "  cert           none in acme.json\n")
	}
	if r.HTTPStatus != 0 {
		fmt.Fprintf(w, "  https          GET https://%s/ → %d (%dms)\n", r.Domain, r.HTTPStatus, r.HTTPLatencyMS)
	} else {
		fmt.Fprintf(w, "  https          GET https://%s/ → unreachable (%dms)\n", r.Domain, r.HTTPLatencyMS)
	}
	if r.LastDeployAt != "" {
		fmt.Fprintf(w, "  last-deploy    sha=%s at %s\n", r.LastDeploySHA, r.LastDeployAt)
	} else {
		fmt.Fprintf(w, "  last-deploy    never (no .flotilla/state.json)\n")
	}
}

// ─── deploy ──────────────────────────────────────────────────────────

func cmdDeploy(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: flotilla deploy [path] [--all] [--keep-going] [--dry] [--quiet] [--json]")
		fs.PrintDefaults()
	}
	common := bindCommon(fs)
	var dry, keepGoing bool
	fs.BoolVar(&dry, "dry", false, "validation only (pipeline steps 0-3); no docker, no fs writes")
	fs.BoolVar(&keepGoing, "keep-going", false, "with --all: continue after a project fails")
	flagArgs, posArgs := splitArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}

	targets, err := resolveTargets(posArgs, common.all)
	if err != nil {
		fmt.Fprintln(os.Stderr, "flotilla deploy:", err)
		return 2
	}

	logger := flog.New(os.Stderr, flog.Options{Quiet: common.quiet, JSON: common.json})

	type result struct {
		Dir      string `json:"dir"`
		LastStep string `json:"last_step"`
		Dry      bool   `json:"dry"`
		OK       bool   `json:"ok"`
		Error    string `json:"error,omitempty"`
	}
	var results []result
	anyFailed := false

	for _, dir := range targets {
		out := deploy.Run(ctx, deploy.Options{
			ProjectDir: dir,
			Dry:        dry,
			Logger:     logger,
		})
		res := result{
			Dir:      dir,
			LastStep: out.LastStep.String(),
			Dry:      out.Dry,
			OK:       out.Err == nil,
		}
		if out.Err != nil {
			res.Error = out.Err.Error()
			anyFailed = true
		}
		results = append(results, res)

		if !common.json && !common.quiet {
			if out.Err != nil {
				fmt.Fprintf(os.Stderr, "✗ %s — failed at step %s: %v\n", dir, out.LastStep, out.Err)
			} else if out.Dry {
				fmt.Fprintf(os.Stdout, "✓ %s — dry run passed (validated through %s)\n", dir, out.LastStep)
			} else {
				fmt.Fprintf(os.Stdout, "✓ %s — deployed (through %s)\n", dir, out.LastStep)
			}
		}

		if out.Err != nil && len(targets) > 1 && !keepGoing {
			if !common.json {
				fmt.Fprintln(os.Stderr, "aborting (use --keep-going to continue past failures)")
			}
			break
		}
	}

	if common.json {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
	}

	if anyFailed {
		return 1
	}
	return 0
}

// ─── help / version ──────────────────────────────────────────────────

func printVersion(w io.Writer) {
	if date == "" {
		fmt.Fprintf(w, "flotilla %s (commit %s)\n", version, commit)
		return
	}
	fmt.Fprintf(w, "flotilla %s (commit %s, built %s)\n", version, commit, date)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `flotilla — shared-VPS multi-project Docker Compose orchestrator

Usage:
  flotilla status [path] [--all] [--quiet] [--json]
  flotilla deploy [path] [--all] [--keep-going] [--dry] [--quiet] [--json]
  flotilla --version
  flotilla --help

Arguments:
  path            project directory (contains project.yml); default "."

Flags:
  --all           operate on every /opt/*/project.yml
  --keep-going    with --all: don't stop at the first failed project
  --dry           deploy: validation only (no docker, no fs writes)
  --quiet, -q     errors only; suppress progress
  --json          machine-readable output
  --version       print version and exit
  --help          print this help and exit

See https://github.com/DmitriyKurilenko/flotilla and docs/ARCHITECTURE.md.`)
}
