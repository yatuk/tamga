package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// classifierReport says what the inline classifier added to a run on top of
// the rules, so the two layers can be read apart.
type classifierReport struct {
	Model     string  `json:"model"`
	Threshold float64 `json:"threshold"`
	// Asked is how many samples the rules did not block and the classifier
	// was asked about.
	Asked int `json:"asked"`
	// AddedTP are attacks the rules let through and the classifier caught;
	// AddedFP are benign samples the rules passed and the classifier flagged.
	AddedTP int `json:"added_tp"`
	AddedFP int `json:"added_fp"`
	// RulesOnly is the same corpus without the classifier, from the same run.
	RulesOnly  layerScore      `json:"rules_only"`
	Latency    latencyReport   `json:"classifier_latency"`
	ByLanguage []languageRow   `json:"by_language"`
	Flagged    []flaggedBenign `json:"benign_flagged,omitempty"`
}

type layerScore struct {
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
	TN        int     `json:"tn"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
}

// languageRow splits the result by the language of the sample. The language
// is guessed from the text (Turkish letters and common words), which is good
// enough to see whether the model works in Turkish at all, and no more.
type languageRow struct {
	Language  string  `json:"language"`
	Attacks   int     `json:"attacks"`
	Caught    int     `json:"caught"`
	Recall    float64 `json:"recall"`
	Benign    int     `json:"benign"`
	Flagged   int     `json:"flagged"`
	RulesOnly int     `json:"caught_by_rules_alone"`
}

type flaggedBenign struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

// withClassifierFinding returns the action once the classifier's finding is
// added to what the rules found.
func withClassifierFinding(pol *policy.Policy, findings []scanner.Finding, score float64) policy.Action {
	all := append(append([]scanner.Finding(nil), findings...), scanner.Finding{
		Type:       "injection",
		Category:   "classifier",
		Severity:   "high",
		Confidence: score,
	})
	if pol != nil {
		return pol.Evaluate(all)
	}
	return defaultActionForFindings(all)
}

func mitigated(a policy.Action) bool { return a != policy.ActionPass }

func scoreLayer(results []result, action func(result) policy.Action) layerScore {
	var s layerScore
	for _, r := range results {
		want, got := r.sample.ExpectedAction != "PASS", mitigated(action(r))
		switch {
		case want && got:
			s.TP++
		case want && !got:
			s.FN++
		case !want && got:
			s.FP++
		default:
			s.TN++
		}
	}
	s.Precision = safeDiv(float64(s.TP), float64(s.TP+s.FP))
	s.Recall = safeDiv(float64(s.TP), float64(s.TP+s.FN))
	return s
}

var turkishWords = []string{" bir ", " ve ", " için ", " bu ", " önceki ", " talimat", " kural", " unut", " yok say", " göster", " lütfen", " artık ", " sen ", " ile ", " ben ", " numara", " şifre", " hesab"}

func languageOf(text string) string {
	if strings.ContainsAny(text, "çğıöşüÇĞİÖŞÜ") {
		return "tr"
	}
	low := " " + strings.ToLower(text) + " "
	for _, w := range turkishWords {
		if strings.Contains(low, w) {
			return "tr"
		}
	}
	return "en"
}

func buildClassifierReport(results []result, model string, threshold float64) *classifierReport {
	rep := &classifierReport{Model: model, Threshold: threshold}
	rep.RulesOnly = scoreLayer(results, func(r result) policy.Action { return r.rulesAction })

	var asked []result
	langs := map[string]*languageRow{}
	for _, r := range results {
		lang := languageOf(r.sample.Prompt)
		row, ok := langs[lang]
		if !ok {
			row = &languageRow{Language: lang}
			langs[lang] = row
		}
		attack := r.sample.ExpectedAction != "PASS"
		if attack {
			row.Attacks++
			if mitigated(r.action) {
				row.Caught++
			}
			if mitigated(r.rulesAction) {
				row.RulesOnly++
			}
		} else {
			row.Benign++
			if mitigated(r.action) {
				row.Flagged++
			}
		}
		if !r.asked {
			continue
		}
		rep.Asked++
		asked = append(asked, result{elapsed: r.clsElapsed})
		if !mitigated(r.rulesAction) && mitigated(r.action) {
			if attack {
				rep.AddedTP++
			} else {
				rep.AddedFP++
				rep.Flagged = append(rep.Flagged, flaggedBenign{ID: r.sample.ID, Score: r.score})
			}
		}
	}
	rep.Latency = computeLatency(asked)
	for _, row := range langs {
		row.Recall = safeDiv(float64(row.Caught), float64(row.Attacks))
		rep.ByLanguage = append(rep.ByLanguage, *row)
	}
	sort.Slice(rep.ByLanguage, func(i, j int) bool { return rep.ByLanguage[i].Language < rep.ByLanguage[j].Language })
	return rep
}

func printClassifierReport(rep *classifierReport) {
	fmt.Printf("\nClassifier: %s at threshold %.2f\n", rep.Model, rep.Threshold)
	fmt.Printf("  rules alone:      precision %.3f  recall %.3f  (TP %d FP %d FN %d)\n",
		rep.RulesOnly.Precision, rep.RulesOnly.Recall, rep.RulesOnly.TP, rep.RulesOnly.FP, rep.RulesOnly.FN)
	fmt.Printf("  asked about %d samples the rules did not block: +%d attacks caught, +%d benign flagged\n",
		rep.Asked, rep.AddedTP, rep.AddedFP)
	fmt.Printf("  classifier call latency: p50 %.1f ms  p95 %.1f ms  p99 %.1f ms  max %.1f ms\n",
		rep.Latency.P50Ms, rep.Latency.P95Ms, rep.Latency.P99Ms, rep.Latency.MaxMs)
	for _, row := range rep.ByLanguage {
		fmt.Printf("  %s: attacks %d/%d caught (rules alone %d), benign flagged %d/%d\n",
			row.Language, row.Caught, row.Attacks, row.RulesOnly, row.Flagged, row.Benign)
	}
	for _, f := range rep.Flagged {
		fmt.Printf("  benign flagged: %-12s score %.3f\n", f.ID, f.Score)
	}
}

// printSweep shows what each threshold would have given on this corpus. It
// is for choosing a threshold on the tuning set; a number chosen here is
// reported on the held-out set, not on this one.
func printSweep(results []result, pol *policy.Policy) {
	fmt.Println("\nThreshold sweep (rules + classifier):")
	fmt.Println("  threshold  precision  recall   FP   FN")
	for _, thr := range []float64{0.5, 0.7, 0.8, 0.9, 0.95, 0.98, 0.99, 0.995} {
		s := scoreLayer(results, func(r result) policy.Action {
			if r.asked && r.score >= thr && r.rulesAction != policy.ActionBlock {
				return withClassifierFinding(pol, nil, r.score)
			}
			return r.rulesAction
		})
		fmt.Printf("  %9.3f  %9.3f  %6.3f  %3d  %3d\n", thr, s.Precision, s.Recall, s.FP, s.FN)
	}
}
