package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/classifier"
	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// The structural signals (internal/classifier/structure.go) make the
// classifier judge a text by signal_threshold. They are no verdict and do
// nothing without the classifier, so what is measured here is how many
// texts of each set carry one: the attacks, where a signal can only help,
// and the benign rows, where it gives the classifier the chance to err.
//
// The rules run first, as in the proxy: a request they block is not sent
// to the classifier, and neither is a text they already found an injection
// in. "Left" counts the rows the classifier would be asked about.
//
// With TAMGA_CLASSIFIER_SCORES naming a JSON file of scores ({"sha256 of
// the text": score}), the classifier's verdicts are added, at the shipped
// threshold and signal threshold. TAMGA_STRUCTURE_TEXTS_OUT writes the
// texts to score, one JSON object per line. Both are for a local
// measurement and are not needed by the test.
//
// The locked parts are left out unless TAMGA_STRUCTURE_LOCKED=1, and are
// then reported as counts only.

type structureSet struct {
	name, path string
	tool       bool
	locked     bool
}

var structureSets = []structureSet{
	{"prompts.csv", "../../testdata/redteam/prompts.csv", false, false},
	{"tr_tuning.csv", trTuningCSV, false, false},
	{"trd_tuning.csv", trdTuningCSV, false, false},
	{"discourse_tuning.csv", dsTuningCSV, false, false},
	{"tooljson_tuning.csv", tjTuningCSV, true, false},
	{"holdout.csv", "../../testdata/redteam/holdout.csv", false, true},
	{"tr_locked.csv", trLockedCSV, false, true},
	{"trd_locked.csv", trdLockedCSV, false, true},
	{"discourse_locked.csv", dsLockedCSV, false, true},
	{"tooljson_locked.csv", tjLockedCSV, true, true},
}

// structureLabel is 1 for an instruction aimed at the model, 0 for a benign
// row, -1 for neither (PII, secrets, requests for data), the labels of
// classifier/finetune.
func structureLabel(category, expected string) int {
	pass := expected == "PASS"
	switch {
	case strings.HasPrefix(category, "tr.benign.") || strings.HasPrefix(category, "trd.benign"):
		return 0
	case strings.HasPrefix(category, "trd."):
		if pass {
			return -1
		}
		return 1
	case category == "tr.exfiltration":
		return -1
	case strings.HasPrefix(category, "tr."):
		return 1
	case strings.HasPrefix(category, "ds.benign"), strings.HasPrefix(category, "tj.benign"):
		if pass {
			return 0
		}
		return -1
	case strings.HasPrefix(category, "ds.attack"), strings.HasPrefix(category, "tj.attack"):
		return 1
	case category == "benign" || strings.HasPrefix(category, "benign."):
		if pass {
			return 0
		}
		return -1
	}
	for _, p := range []string{"jailbreak.", "injection.", "indirect.", "tool."} {
		if strings.HasPrefix(category, p) && !pass {
			return 1
		}
	}
	return -1
}

// structureRow is what the proxy would do with one row.
type structureRow struct {
	id       string
	attack   bool
	text     string // the text sent to the classifier
	blocked  bool   // the rules block the request
	left     bool   // the classifier is asked about the text
	signal   string // a signal the proxy had before the structural ones
	category string // the structural signal, "" when none
}

func structureRows(t *testing.T, set structureSet, pol *policy.Policy, reg *scanner.Registry) []structureRow {
	t.Helper()
	load := loadCSV
	if set.tool {
		load = loadToolJSON
	}
	samples, err := load(set.path)
	if err != nil {
		t.Fatalf("load %s: %v", set.path, err)
	}
	var out []structureRow
	for _, s := range samples {
		label := structureLabel(s.Category, s.ExpectedAction)
		if label < 0 {
			continue
		}
		segs := []extract.Segment{{Role: extract.RoleUser, Kind: extract.KindText, Path: "messages[0].content", Text: s.Prompt}}
		if set.tool {
			segs = []extract.Segment{
				{Role: extract.RoleUser, Kind: extract.KindText, Path: "messages[0].content", Text: "Summarise what the tool returned."},
				{Role: extract.RoleTool, Kind: extract.KindToolResult, Path: "messages[2].content", Text: s.Prompt},
			}
		}
		idx := len(segs) - 1
		findings, _ := reg.ScanSegments(context.Background(), segs, scanner.PipelineConfig{})
		row := structureRow{id: s.ID, attack: label == 1, text: segs[idx].Text}
		row.blocked = pol.Evaluate(findings) == policy.ActionBlock
		flagged := false
		for _, f := range findings {
			if f.Type != "injection" || f.Seg != idx {
				continue
			}
			if scanner.Signal(f) {
				row.signal = f.Category
			} else {
				flagged = true
			}
		}
		row.left = !row.blocked && !flagged && classifier.Prepare(row.text) != ""
		if row.signal == "" {
			var before classifier.ScriptCounts
			for _, seg := range segs[:idx] {
				before.Add(seg.Text)
			}
			if _, switched := classifier.ScriptSwitch(before.Dominant(), row.text); switched {
				row.signal = "script_switch"
			}
		}
		row.category = classifier.Structure(string(segs[idx].Role), row.text)
		out = append(out, row)
	}
	return out
}

type structureCount struct {
	attacks, attacksSignalled, attacksLeft, attacksLeftSignalled int
	benign, benignSignalled, benignLeft, benignLeftSignalled     int
	// With scores: what the classifier flags among the rows left to it,
	// before (the signals the proxy had) and with the structural ones.
	scored                 bool
	tpBefore, tpAfter      int
	fpBefore, fpAfter      int
	bySignal               map[string][2]int // signal -> attacks, benign (rows left)
	benignIDs              []string
	missingScores          int
	addedTPIDs, addedFPIDs []string
}

func countStructure(rows []structureRow, scores map[string]float64, threshold, signalThreshold float64) structureCount {
	c := structureCount{bySignal: map[string][2]int{}, scored: scores != nil}
	for _, r := range rows {
		sig := r.category != ""
		if r.attack {
			c.attacks++
			if sig {
				c.attacksSignalled++
			}
		} else {
			c.benign++
			if sig {
				c.benignSignalled++
			}
		}
		if !r.left {
			continue
		}
		n := c.bySignal[r.category]
		if r.attack {
			c.attacksLeft++
			if sig {
				c.attacksLeftSignalled++
				n[0]++
			}
		} else {
			c.benignLeft++
			if sig {
				c.benignLeftSignalled++
				n[1]++
				c.benignIDs = append(c.benignIDs, r.id+"("+r.category+")")
			}
		}
		if sig {
			c.bySignal[r.category] = n
		}
		if scores == nil {
			continue
		}
		score, ok := scores[textKey(r.text)]
		if !ok {
			c.missingScores++
			continue
		}
		before := score >= threshold || (r.signal != "" && score >= signalThreshold)
		after := before || (sig && score >= signalThreshold)
		if r.attack {
			if before {
				c.tpBefore++
			}
			if after {
				c.tpAfter++
			}
			if after && !before {
				c.addedTPIDs = append(c.addedTPIDs, r.id)
			}
		} else {
			if before {
				c.fpBefore++
			}
			if after {
				c.fpAfter++
			}
			if after && !before {
				c.addedFPIDs = append(c.addedFPIDs, r.id+"("+r.category+")")
			}
		}
	}
	return c
}

func textKey(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func loadScores(t *testing.T) map[string]float64 {
	path := os.Getenv("TAMGA_CLASSIFIER_SCORES")
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scores: %v", err)
	}
	scores := map[string]float64{}
	if err := json.Unmarshal(raw, &scores); err != nil {
		t.Fatalf("read scores: %v", err)
	}
	return scores
}

// Ceilings on the benign rows of each tuning part that carry a structural
// signal and are left to the classifier: a pattern that grows to cover
// more ordinary text fails here. Floors on the attacks left to the
// classifier that carry one.
var structureBounds = map[string]struct{ minAttacks, maxBenign int }{
	"prompts.csv":          {5, 0},
	"tr_tuning.csv":        {14, 10},
	"trd_tuning.csv":       {135, 1},
	"discourse_tuning.csv": {0, 8},
	"tooljson_tuning.csv":  {0, 0},
}

func TestStructureSignals(t *testing.T) {
	pol := loadPolicy("../../tamga-policy.yaml")
	if pol == nil {
		t.Fatal("default policy did not load")
	}
	reg := buildRegistry()
	scores := loadScores(t)
	locked := os.Getenv("TAMGA_STRUCTURE_LOCKED") == "1"
	threshold, signalThreshold := policy.DefaultClassifierThreshold, policy.DefaultClassifierSignalThreshold

	var dump *bufio.Writer
	if path := os.Getenv("TAMGA_STRUCTURE_TEXTS_OUT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		dump = bufio.NewWriter(f)
		defer func() { _ = dump.Flush() }()
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "\n  %-22s %8s %8s %8s %8s | %8s %8s %8s %8s", "set", "attacks", "signal", "left", "left+sig", "benign", "signal", "left", "left+sig")
	if scores != nil {
		fmt.Fprintf(&sb, " | TP %.2f/%.2f before after  FP before after", threshold, signalThreshold)
	}
	sb.WriteString("\n")
	for _, set := range structureSets {
		if set.locked && !locked && dump == nil {
			continue
		}
		rows := structureRows(t, set, pol, reg)
		if dump != nil {
			enc := json.NewEncoder(dump)
			for _, r := range rows {
				if r.left {
					_ = enc.Encode(map[string]string{"key": textKey(r.text), "text": r.text})
				}
			}
		}
		if set.locked && !locked {
			continue
		}
		c := countStructure(rows, scores, threshold, signalThreshold)
		fmt.Fprintf(&sb, "  %-22s %8d %8d %8d %8d | %8d %8d %8d %8d", set.name,
			c.attacks, c.attacksSignalled, c.attacksLeft, c.attacksLeftSignalled,
			c.benign, c.benignSignalled, c.benignLeft, c.benignLeftSignalled)
		if c.scored {
			fmt.Fprintf(&sb, " | %d -> %d   %d -> %d", c.tpBefore, c.tpAfter, c.fpBefore, c.fpAfter)
			if c.missingScores > 0 {
				fmt.Fprintf(&sb, "  (%d unscored)", c.missingScores)
			}
		}
		sb.WriteString("\n")
		if set.locked {
			continue
		}
		keys := make([]string, 0, len(c.bySignal))
		for k := range c.bySignal {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&sb, "      %-22s attacks %d, benign %d\n", k, c.bySignal[k][0], c.bySignal[k][1])
		}
		if testing.Verbose() {
			fmt.Fprintf(&sb, "      benign with a signal: %v\n", c.benignIDs)
			if c.scored {
				fmt.Fprintf(&sb, "      added TP: %v\n      added FP: %v\n", c.addedTPIDs, c.addedFPIDs)
			}
		}
		b, ok := structureBounds[set.name]
		if !ok {
			continue
		}
		if c.attacksLeftSignalled < b.minAttacks {
			t.Errorf("%s: %d attacks left to the classifier carry a structural signal, want at least %d", set.name, c.attacksLeftSignalled, b.minAttacks)
		}
		if c.benignLeftSignalled > b.maxBenign {
			t.Errorf("%s: %d benign rows left to the classifier carry a structural signal, want at most %d: %v", set.name, c.benignLeftSignalled, b.maxBenign, c.benignIDs)
		}
	}
	t.Log(sb.String())
}

// The benign developer texts carry no structural signal, as a message of
// the user or as a tool result.
func TestBenignDevTextsCarryNoStructuralSignal(t *testing.T) {
	for _, b := range benignDevTexts {
		for _, role := range []string{"user", "tool"} {
			if s := classifier.Structure(role, b.text); s != "" {
				t.Errorf("%s as %s: %s", b.id, role, s)
			}
		}
	}
}
