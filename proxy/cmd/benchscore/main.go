// Command benchscore scans texts with Tamga's rules and writes one verdict
// per text, for scripts/bench/compare.py, which sets Tamga beside other
// tools on public sets.
//
// It reads JSON Lines of {"id": ..., "text": ...} and writes JSON Lines with
// the shipped policy's action on all findings and on the injection findings
// alone, and what the proxy would send the inline classifier: the prepared
// text, the threshold it would be judged by, or nothing when the rules have
// already decided. The classifier itself is not called here; compare.py
// scores the prepared text with the service's own code.
//
// Usage (from proxy/):
//
//	go run ./cmd/benchscore -in texts.jsonl -out verdicts.jsonl
//
// Each text is scanned as one user message: the same scanners as the proxy
// (cmd/tamga), the same policy file as cmd/redteam.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/yatuk/tamga/internal/classifier"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

type input struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type verdict struct {
	ID string `json:"id"`
	// Action is the shipped policy's action on every finding of the text:
	// what the proxy does with it with the classifier off.
	Action string `json:"action"`
	// InjectionAction is the action on the injection findings alone, so
	// that a PII or secret finding does not count as an injection caught.
	InjectionAction string `json:"injection_action"`
	// Categories are the categories of the injection findings.
	Categories []string `json:"categories,omitempty"`
	// AskClassifier is whether the proxy would send this text to the
	// classifier: the rules did not block it, found no injection that is
	// more than a signal, and Prepare left text to read.
	AskClassifier bool `json:"ask_classifier"`
	// Prepared is what Prepare leaves of the text: what the classifier
	// reads, also when the proxy would not ask it. Empty when too few
	// letters are left.
	Prepared string `json:"prepared,omitempty"`
	// Signal names the signal that would put the text under
	// signal_threshold instead of threshold; empty when there is none.
	Signal string `json:"signal,omitempty"`
	// ClassifierAction is the policy's action when the classifier reports
	// the text as an injection: what a flag by the model leads to.
	ClassifierAction string `json:"classifier_action,omitempty"`
	// ElapsedUS is the time the rules took, scan and policy, in microseconds.
	ElapsedUS int64 `json:"elapsed_us"`
}

func main() {
	inPath := flag.String("in", "", "JSON Lines file of {\"id\", \"text\"}")
	outPath := flag.String("out", "", "JSON Lines file to write (default: standard output)")
	policyPath := flag.String("policy", "tamga-policy.yaml", "Policy file")
	flag.Parse()

	pol, err := policy.LoadFromFile(*policyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "policy: %v\n", err)
		os.Exit(2)
	}
	in, err := os.Open(*inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "input: %v\n", err)
		os.Exit(2)
	}
	defer func() { _ = in.Close() }()
	var out io.Writer = os.Stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "output: %v\n", err)
			os.Exit(2)
		}
		defer func() { _ = f.Close() }()
		out = f
	}
	reg, err := buildRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "scanners: %v\n", err)
		os.Exit(2)
	}
	if err := run(reg, pol, in, out); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

// buildRegistry registers the scanners cmd/tamga registers.
func buildRegistry() (*scanner.Registry, error) {
	reg := scanner.NewRegistry()
	reg.Register(scanner.NewPIIScanner())
	reg.Register(scanner.NewSecretScanner())
	reg.Register(scanner.NewInjectionScanner())
	reg.Register(scanner.NewJailbreakScanner())
	reg.Register(scanner.NewContentModerationScanner())
	return reg, scanner.InitDFA()
}

func run(reg *scanner.Registry, pol *policy.Policy, in io.Reader, out io.Writer) error {
	dec := bufio.NewScanner(in)
	dec.Buffer(make([]byte, 0, 1<<20), 64<<20)
	w := bufio.NewWriter(out)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	line := 0
	for dec.Scan() {
		line++
		if len(dec.Bytes()) == 0 {
			continue
		}
		var item input
		if err := json.Unmarshal(dec.Bytes(), &item); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if err := enc.Encode(score(reg, pol, item)); err != nil {
			return err
		}
	}
	if err := dec.Err(); err != nil {
		return err
	}
	return w.Flush()
}

// timeRules is the mean time of a scan and policy evaluation of text, over
// as many runs as fill minTiming (at most maxRuns). One run is too short to
// time: the monotonic clock on Windows advances in steps of about a
// millisecond. The text is scanned after a first, untimed scan.
func timeRules(reg *scanner.Registry, pol *policy.Policy, text string) time.Duration {
	const minTiming, maxRuns = 20 * time.Millisecond, 200
	start := time.Now()
	runs := 0
	for runs < maxRuns && (runs == 0 || time.Since(start) < minTiming) {
		findings, _ := reg.ScanAll(context.Background(), []byte(text))
		pol.Evaluate(findings)
		runs++
	}
	return time.Since(start) / time.Duration(runs)
}

// score scans one text as the proxy scans a user message, and decides
// whether the classifier would be asked about it, as classifySegments does.
func score(reg *scanner.Registry, pol *policy.Policy, item input) verdict {
	findings, _ := reg.ScanAll(context.Background(), []byte(item.Text))
	action := pol.Evaluate(findings)
	elapsed := timeRules(reg, pol, item.Text)
	var injection []scanner.Finding
	flagged, signal := false, ""
	for _, f := range findings {
		if f.Type != "injection" {
			continue
		}
		injection = append(injection, f)
		if scanner.Signal(f) {
			if signal == "" {
				signal = f.Category
			}
		} else {
			flagged = true
		}
	}
	v := verdict{ID: item.ID, Action: string(action), InjectionAction: string(pol.Evaluate(injection))}
	v.ElapsedUS = elapsed.Microseconds()
	for _, f := range injection {
		v.Categories = append(v.Categories, f.Category)
	}

	// Prepared is written for every text, so that the model can also be
	// scored alone; AskClassifier says whether the proxy would send it.
	v.Prepared = classifier.Prepare(item.Text)
	if action == policy.ActionBlock || flagged || v.Prepared == "" {
		return v
	}
	if signal == "" {
		// One message: there is no earlier user text to switch script from,
		// so only a switch inside the text counts.
		if _, switched := classifier.ScriptSwitch(classifier.ScriptNone, item.Text); switched {
			signal = "script_switch"
		} else {
			signal = classifier.Structure("user", item.Text)
		}
	}
	v.AskClassifier, v.Signal = true, signal
	withFinding := append(append([]scanner.Finding(nil), injection...), scanner.Finding{
		Type: "injection", Category: "classifier", Severity: "high", Confidence: 1,
	})
	v.ClassifierAction = string(pol.Evaluate(withFinding))
	return v
}
