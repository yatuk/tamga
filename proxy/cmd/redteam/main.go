// Command redteam runs a CSV of adversarial + benign prompts through the
// local scanner registry and prints a precision / recall summary. It is
// designed for CI use (`make redteam`) and for the dashboard Playground's
// "Red-team set" toggle.
//
// Usage:
//
//	go run ./cmd/redteam -in ./testdata/redteam/prompts.csv
//
// Exit code is non-zero when precision falls below the configured threshold,
// letting CI gate merges on detection regressions.
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// categoryBucket tracks TP/FP/FN/TN for one of the CSV's `category`
// values (e.g. `pii.credit_card`, `jailbreak.override`) so the runner
// can print a per-category precision/recall table in addition to the
// aggregate score. That lets us pinpoint regressions to specific
// detector families instead of looking at a single precision number.
type categoryBucket struct {
	TP, FP, FN, TN int
}

type sample struct {
	ID             string
	Category       string
	ExpectedAction string
	Prompt         string
}

type result struct {
	sample  sample
	action  policy.Action
	count   int
	elapsed time.Duration
}

func main() {
	inPath := flag.String("in", "testdata/redteam/prompts.csv", "CSV path")
	policyPath := flag.String("policy", "tamga-policy.yaml", "Policy file (optional)")
	minPrecision := flag.Float64("min-precision", 0.70, "Minimum precision before exit=1")
	minRecall := flag.Float64("min-recall", 0.70, "Minimum recall before exit=1")
	verbose := flag.Bool("v", false, "Print per-sample results")
	jsonOut := flag.String("json", "", "Optional path to write a machine-readable benchmark report (JSON)")
	flag.Parse()

	samples, err := loadCSV(*inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load csv: %v\n", err)
		os.Exit(2)
	}

	reg := buildRegistry()
	pol := loadPolicy(*policyPath)

	var results []result
	for _, s := range samples {
		start := time.Now()
		findings, _ := reg.ScanAll(context.Background(), []byte(s.Prompt))
		var act policy.Action
		if pol != nil {
			act = pol.Evaluate(findings)
		} else {
			act = defaultActionForFindings(findings)
		}
		results = append(results, result{sample: s, action: act, count: len(findings), elapsed: time.Since(start)})
	}

	tp, fp, fn, tn := 0, 0, 0, 0
	buckets := map[string]*categoryBucket{}
	for _, r := range results {
		wantMitigated := r.sample.ExpectedAction != "PASS"
		gotMitigated := r.action != policy.ActionPass
		cat := r.sample.Category
		if cat == "" {
			cat = "uncategorised"
		}
		if _, ok := buckets[cat]; !ok {
			buckets[cat] = &categoryBucket{}
		}
		switch {
		case wantMitigated && gotMitigated:
			tp++
			buckets[cat].TP++
		case wantMitigated && !gotMitigated:
			fn++
			buckets[cat].FN++
		case !wantMitigated && gotMitigated:
			fp++
			buckets[cat].FP++
		default:
			tn++
			buckets[cat].TN++
		}
		if *verbose {
			fmt.Printf("  %-10s  want=%-6s  got=%-6s  findings=%d  %s\n",
				r.sample.ID, r.sample.ExpectedAction, r.action, r.count, truncate(r.sample.Prompt, 60))
		}
	}

	precision := safeDiv(float64(tp), float64(tp+fp))
	recall := safeDiv(float64(tp), float64(tp+fn))
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}

	fmt.Printf("\nSamples: %d   TP: %d   FP: %d   FN: %d   TN: %d\n", len(results), tp, fp, fn, tn)
	fmt.Printf("Precision: %.3f   Recall: %.3f   F1: %.3f\n", precision, recall, f1)
	printBucketTable(buckets)

	if strings.TrimSpace(*jsonOut) != "" {
		if err := writeJSONReport(*jsonOut, *inPath, results, buckets, precision, recall, f1, tp, fp, fn, tn); err != nil {
			fmt.Fprintf(os.Stderr, "write json: %v\n", err)
			os.Exit(2)
		}
	}

	if precision < *minPrecision || recall < *minRecall {
		fmt.Fprintf(os.Stderr, "\nFAIL: precision or recall below threshold (min_precision=%.2f, min_recall=%.2f)\n",
			*minPrecision, *minRecall)
		os.Exit(1)
	}
}

func loadCSV(path string) ([]sample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
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
		// Unescape CSV-newlines used to cram many-shot samples.
		prompt := strings.ReplaceAll(rec[3], `\n`, "\n")
		out = append(out, sample{ID: rec[0], Category: rec[1], ExpectedAction: strings.ToUpper(strings.TrimSpace(rec[2])), Prompt: prompt})
	}
	return out, nil
}

func buildRegistry() *scanner.Registry {
	reg := scanner.NewRegistry()
	reg.Register(scanner.NewPIIScanner())
	reg.Register(scanner.NewSecretScanner())
	reg.Register(scanner.NewInjectionScanner())
	reg.Register(scanner.NewJailbreakScanner())
	_ = scanner.InitDFA()
	return reg
}

func loadPolicy(path string) *policy.Policy {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	p, err := policy.LoadFromFile(path)
	if err != nil {
		return nil
	}
	return p
}

func defaultActionForFindings(findings []scanner.Finding) policy.Action {
	if len(findings) == 0 {
		return policy.ActionPass
	}
	best := policy.ActionLog
	for _, f := range findings {
		switch f.Severity {
		case "critical":
			return policy.ActionBlock
		case "high":
			if best != policy.ActionBlock {
				best = policy.ActionRedact
			}
		case "medium":
			if best == policy.ActionLog {
				best = policy.ActionWarn
			}
		}
	}
	return best
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// printBucketTable emits a per-category precision/recall table sorted
// by category name so it reads like a stable diff across CI runs.
func printBucketTable(buckets map[string]*categoryBucket) {
	if len(buckets) == 0 {
		return
	}
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println()
	fmt.Printf("  %-28s  %6s  %3s  %3s  %3s  %3s  %9s  %6s  %6s\n",
		"category", "N", "TP", "FP", "FN", "TN", "precision", "recall", "F1")
	fmt.Println("  " + strings.Repeat("-", 82))
	for _, k := range keys {
		b := buckets[k]
		n := b.TP + b.FP + b.FN + b.TN
		prec := safeDiv(float64(b.TP), float64(b.TP+b.FP))
		rec := safeDiv(float64(b.TP), float64(b.TP+b.FN))
		f1 := 0.0
		if prec+rec > 0 {
			f1 = 2 * prec * rec / (prec + rec)
		}
		fmt.Printf("  %-28s  %6d  %3d  %3d  %3d  %3d  %9.3f  %6.3f  %6.3f\n",
			k, n, b.TP, b.FP, b.FN, b.TN, prec, rec, f1)
	}
}

// writeJSONReport serialises an aggregate + per-category benchmark report
// to disk. The shape is intentionally flat and stable so external
// dashboards (and our own marketing /evals page) can consume it without
// schema guessing.
func writeJSONReport(path, corpus string, results []result, buckets map[string]*categoryBucket, precision, recall, f1 float64, tp, fp, fn, tn int) error {
	type catRow struct {
		Category  string  `json:"category"`
		N         int     `json:"n"`
		TP        int     `json:"tp"`
		FP        int     `json:"fp"`
		FN        int     `json:"fn"`
		TN        int     `json:"tn"`
		Precision float64 `json:"precision"`
		Recall    float64 `json:"recall"`
		F1        float64 `json:"f1"`
	}
	type report struct {
		GeneratedAt string        `json:"generated_at"`
		Corpus      string        `json:"corpus"`
		Samples     int           `json:"samples"`
		TP          int           `json:"tp"`
		FP          int           `json:"fp"`
		FN          int           `json:"fn"`
		TN          int           `json:"tn"`
		Precision   float64       `json:"precision"`
		Recall      float64       `json:"recall"`
		F1          float64       `json:"f1"`
		Latency     latencyReport `json:"scan_latency"`
		Categories  []catRow      `json:"categories"`
	}

	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	cats := make([]catRow, 0, len(keys))
	for _, k := range keys {
		b := buckets[k]
		n := b.TP + b.FP + b.FN + b.TN
		prec := safeDiv(float64(b.TP), float64(b.TP+b.FP))
		rec := safeDiv(float64(b.TP), float64(b.TP+b.FN))
		bf1 := 0.0
		if prec+rec > 0 {
			bf1 = 2 * prec * rec / (prec + rec)
		}
		cats = append(cats, catRow{Category: k, N: n, TP: b.TP, FP: b.FP, FN: b.FN, TN: b.TN, Precision: prec, Recall: rec, F1: bf1})
	}

	lat := computeLatency(results)
	rep := report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Corpus:      filepath.Base(corpus),
		Samples:     len(results),
		TP:          tp, FP: fp, FN: fn, TN: tn,
		Precision: precision, Recall: recall, F1: f1,
		Latency:    lat,
		Categories: cats,
	}

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o750)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// latencyReport captures inline-scan wall-clock percentiles across the
// corpus so the published benchmark tells the whole story: accuracy AND
// the latency at which that accuracy is achieved.
type latencyReport struct {
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
	P99Ms float64 `json:"p99_ms"`
	MaxMs float64 `json:"max_ms"`
}

func computeLatency(results []result) (out latencyReport) {
	if len(results) == 0 {
		return
	}
	ds := make([]float64, 0, len(results))
	for _, r := range results {
		ds = append(ds, float64(r.elapsed.Microseconds())/1000.0)
	}
	sort.Float64s(ds)
	pick := func(p float64) float64 {
		idx := int(float64(len(ds)-1) * p)
		return ds[idx]
	}
	out.P50Ms = pick(0.50)
	out.P95Ms = pick(0.95)
	out.P99Ms = pick(0.99)
	out.MaxMs = ds[len(ds)-1]
	return
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
