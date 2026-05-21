package contract

import "testing"

func TestVerdictString(t *testing.T) {
	tests := []struct {
		v    Verdict
		want string
	}{
		{Pass, "pass"},
		{Warn, "warn"},
		{Fail, "fail"},
		{Verdict(99), "?"},
	}
	for _, tt := range tests {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("Verdict(%d).String() = %q, want %q", tt.v, got, tt.want)
		}
	}
}

func TestHasFailures(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		want    bool
	}{
		{"empty", nil, false},
		{"all pass", []Result{{Verdict: Pass}, {Verdict: Pass}}, false},
		{"warn only", []Result{{Verdict: Warn}, {Verdict: Pass}}, false},
		{"one fail", []Result{{Verdict: Pass}, {Verdict: Fail}, {Verdict: Warn}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasFailures(tt.results); got != tt.want {
				t.Errorf("HasFailures = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegistry_IsClosedV01Set(t *testing.T) {
	reg := Registry()
	wantIDs := []string{"L001", "L002", "L003", "L004", "L005", "L006", "L007", "L008"}
	if len(reg) != len(wantIDs) {
		t.Fatalf("Registry has %d rules, want %d", len(reg), len(wantIDs))
	}
	for i, r := range reg {
		if r.ID() != wantIDs[i] {
			t.Errorf("Registry[%d].ID() = %q, want %q (order matters)", i, r.ID(), wantIDs[i])
		}
		if r.Title() == "" {
			t.Errorf("rule %s has empty Title()", r.ID())
		}
	}
}

func TestRun_ReturnsOneResultPerRule(t *testing.T) {
	// With an empty Context every rule should still produce a Result
	// (Pass for the «nothing to check» short-circuits) and never panic.
	results := Run(Context{})
	if len(results) != len(Registry()) {
		t.Fatalf("Run returned %d results, want %d", len(results), len(Registry()))
	}
	for _, res := range results {
		if res.Rule == nil {
			t.Errorf("result has nil Rule: %+v", res)
		}
	}
}
