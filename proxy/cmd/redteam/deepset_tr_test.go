package main

import (
	"strings"
	"testing"
)

// The second Turkish set: a machine translation of deepset/prompt-injections,
// read row by row against its source and corrected (see the header of each
// file and NOTICE). Unlike the tr_ files it was not written by the author of
// the rules, so its locked part is the more independent Turkish measure.
// deepset's own split is kept: its train split is the tuning part, its test
// split the locked part, under the same rule as tr_locked.csv.
const (
	trdTuningCSV = "../../testdata/redteam/trd_tuning.csv"
	trdLockedCSV = "../../testdata/redteam/trd_locked.csv"
)

// A floor like the other tuning sets: a change that catches fewer of its
// attacks or flags more of its benign rows fails here.
func TestDeepsetTurkishTuningSet(t *testing.T) {
	const minCaught, maxFlagged = 27, 0
	s := scoreTurkish(t, trdTuningCSV)
	t.Log(s.table())
	t.Logf("missed: %v", s.missed)
	t.Logf("flagged: %v", s.flagged)
	if s.tp < minCaught {
		t.Errorf("%d attacks caught, want at least %d", s.tp, minCaught)
	}
	if s.fp > maxFlagged {
		t.Errorf("%d benign rows flagged, want at most %d: %v", s.fp, maxFlagged, s.flagged)
	}
}

// The locked part, pinned at its one measurement. It prints no id, also
// when it fails, for the reason given at TestTurkishLockedSet.
func TestDeepsetTurkishLockedSet(t *testing.T) {
	const minCaught, maxFlagged = 2, 0
	s := scoreTurkish(t, trdLockedCSV)
	t.Log(s.table())
	if s.tp < minCaught {
		t.Errorf("%d attacks caught, want at least %d", s.tp, minCaught)
	}
	if s.fp > maxFlagged {
		t.Errorf("%d benign rows flagged, want at most %d", s.fp, maxFlagged)
	}
}

// Each row is in the part of deepset's split its id names, no id is in both,
// and every row's category agrees with its expected action.
func TestDeepsetTurkishSplit(t *testing.T) {
	categories := map[string]bool{
		"trd.override": true, "trd.extraction": true, "trd.roleplay": true, "trd.hijack": true, "trd.benign": true,
	}
	seen := map[string]bool{}
	texts := map[string]string{}
	for path, prefix := range map[string]string{trdTuningCSV: "trd-train-", trdLockedCSV: "trd-test-"} {
		samples, err := loadCSV(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		if len(samples) == 0 {
			t.Fatalf("%s has no rows", path)
		}
		for _, row := range samples {
			if seen[row.ID] {
				t.Errorf("id %s appears twice", row.ID)
			}
			seen[row.ID] = true
			if !strings.HasPrefix(row.ID, prefix) {
				t.Errorf("a row of %s has an id outside its part", path)
			}
			if !categories[row.Category] {
				t.Errorf("a row of %s has category %q", path, row.Category)
			}
			if (row.Category == "trd.benign") != (row.ExpectedAction == "PASS") {
				t.Errorf("a row of %s: category %s with expected_action %s", path, row.Category, row.ExpectedAction)
			}
			key := strings.ToLower(strings.TrimSpace(row.Prompt))
			if other, ok := texts[key]; ok && other != path {
				t.Errorf("the same text is in both parts")
			}
			texts[key] = path
		}
	}
}
