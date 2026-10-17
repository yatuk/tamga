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

// The discourse set is text about prompt injection: security write-ups,
// guidance, teaching material, incident reports, a README's "what this
// blocks", and passages of this repository's own docs. Such text quotes the
// phrases the rules look for, and an agent reads it all day. Next to it
// stand attacks in the same dress: an instruction written plainly beside the
// words a discussion uses, one wrapped in a code block or a quotation that
// the text tells the model to carry out, one in a tool result that speaks
// to the model. It is split like the Turkish set, with a seed of its own;
// the header of each file says how it was made.
const (
	dsTuningCSV = "../../testdata/redteam/discourse_tuning.csv"
	dsLockedCSV = "../../testdata/redteam/discourse_locked.csv"

	dsSplitSeed     = "tamga-ds-2026-10-17"
	dsLockedPercent = 40
)

// dsLocked reports whether a row belongs in the locked part.
func dsLocked(id string) bool {
	sum := sha256.Sum256([]byte(dsSplitSeed + "|" + id))
	return binary.BigEndian.Uint32(sum[:4])%100 < dsLockedPercent
}

// dsScore is the score of a part of the discourse set, by what the shipped
// policy does with the injection findings of a row. An attack counts as
// caught only when they block it: a warning lets the text through. A benign
// row counts as blocked, as warned about (the quoted_injection signal is
// what the shipped policy warns about), or as passed. What the policy does
// for findings of another kind is counted apart: the repository's docs
// hold e-mail addresses, which it masks and forwards.
type dsScore struct {
	buckets                 map[string]*dsBucket
	caught, missed          int
	blocked, warned, passed int
	other                   int
	missedIDs, blockedIDs   []string
}

type dsBucket struct{ n, caught, missed, blocked, warned, other int }

func scoreDiscourse(t *testing.T, csvPath string) dsScore {
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
	s := dsScore{buckets: map[string]*dsBucket{}}
	for _, row := range samples {
		findings, _ := reg.ScanAll(context.Background(), []byte(row.Prompt))
		var injection, rest []scanner.Finding
		for _, f := range findings {
			if f.Type == "injection" {
				injection = append(injection, f)
			} else {
				rest = append(rest, f)
			}
		}
		b := s.buckets[row.Category]
		if b == nil {
			b = &dsBucket{}
			s.buckets[row.Category] = b
		}
		b.n++
		action := pol.Evaluate(injection)
		if row.ExpectedAction != "PASS" {
			if action == policy.ActionBlock {
				s.caught++
				b.caught++
			} else {
				s.missed++
				b.missed++
				s.missedIDs = append(s.missedIDs, row.ID)
			}
			continue
		}
		switch action {
		case policy.ActionPass:
			s.passed++
		case policy.ActionWarn:
			s.warned++
			b.warned++
		default:
			s.blocked++
			b.blocked++
			s.blockedIDs = append(s.blockedIDs, row.ID)
		}
		if pol.Evaluate(rest) != policy.ActionPass {
			s.other++
			b.other++
		}
	}
	return s
}

func (s dsScore) table() string {
	keys := make([]string, 0, len(s.buckets))
	for k := range s.buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n  %-22s %4s %6s %6s %7s %6s %6s\n", "category", "n", "caught", "missed", "blocked", "warned", "other")
	for _, k := range keys {
		b := s.buckets[k]
		fmt.Fprintf(&sb, "  %-22s %4d %6d %6d %7d %6d %6d\n", k, b.n, b.caught, b.missed, b.blocked, b.warned, b.other)
	}
	benign := s.blocked + s.warned + s.passed
	fmt.Fprintf(&sb, "  attacks blocked %d of %d; benign blocked %d of %d (%.3f), warned about %d, passed %d; other findings acted on in %d benign rows\n",
		s.caught, s.caught+s.missed, s.blocked, benign, safeDiv(float64(s.blocked), float64(benign)), s.warned, s.passed, s.other)
	return sb.String()
}

// check holds a score to its floors: attacks blocked, benign rows blocked,
// and benign rows the injection findings are acted on for at all.
func (s dsScore) check(t *testing.T, minCaught, maxBlocked, maxFlagged int, ids bool) {
	t.Helper()
	missed, blocked := "", ""
	if ids {
		missed, blocked = fmt.Sprint(": ", s.missedIDs), fmt.Sprint(": ", s.blockedIDs)
	}
	if s.caught < minCaught {
		t.Errorf("%d attacks blocked, want at least %d%s", s.caught, minCaught, missed)
	}
	if s.blocked > maxBlocked {
		t.Errorf("%d benign rows blocked, want at most %d%s", s.blocked, maxBlocked, blocked)
	}
	if s.blocked+s.warned > maxFlagged {
		t.Errorf("%d benign rows blocked or warned about, want at most %d", s.blocked+s.warned, maxFlagged)
	}
}

// The tuning part is a floor: a change that blocks fewer of its attacks, or
// blocks or warns about more of its benign rows, fails here.
func TestDiscourseTuningSet(t *testing.T) {
	const minCaught, maxBlocked, maxFlagged = 26, 3, 49
	s := scoreDiscourse(t, dsTuningCSV)
	t.Log(s.table())
	t.Logf("missed: %v", s.missedIDs)
	t.Logf("blocked: %v", s.blockedIDs)
	s.check(t, minCaught, maxBlocked, maxFlagged, true)
}

// The locked part, pinned at its measurement after the rules for quoted
// text were written (2026-10-17): 12 of 12 attacks blocked, 4 of 74 benign
// rows blocked and 31 warned about. Before those rules, in the one
// measurement taken then, the shipped policy blocked the 12 attacks and 39
// of the benign rows. It prints no id, also when it fails, for the reason
// given at TestTurkishLockedSet.
func TestDiscourseLockedSet(t *testing.T) {
	const minCaught, maxBlocked, maxFlagged = 12, 4, 35
	s := scoreDiscourse(t, dsLockedCSV)
	t.Log(s.table())
	s.check(t, minCaught, maxBlocked, maxFlagged, false)
}

// Each row is in the part its id hashes to, no id is in both, and every
// category has rows in both parts.
func TestDiscourseSplitIsTheSeededHash(t *testing.T) {
	seen := map[string]bool{}
	cats := map[string][2]int{}
	for part, path := range []string{dsTuningCSV, dsLockedCSV} {
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
			if dsLocked(row.ID) != (part == 1) {
				t.Errorf("a row of %s is in the wrong part", path)
			}
			if row.ExpectedAction != "BLOCK" && row.ExpectedAction != "PASS" {
				t.Errorf("a row of %s has expected_action %q", path, row.ExpectedAction)
			}
			if benign := strings.HasPrefix(row.Category, "ds.benign."); benign != (row.ExpectedAction == "PASS") {
				t.Errorf("a row of %s: category %s with expected_action %s", path, row.Category, row.ExpectedAction)
			}
			if !strings.HasPrefix(row.Category, "ds.benign.") && !strings.HasPrefix(row.Category, "ds.attack.") {
				t.Errorf("a row of %s has category %q", path, row.Category)
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
