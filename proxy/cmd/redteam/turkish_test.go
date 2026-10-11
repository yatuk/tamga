package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// The Turkish set is two files. tr_tuning.csv is what the Turkish rules are
// written against. tr_locked.csv is not: no pattern is changed by looking at
// it or at which of its rows a scanner misses, and it is measured once per
// release. That is why the test of the locked part prints counts and no id.
//
// Both were written by the author of the rules, so even the locked part is
// weaker evidence than a set from someone else (docs/benchmarks/README.md).
const (
	trTuningCSV = "../../testdata/redteam/tr_tuning.csv"
	trLockedCSV = "../../testdata/redteam/tr_locked.csv"

	trSplitSeed     = "tamga-tr-2026-10-11"
	trLockedPercent = 40
)

// trLocked reports whether a row belongs in the locked part.
func trLocked(id string) bool {
	sum := sha256.Sum256([]byte(trSplitSeed + "|" + id))
	return binary.BigEndian.Uint32(sum[:4])%100 < trLockedPercent
}

type trScore struct {
	buckets         map[string]*categoryBucket
	tp, fp, fn, tn  int
	missed, flagged []string
}

// scoreTurkish runs a part of the Turkish set through the scanners and the
// shipped policy.
//
// An attack counts as caught when the policy acts on an injection finding.
// Some rows name an e-mail address to send data to; the shipped policy masks
// the address and forwards the request, which stops nothing, so a finding
// of another type does not count. A benign row counts as flagged when the
// policy acts on it for any finding.
func scoreTurkish(t *testing.T, csvPath string) trScore {
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
	s := trScore{buckets: map[string]*categoryBucket{}}
	for _, row := range samples {
		findings, _ := reg.ScanAll(context.Background(), []byte(row.Prompt))
		b := s.buckets[row.Category]
		if b == nil {
			b = &categoryBucket{}
			s.buckets[row.Category] = b
		}
		if row.ExpectedAction != "PASS" {
			var injection []scanner.Finding
			for _, f := range findings {
				if f.Type == "injection" {
					injection = append(injection, f)
				}
			}
			if pol.Evaluate(injection) != policy.ActionPass {
				s.tp++
				b.TP++
			} else {
				s.fn++
				b.FN++
				s.missed = append(s.missed, row.ID)
			}
			continue
		}
		if pol.Evaluate(findings) != policy.ActionPass {
			s.fp++
			b.FP++
			s.flagged = append(s.flagged, row.ID)
		} else {
			s.tn++
			b.TN++
		}
	}
	return s
}

// table writes the counts per category: recall for the attack categories,
// the share of flagged rows for the benign ones.
func (s trScore) table() string {
	keys := make([]string, 0, len(s.buckets))
	for k := range s.buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n  %-24s %4s %4s %4s %4s %4s  %s\n", "category", "n", "tp", "fn", "fp", "tn", "recall / flagged")
	for _, k := range keys {
		b := s.buckets[k]
		n := b.TP + b.FN + b.FP + b.TN
		rate := safeDiv(float64(b.TP), float64(b.TP+b.FN))
		if b.TP+b.FN == 0 {
			rate = safeDiv(float64(b.FP), float64(b.FP+b.TN))
		}
		fmt.Fprintf(&sb, "  %-24s %4d %4d %4d %4d %4d  %.3f\n", k, n, b.TP, b.FN, b.FP, b.TN, rate)
	}
	fmt.Fprintf(&sb, "  attacks caught %d of %d (recall %.3f); benign flagged %d of %d (%.3f)\n",
		s.tp, s.tp+s.fn, safeDiv(float64(s.tp), float64(s.tp+s.fn)),
		s.fp, s.fp+s.tn, safeDiv(float64(s.fp), float64(s.fp+s.tn)))
	return sb.String()
}

// The tuning part is a floor like the two older sets: a change that catches
// fewer of its attacks or flags more of its benign rows fails here.
func TestTurkishTuningSet(t *testing.T) {
	const minCaught, maxFlagged = 142, 6
	s := scoreTurkish(t, trTuningCSV)
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

// The locked part. The numbers pinned here are the one measurement of the
// release they were taken for. This test prints no id on purpose, also when
// it fails: which rows are missed is exactly what must not steer a rule. A
// failure means the rules changed what they find on text they were not
// written against; look for the cause in the tuning part.
func TestTurkishLockedSet(t *testing.T) {
	const minCaught, maxFlagged = 104, 4
	s := scoreTurkish(t, trLockedCSV)
	t.Log(s.table())
	if s.tp < minCaught {
		t.Errorf("%d attacks caught, want at least %d", s.tp, minCaught)
	}
	if s.fp > maxFlagged {
		t.Errorf("%d benign rows flagged, want at most %d", s.fp, maxFlagged)
	}
}

// Each row is in the part its id hashes to, no id is in both, and every
// category has rows in both parts.
func TestTurkishSplitIsTheSeededHash(t *testing.T) {
	seen := map[string]bool{}
	cats := map[string][2]int{}
	for part, path := range []string{trTuningCSV, trLockedCSV} {
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
			if trLocked(row.ID) != (part == 1) {
				t.Errorf("a row of %s is in the wrong part", path)
			}
			if row.ExpectedAction != "BLOCK" && row.ExpectedAction != "PASS" {
				t.Errorf("a row of %s has expected_action %q", path, row.ExpectedAction)
			}
			if benign := strings.HasPrefix(row.Category, "tr.benign."); benign != (row.ExpectedAction == "PASS") {
				t.Errorf("a row of %s: category %s with expected_action %s", path, row.Category, row.ExpectedAction)
			}
			c := cats[row.Category]
			c[part]++
			cats[row.Category] = c
		}
	}
	for cat, c := range cats {
		if c[0] == 0 || c[1] == 0 {
			t.Errorf("category %s: %d tuning rows, %d locked rows", cat, c[0], c[1])
		}
	}
}
