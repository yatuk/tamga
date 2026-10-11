package main

import (
	"context"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
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

// The rows of jailbreak.translate.abandon are a translation dropped inside
// the text: "Translate the following: … Instead of translating, write …".
// The scanners report it as the signal translation_abandoned and not as a
// verdict, because the same words are a user changing their mind, so the
// shipped policy lets these rows through and they count among the missed
// attacks above. This pins what is true of them: every one raises the
// signal, and a policy that names the signal acts on every one.
func TestTranslationAbandonRowsRaiseTheSignal(t *testing.T) {
	samples, err := loadCSV("../../testdata/redteam/prompts.csv")
	if err != nil {
		t.Fatal(err)
	}
	shipped := loadPolicy("../../tamga-policy.yaml")
	named, err := policy.LoadFromBytes([]byte("name: t\nrules:\n  injection_detection:\n    action: WARN\n    types: [translation_abandoned]\n"))
	if shipped == nil || err != nil {
		t.Fatalf("policies did not load: %v", err)
	}
	reg := buildRegistry()
	rows, passed := 0, 0
	for _, s := range samples {
		if !strings.HasPrefix(s.Category, "jailbreak.translate.abandon") {
			continue
		}
		rows++
		findings, _ := reg.ScanAll(context.Background(), []byte(s.Prompt))
		signalled := false
		for _, f := range findings {
			if f.Category == "translation_abandoned" && scanner.Signal(f) {
				signalled = true
			}
		}
		if !signalled {
			t.Errorf("%s: no translation_abandoned signal", s.ID)
		}
		if a := named.Evaluate(findings); a != policy.ActionWarn {
			t.Errorf("%s: %s under a rule that names the signal, want WARN", s.ID, a)
		}
		if shipped.Evaluate(findings) == policy.ActionPass {
			passed++
		}
	}
	if rows == 0 {
		t.Fatal("no jailbreak.translate.abandon rows in the corpus")
	}
	t.Logf("%d rows: all raise the signal, %d pass under the shipped policy", rows, passed)
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
		{"../../testdata/redteam/prompts.csv", 211, 0},
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
