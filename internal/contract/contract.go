// Package contract is the lint engine that enforces flotilla's
// conventions on downstream projects. See docs/ARCHITECTURE.md §7 for
// the closed list of v0.1 rules (L001-L008).
//
// New rules are added in waves; the Registry function is the single
// source of which rules are active.
package contract

import (
	"github.com/DmitriyKurilenko/flotilla/internal/compose"
	"github.com/DmitriyKurilenko/flotilla/internal/project"
)

// Verdict is the outcome of one rule against one project.
type Verdict int

const (
	// Pass means the rule is satisfied.
	Pass Verdict = iota
	// Warn is informational; deploy continues.
	Warn
	// Fail aborts the deploy pipeline at the lint step.
	Fail
)

// String returns "pass" / "warn" / "fail" for human output.
func (v Verdict) String() string {
	switch v {
	case Pass:
		return "pass"
	case Warn:
		return "warn"
	case Fail:
		return "fail"
	default:
		return "?"
	}
}

// Rule is one named, documented check.
type Rule interface {
	// ID is the "L00X" identifier shared with docs/ARCHITECTURE.md §7.
	ID() string
	// Title is a human-readable one-liner.
	Title() string
	// Check evaluates the rule against ctx and returns a Result.
	Check(ctx Context) Result
}

// Context is everything a rule needs to evaluate. Built once per lint
// invocation and shared across all rules.
type Context struct {
	Project         *project.File
	Compose         *compose.File
	RawCompose      []byte
	DockerfilePaths []string // candidate Dockerfile paths found in the project dir
	EnvPath         string   // absolute path to .env
	EnvExamplePath  string   // absolute path to .env.example
}

// Result is one rule's verdict on one project.
type Result struct {
	Rule    Rule
	Verdict Verdict
	Message string // shown to the operator
	Hint    string // optional remediation hint
}

// HasFailures returns true when any result in the slice is Fail. Warns
// don't count.
func HasFailures(results []Result) bool {
	for _, r := range results {
		if r.Verdict == Fail {
			return true
		}
	}
	return false
}

// Registry is implemented in rules.go — it returns the closed v0.1
// rule set (L001-L008) in declared order.

// Run executes every rule from Registry() against ctx and returns all
// results, preserving rule order. Empty registry returns nil.
func Run(ctx Context) []Result {
	rules := Registry()
	if len(rules) == 0 {
		return nil
	}
	out := make([]Result, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.Check(ctx))
	}
	return out
}
