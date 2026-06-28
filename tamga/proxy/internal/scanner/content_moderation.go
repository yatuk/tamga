// Package scanner — content moderation scanner (toxicity, refusal, banned code/topics).
//
// Ported from the Python analyzer's heuristic fallback in toxicity.py.
// These are pure regex patterns that run inline in the Go proxy at <1ms
// latency — no ML/LLM required. The Python analyzer retains the LLM Guard
// ML-based classifiers for cases that genuinely need semantic reasoning.
package scanner

import (
	"context"
	"regexp"
	"sync"
)

func init() {
	compileModerationPatterns()
}

// ── Pattern categories ──────────────────────────────────────────────────────

type modPattern struct {
	Regex      *regexp.Regexp
	Category   string
	Severity   string
	Confidence float64
}

var (
	modPatterns     []modPattern
	modPatternsOnce sync.Once
)

func compileModerationPatterns() {
	modPatternsOnce.Do(func() {
		modPatterns = make([]modPattern, 0, 60)

		add := func(expr, cat, sev string, conf float64) {
			modPatterns = append(modPatterns, modPattern{
				Regex:      regexp.MustCompile(expr),
				Category:   cat,
				Severity:   sev,
				Confidence: conf,
			})
		}

		// ── Toxicity / Hate Speech (EN + TR) ──────────────────────────────
		add(`(?i)\b(kill\s+(yourself|urself|all\s+\w+))\b`, "violence_threat", "high", 0.92)
		add(`(?i)\b(i\s+hope\s+you\s+die|i.ll\s+kill\s+you)\b`, "violence_threat", "high", 0.92)
		add(`(?i)\b(fuck\s+you|go\s+to\s+hell|piece\s+of\s+shit)\b`, "profanity", "medium", 0.75)
		add(`(?i)\b(n[i1]gg[e3]r|k[i1]k[e3]|sp[i1]c|ch[i1]nk|f[a@]gg[o0]t|r[e3]t[a@]rd)\b`, "hate_speech", "high", 0.92)
		add(`(?i)\b(ter[öo]rist|cihad[çc]?[iı]\s*(il[ea]n|ça[ğg]r[ıi]s[ıi]))\b`, "extremism", "high", 0.92)

		// Turkish hate speech / profanity
		add(`(?i)\b(am[kıi]na?\s*koy[ae]y[ıi]m|s[iİ]kt[ıi]r\s*git|yarra[kğ][ıi]\s*ye)\b`, "profanity_tr", "high", 0.92)
		add(`(?i)\b(öl[üu]m\s*(dile|tehdit)|geber|katil\s*ol)\b`, "violence_threat_tr", "high", 0.92)
		add(`(?i)\b(k[üu]rt\s*(pis|it|köpek)|ermeni\s*(it|köpek))\b`, "hate_speech_tr", "high", 0.92)
		add(`(?i)\b(oro[sş]pu|pi[çc]|kahp[ei]|pezevenk|h[ae]yvan\s*o[ğg]lu)\b`, "profanity_tr", "high", 0.92)

		// CSAM / child safety (critical)
		add(`(?i)\b(child\s*porn|underage|csa[mr]?\s|pedophil)`, "csam_related", "critical", 0.95)
		add(`(?i)\b(çocuk\s*(porno|istismar|taciz)|reşit\s*olmayan)\b`, "csam_related_tr", "critical", 0.95)

		// ── Refusal Detection ──────────────────────────────────────────────
		add(`(?i)\b(I\s+(am\s+sorry|can.?t|cannot|won.?t|will\s+not)\s+(help|assist|provide|answer|comply|generate|respond))\b`, "refusal", "medium", 0.85)
		add(`(?i)\b(As\s+an\s+AI\s+(language\s+)?model)\b`, "refusal", "low", 0.55)
		add(`(?i)\b(Üzgünüm.*(yardımcı|cevap|yanıt).*olam[ae][mz])\b`, "refusal_tr", "medium", 0.85)
		add(`(?i)\b(Ben\s+bir\s+yapay\s+zeka\s+(modeliyim|asistanıyım))\b`, "refusal_tr", "low", 0.55)

		// ── Banned Code (malware, exploits, injection) ────────────────────
		add(`(?i)\b(import\s+(os|subprocess|socket|requests).*exec\()`, "unsafe_code_exec", "high", 0.90)
		add(`(?i)\b(rm\s+-rf\s+/|del\s+/[Ff].*system32|format\s+[cC]:)`, "destructive_command", "high", 0.95)
		add(`(?i)\b(eval\(.*__import__|exec\(.*compile\()`, "code_injection", "high", 0.90)
		add(`(?i)\b((?:UNION\s+)?SELECT\s+.*FROM\s+.*--|DROP\s+TABLE\s+|1\s*=\s*1\s*--)`, "sql_injection_code", "high", 0.90)
		add(`(?i)\b(<script>.*alert|document\.cookie\b|onerror\s*=\s*alert)`, "xss_code", "high", 0.90)
		add(`(?i)\b(ransomware|keylogger|botnet|trojan.*payload|backdoor.*reverse\s*shell)\b`, "malware_discussion", "medium", 0.80)
		add(`(?i)\b(how\s+to\s+(hack|crack|exploit|bypass|brute\s*force|phish))\b`, "hacking_howto", "medium", 0.78)

		// ── Banned Topics (weapons, drugs, self-harm) ─────────────────────
		add(`(?i)\b(how\s+to\s+(make|build|create|manufacture)\s+(a\s+)?(bomb|weapon|explosive|poison|ricin|anthrax))\b`, "weapons_manufacturing", "high", 0.95)
		add(`(?i)\b(suicide\s+method|best\s+way\s+to\s+(die|kill\s+myself))\b`, "self_harm_method", "critical", 0.95)
		add(`(?i)\b(nasıl\s+(intihar|bomba|uyuşturucu)\s*(edilir|yapılır))\b`, "forbidden_topics_tr", "high", 0.92)
		add(`(?i)\b(where\s+to\s+buy\s+(drugs|heroin|cocaine|meth|fentanyl))\b`, "illegal_drugs", "high", 0.92)

		// ── Enhanced PII (not yet in pii.go) ──────────────────────────────
		add(`(?i)\b([A-Z]<?\d{7,9}|[A-Z]\d{8})\b`, "passport_number", "high", 0.70)                      // US/TR passport format
		add(`(?i)\b(passport\s*(no|number|#)?[:.\s]*[A-Z0-9]{6,12})\b`, "passport_number", "high", 0.85) // with context word
		add(`(?i)\b((?:DOB|birth|born)\s*(?:date\s*)?[:.\s]*\d{1,2}[/-]\d{1,2}[/-]\d{2,4})\b`, "date_of_birth", "high", 0.75)
		add(`(?i)\b(\d{1,2}[/-]\d{1,2}[/-]\d{2,4})\b`, "date_of_birth", "medium", 0.55) // bare date (noisy)
		add(`(?i)\b(medical\s*record\s*(?:number|no|#)?[:.\s]*\d{4,12})\b`, "medical_record", "high", 0.80)
		add(`(?i)\b(NPI[:.\s]*\d{10}|National\s+Provider\s+ID[:.\s]*\d{10})\b`, "npi_number", "high", 0.85)
		add(`(?i)\b(DEA\s*(?:number|#)?[:.\s]*[A-Z]{2}\d{7})\b`, "dea_number", "high", 0.85)
	})
}

// ── Scanner ──────────────────────────────────────────────────────────────────

// ContentModerationScanner detects toxic, harmful, or policy-violating
// content using compiled regex patterns. It runs inline in the Go proxy
// at <1ms latency — no external service call needed.
type ContentModerationScanner struct{}

// NewContentModerationScanner returns a ready-to-use scanner.
func NewContentModerationScanner() *ContentModerationScanner {
	return &ContentModerationScanner{}
}

func (s *ContentModerationScanner) Name() string { return "content_moderation" }

func (s *ContentModerationScanner) Scan(_ context.Context, content []byte) ([]Finding, error) {
	if len(content) == 0 {
		return nil, nil
	}

	compileModerationPatterns()
	seen := make(map[string]struct{}, 16)
	var findings []Finding

	text := string(content)
	for _, p := range modPatterns {
		matches := p.Regex.FindAllString(text, -1)
		for _, m := range matches {
			dedupKey := p.Category + ":" + truncateStr(m, 60)
			if _, ok := seen[dedupKey]; ok {
				continue
			}
			seen[dedupKey] = struct{}{}

			findings = append(findings, Finding{
				Type:       "content_moderation",
				Category:   p.Category,
				Severity:   p.Severity,
				Match:      truncateStr(m, 200),
				Confidence: p.Confidence,
			})
		}
	}

	return findings, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
