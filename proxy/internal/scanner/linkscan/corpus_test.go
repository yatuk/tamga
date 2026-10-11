package linkscan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two sets are in testdata, as JSON, so that the proxy's tests can put
// the same answers through a buffered and a streamed response.
//
// benign_answers.json holds answers of the kind a model gives every day:
// links to documentation, repositories, packages, Turkish sites, searches,
// signed downloads, badges, local services, links shown in code. None of
// them may be reported. They were written for this test; every host that is
// not a well-known one is under example.com, example.org or a reserved name.
//
// malicious_answers.json holds answers with a link that must be reported,
// and the category it must be reported under.
type maliciousAnswer struct {
	Name     string `json:"name"`
	Text     string `json:"text"`
	Category string `json:"category"`
}

var (
	benignAnswers    []string
	maliciousAnswers []maliciousAnswer
)

func init() {
	load := func(name string, into interface{}) {
		raw, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			panic(err)
		}
		if err := json.Unmarshal(raw, into); err != nil {
			panic(name + ": " + err.Error())
		}
	}
	load("benign_answers.json", &benignAnswers)
	load("malicious_answers.json", &maliciousAnswers)
}

func categories(hits []Hit) []string {
	var out []string
	for _, h := range hits {
		out = append(out, h.Category)
	}
	return out
}

// The measurement the brief asks for: no ordinary answer is reported.
func TestCorpus_BenignAnswersAreNotReported(t *testing.T) {
	if len(benignAnswers) < 40 {
		t.Fatalf("the benign set has %d answers, at least 40 are wanted", len(benignAnswers))
	}
	links, flagged := 0, 0
	for i, a := range benignAnswers {
		links += strings.Count(a, "://")
		if hits := Scan(a, nil); len(hits) > 0 {
			flagged++
			for _, h := range hits {
				t.Errorf("benign answer %d: %s reported for %q", i, h.Category, a[h.Start:h.End])
			}
		}
	}
	t.Logf("%d benign answers with %d addresses: %d reported", len(benignAnswers), links, flagged)
}

func TestCorpus_MaliciousLinksAreReported(t *testing.T) {
	caught := 0
	for _, m := range maliciousAnswers {
		hits := Scan(m.Text, nil)
		found := false
		for _, h := range hits {
			if h.Category == m.Category {
				found = true
				if h.Start < 0 || h.End > len(m.Text) || h.Start >= h.End {
					t.Errorf("%s: span %d..%d is not in the text", m.Name, h.Start, h.End)
				}
			}
		}
		if found {
			caught++
		} else {
			t.Errorf("%s: want %s, got %v", m.Name, m.Category, categories(hits))
		}
	}
	t.Logf("%d of %d malicious links reported", caught, len(maliciousAnswers))
}

// The benign set stays unreported however the text is cut: a stream is
// scanned in windows, and a window can start and end anywhere. A window
// that starts inside a code block no longer sees the fence, so the answers
// that show a link in code are left out here.
func TestCorpus_BenignAnswersInWindows(t *testing.T) {
	for i, a := range benignAnswers {
		if strings.Contains(a, "`") {
			continue
		}
		for from := 0; from < len(a); from += 7 {
			for _, size := range []int{40, 120, 320} {
				to := from + size
				if to > len(a) {
					to = len(a)
				}
				// A window is cut on a rune, as the stream guard cuts it.
				for from < to && a[from]&0xC0 == 0x80 {
					from++
				}
				for to < len(a) && a[to]&0xC0 == 0x80 {
					to++
				}
				for _, h := range Scan(a[from:to], nil) {
					if h.End == to-from && to < len(a) {
						// An address the window cuts off may still be
						// growing: the stream guard holds such a finding
						// until more text has come, and judges it then.
						continue
					}
					t.Fatalf("benign answer %d, window %d..%d: %s reported for %q", i, from, to, h.Category, a[from:to][h.Start:h.End])
				}
			}
		}
	}
}
