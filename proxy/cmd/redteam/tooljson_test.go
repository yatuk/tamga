package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// The tool-JSON set is tool results as MCP tools and function calls return
// them: JSON, with the instruction in a string value, behind \u escapes,
// encoded twice, split over two fields of one object, in base64, or in a
// field a tool fills from someone else's text. A row is scanned as the
// content of one tool result, the way the proxy scans a request, and not as
// one loose text: what the proxy does with a tool result that is JSON is
// what is measured. It is split like the Turkish set, with a seed of its
// own; the header of each file says how it was made.
const (
	tjTuningCSV = "../../testdata/redteam/tooljson_tuning.csv"
	tjLockedCSV = "../../testdata/redteam/tooljson_locked.csv"

	tjSplitSeed     = "tamga-tj-2026-10-17"
	tjLockedPercent = 40
)

func tjLocked(id string) bool {
	sum := sha256.Sum256([]byte(tjSplitSeed + "|" + id))
	return binary.BigEndian.Uint32(sum[:4])%100 < tjLockedPercent
}

// loadToolJSON reads a part of the set. Unlike loadCSV it unescapes
// nothing: a \n in a row is the JSON escape the tool wrote.
func loadToolJSON(path string) ([]sample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.Comment = '#'
	var out []sample
	first := true
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if first {
			first = false
			continue
		}
		if len(rec) < 4 {
			continue
		}
		out = append(out, sample{ID: rec[0], Category: rec[1], ExpectedAction: strings.ToUpper(strings.TrimSpace(rec[2])), Prompt: rec[3]})
	}
	return out, nil
}

type tjScore struct {
	buckets         map[string]*categoryBucket
	tp, fp, fn, tn  int
	missed, flagged []string
}

// scoreToolJSON scans each row as a tool result and scores it with the
// shipped policy. An attack counts as caught when the policy blocks it on
// its injection findings; a benign row counts as flagged when the policy
// acts on any of its findings.
func scoreToolJSON(t *testing.T, csvPath string) tjScore {
	t.Helper()
	samples, err := loadToolJSON(csvPath)
	if err != nil {
		t.Fatalf("load %s: %v", csvPath, err)
	}
	pol := loadPolicy("../../tamga-policy.yaml")
	if pol == nil {
		t.Fatal("default policy did not load")
	}
	reg := buildRegistry()
	s := tjScore{buckets: map[string]*categoryBucket{}}
	for _, row := range samples {
		segs := []extract.Segment{
			{Role: extract.RoleUser, Kind: extract.KindText, Path: "messages[0].content", Text: "Summarise what the tool returned."},
			{Role: extract.RoleTool, Kind: extract.KindToolResult, Path: "messages[2].content", Text: row.Prompt},
		}
		findings, _ := reg.ScanSegments(context.Background(), segs, scanner.PipelineConfig{})
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
			if pol.Evaluate(injection) == policy.ActionBlock {
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

func (s tjScore) table() string {
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

// The tuning part is a floor: a change that catches fewer of its attacks or
// flags more of its benign rows fails here.
func TestToolJSONTuningSet(t *testing.T) {
	const minCaught, maxFlagged = 62, 2
	s := scoreToolJSON(t, tjTuningCSV)
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

// The locked part. It prints no id, also when it fails (see
// TestTurkishLockedSet).
func TestToolJSONLockedSet(t *testing.T) {
	const minCaught, maxFlagged = 55, 1
	s := scoreToolJSON(t, tjLockedCSV)
	t.Log(s.table())
	if s.tp < minCaught {
		t.Errorf("%d attacks caught, want at least %d", s.tp, minCaught)
	}
	if s.fp > maxFlagged {
		t.Errorf("%d benign rows flagged, want at most %d", s.fp, maxFlagged)
	}
}

// Each row is in the part its id hashes to, no id is in both, every
// category has rows in both parts, and every row is a JSON document.
func TestToolJSONSplitIsTheSeededHash(t *testing.T) {
	seen := map[string]bool{}
	cats := map[string][2]int{}
	for part, path := range []string{tjTuningCSV, tjLockedCSV} {
		samples, err := loadToolJSON(path)
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
			if tjLocked(row.ID) != (part == 1) {
				t.Errorf("a row of %s is in the wrong part", path)
			}
			if benign := strings.HasPrefix(row.Category, "tj.benign."); benign != (row.ExpectedAction == "PASS") {
				t.Errorf("a row of %s: category %s with expected_action %s", path, row.Category, row.ExpectedAction)
			}
			if !json.Valid([]byte(row.Prompt)) {
				t.Errorf("a row of %s is not JSON", path)
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

// A split row is meant to be an attack only read whole: no string of it is
// one alone, so that a row that is caught is caught because two fields were
// read together. The rows were written before the halves were checked, and
// some hold a half that is blocked alone; they are counted here, by id for
// the tuning part and by number for the locked one, and the numbers are
// held so that a change of rule that makes more halves instructions alone
// shows up.
func TestToolJSONSplitHalves(t *testing.T) {
	pol := loadPolicy("../../tamga-policy.yaml")
	reg := buildRegistry()
	for part, path := range []string{tjTuningCSV, tjLockedCSV} {
		samples, err := loadToolJSON(path)
		if err != nil {
			t.Fatal(err)
		}
		rows, whole := 0, 0
		var alone []string
		for _, row := range samples {
			if row.Category != "tj.attack.split" {
				continue
			}
			rows++
			var doc any
			if err := json.Unmarshal([]byte(row.Prompt), &doc); err != nil {
				t.Fatal(err)
			}
			blocked := false
			for _, s := range jsonStrings(doc) {
				found, _ := reg.ScanAll(context.Background(), []byte(s))
				var injection []scanner.Finding
				for _, f := range found {
					if f.Type == "injection" {
						injection = append(injection, f)
					}
				}
				if pol.Evaluate(injection) == policy.ActionBlock {
					blocked = true
				}
			}
			if blocked {
				alone = append(alone, row.ID)
			} else {
				whole++
			}
		}
		if part == 0 {
			t.Logf("%s: %d of %d split rows are attacks only read whole; a half is blocked alone in %v", path, whole, rows, alone)
		} else {
			t.Logf("%s: %d of %d split rows are attacks only read whole", path, whole, rows)
		}
		if want := []int{7, 5}[part]; whole < want {
			t.Errorf("%s: %d split rows are attacks only read whole, there were %d", path, whole, want)
		}
	}
}
func jsonStrings(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		var out []string
		for _, e := range x {
			out = append(out, jsonStrings(e)...)
		}
		return out
	case map[string]any:
		var out []string
		for _, e := range x {
			out = append(out, jsonStrings(e)...)
		}
		return out
	}
	return nil
}
