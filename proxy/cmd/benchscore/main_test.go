package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/policy"
)

// firstRow returns the text of the first row of prompts.csv (the tuning
// corpus) whose category is category. The test writes no attack text of
// its own.
func firstRow(t *testing.T, category string) string {
	t.Helper()
	f, err := os.Open("../../testdata/redteam/prompts.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(rec) >= 4 && rec[1] == category {
			return strings.ReplaceAll(rec[3], `\n`, "\n")
		}
	}
	t.Fatalf("no row of category %s", category)
	return ""
}

func TestRunWritesOneVerdictPerText(t *testing.T) {
	pol, err := policy.LoadFromFile("../../tamga-policy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := buildRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var in bytes.Buffer
	for _, item := range []input{
		{ID: "attack", Text: firstRow(t, "jailbreak.override")},
		{ID: "benign", Text: firstRow(t, "benign")},
		{ID: "short", Text: "ok"},
	} {
		line, _ := json.Marshal(item)
		in.Write(append(line, '\n'))
	}
	var out bytes.Buffer
	if err := run(reg, pol, &in, &out); err != nil {
		t.Fatal(err)
	}
	got := map[string]verdict{}
	sc := bufio.NewScanner(&out)
	for sc.Scan() {
		var v verdict
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		got[v.ID] = v
	}
	if len(got) != 3 {
		t.Fatalf("got %d verdicts, want 3", len(got))
	}
	if a := got["attack"]; a.InjectionAction == "PASS" || a.AskClassifier {
		t.Errorf("attack row: %+v, want an injection action and no classifier call", a)
	}
	b := got["benign"]
	if b.Action != "PASS" || b.InjectionAction != "PASS" || !b.AskClassifier || b.Prepared == "" {
		t.Errorf("benign row: %+v, want PASS and a classifier call", b)
	}
	if b.ClassifierAction == "" || b.ClassifierAction == "PASS" {
		t.Errorf("a classifier flag leads to %q, want an action", b.ClassifierAction)
	}
	if s := got["short"]; s.AskClassifier {
		t.Errorf("a text with too few letters is not sent to the classifier: %+v", s)
	}
}

func TestRunRejectsALineThatIsNotJSON(t *testing.T) {
	pol, err := policy.LoadFromFile("../../tamga-policy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := buildRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := run(reg, pol, strings.NewReader("not json\n"), io.Discard); err == nil {
		t.Fatal("want an error")
	}
}
