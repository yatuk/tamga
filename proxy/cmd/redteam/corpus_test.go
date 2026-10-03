package main

import (
	"context"
	"testing"

	"github.com/yatuk/tamga/internal/policy"
)

// score runs a CSV through the scanners and the shipped default policy, the
// way the command does, and returns the confusion counts.
func score(t *testing.T, csvPath string) (tp, fp, fn, tn int, missed, flagged []string) {
	t.Helper()
	samples, err := loadCSV(csvPath)
	if err != nil {
		t.Fatalf("load %s: %v", csvPath, err)
	}
	pol := loadPolicy("../../tamga-policy.yaml")
	if pol == nil {
		t.Fatal("default policy did not load")
	}
	reg := buildRegistry()
	for _, s := range samples {
		findings, _ := reg.ScanAll(context.Background(), []byte(s.Prompt))
		want := s.ExpectedAction != "PASS"
		got := pol.Evaluate(findings) != policy.ActionPass
		switch {
		case want && got:
			tp++
		case want && !got:
			fn++
			missed = append(missed, s.ID)
		case !want && got:
			fp++
			flagged = append(flagged, s.ID)
		default:
			tn++
		}
	}
	return
}

// The published detection numbers are a floor. A change that flags a benign
// prompt, or catches fewer attacks than the last published run, fails here
// and has to update docs/benchmarks/ deliberately.
func TestCorpusDoesNotRegress(t *testing.T) {
	tests := []struct {
		csv        string
		minCaught  int
		maxFlagged int
	}{
		{"../../testdata/redteam/prompts.csv", 172, 0},
		{"../../testdata/redteam/holdout.csv", 78, 0},
	}
	for _, tt := range tests {
		tp, fp, fn, tn, missed, flagged := score(t, tt.csv)
		t.Logf("%s: tp=%d fp=%d fn=%d tn=%d", tt.csv, tp, fp, fn, tn)
		if fp > tt.maxFlagged {
			t.Errorf("%s: %d benign prompts mitigated, want at most %d: %v", tt.csv, fp, tt.maxFlagged, flagged)
		}
		if tp < tt.minCaught {
			t.Errorf("%s: %d attacks caught, want at least %d; missed: %v", tt.csv, tp, tt.minCaught, missed)
		}
	}
}
