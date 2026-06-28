package scanner

import (
	"bytes"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coregx/ahocorasick"
	"github.com/rs/zerolog/log"
)

// DFA build metrics — exported for Prometheus scraping.
var (
	dfaBuildCount    atomic.Int64
	dfaLastBuildMs   atomic.Int64
	dfaPatternBytes  atomic.Int64
	dfaTotalPatterns atomic.Int64
)

// DFAStats returns DFA build metrics for observability.
func DFAStats() (buildCount int64, lastBuildMs int64, patternBytes int64, totalPatterns int64) {
	return dfaBuildCount.Load(), dfaLastBuildMs.Load(), dfaPatternBytes.Load(), dfaTotalPatterns.Load()
}

type PatternMeta struct {
	Type     string
	Category string
	Severity string
}

type DFAMatch struct {
	Pattern  string
	Start    int
	End      int
	Type     string
	Category string
	Severity string
}

type DFAScanner struct {
	patterns []string
	meta     []PatternMeta
	matcher  *ahocorasick.Automaton
}

// globalDFAScanner holds the current Aho-Corasick automaton.
// Use LoadDFA() to read and ReloadDFA() to swap atomically.
// Nil-safe: ScanBytes on a nil receiver returns nil.
var globalDFAScanner atomic.Pointer[DFAScanner]

// LoadDFA returns the current DFA scanner (nil before InitDFA).
func LoadDFA() *DFAScanner {
	return globalDFAScanner.Load()
}

// buildDFAPatterns returns the canonical set of patterns and metadata for the
// Aho-Corasick DFA. InitDFA and ReloadDFA both call this to avoid duplication.
func buildDFAPatterns() ([]string, []PatternMeta) {
	patterns := make([]string, 0, 64)
	metas := make([]PatternMeta, 0, 64)

	add := func(pattern, typ, category, severity string) {
		patterns = append(patterns, strings.ToLower(pattern))
		metas = append(metas, PatternMeta{
			Type:     typ,
			Category: category,
			Severity: severity,
		})
	}

	// Secret prefixes and high-signal literals.
	add("AKIA", "secret", "aws_access_key", "critical")
	add("ASIA", "secret", "aws_access_key", "critical")
	add("ghp_", "secret", "github_token", "critical")
	add("ghs_", "secret", "github_token", "critical")
	add("sk-ant-", "secret", "anthropic_key", "critical")
	add("sk-proj-", "secret", "openai_key", "critical")
	add("xoxb-", "secret", "slack_webhook", "high")
	add("pk_live_", "secret", "stripe_key", "high")
	add("sk_live_", "secret", "stripe_key", "critical")
	add("-----begin private key-----", "secret", "private_key", "critical")
	add("-----begin rsa private key-----", "secret", "private_key", "critical")

	// Injection literals (EN + TR).
	for _, p := range injectionPatterns {
		add(p.phrase, "injection", p.category, "high")
	}

	// Context keywords for confidence scoring / future proximity.
	for _, kw := range []string{
		"credit card", "kredi kartı", "kredi kart", "card number", "cvv", "cvc",
		"kimlik", "tc kimlik", "tckn", "iban", "hesap numarası", "bank account",
		"social security", "passport", "pasaport",
	} {
		add(kw, "context", "keyword", "low")
	}

	return patterns, metas
}

// InitDFA compiles the Aho-Corasick DFA from embedded patterns.
func InitDFA() error {
	scanner, err := buildDFA(buildDFAPatterns())
	if err != nil {
		return err
	}
	globalDFAScanner.Store(scanner)
	log.Debug().Str("component", "dfa").Int("patterns", len(scanner.patterns)).Msg("Aho-Corasick DFA compiled")
	return nil
}

// ReloadDFA rebuilds the Aho-Corasick automaton and atomically swaps the
// global pointer. In-flight requests continue using the old automaton; new
// requests see the replacement. The old automaton is GC'd when no references
// remain.
//
// This is safe to call from an fsnotify handler or API endpoint while the
// proxy is serving traffic. Must be called AFTER InitDFA().
func ReloadDFA() error {
	scanner, err := buildDFA(buildDFAPatterns())
	if err != nil {
		return err
	}

	old := globalDFAScanner.Swap(scanner)
	oldPatterns := 0
	if old != nil {
		oldPatterns = len(old.patterns)
	}
	log.Debug().
		Str("component", "dfa").
		Int("patterns_old", oldPatterns).
		Int("patterns_new", len(scanner.patterns)).
		Msg("Aho-Corasick DFA hot-reloaded")
	return nil
}

// DFA pattern limits — prevent unbounded memory growth at startup.
const (
	// DFAWarnPatterns emits a structured warning when exceeded (soft limit).
	DFAWarnPatterns = 10_000
	// DFAMaxPatterns is the hard cap; InitDFA/ReloadDFA return an error above this.
	DFAMaxPatterns = 50_000
)

func buildDFA(patterns []string, metas []PatternMeta) (*DFAScanner, error) {
	if len(patterns) > DFAMaxPatterns {
		return nil, fmt.Errorf("dfa: %d patterns exceeds hard limit of %d", len(patterns), DFAMaxPatterns)
	}
	if len(patterns) > DFAWarnPatterns {
		log.Warn().Str("component", "dfa").Int("patterns", len(patterns)).Int("warn_limit", DFAWarnPatterns).Msg("DFA pattern count above soft limit")
	}

	start := time.Now()
	builder := ahocorasick.NewBuilder().
		AddStrings(patterns).
		SetASCII(false).
		SetByteClasses(true).
		SetPrefilter(true)
	automaton, err := builder.Build()
	if err != nil {
		return nil, err
	}

	elapsed := time.Since(start).Milliseconds()
	dfaBuildCount.Add(1)
	dfaLastBuildMs.Store(elapsed)
	dfaTotalPatterns.Store(int64(len(patterns)))
	// Rough estimate: each pattern ~ len(pattern) bytes + overhead
	approxBytes := int64(0)
	for _, p := range patterns {
		approxBytes += int64(len(p))
	}
	dfaPatternBytes.Store(approxBytes)

	return &DFAScanner{
		patterns: patterns,
		meta:     metas,
		matcher:  automaton,
	}, nil
}

func (s *DFAScanner) ScanBytes(content []byte) []DFAMatch {
	if s == nil || s.matcher == nil || len(content) == 0 {
		return nil
	}
	lower := bytes.ToLower(content)
	raw := s.matcher.FindAll(lower, -1)
	if len(raw) == 0 {
		return nil
	}
	out := make([]DFAMatch, 0, len(raw))
	for _, m := range raw {
		if m.PatternID < 0 || m.PatternID >= len(s.patterns) || m.PatternID >= len(s.meta) {
			continue
		}
		meta := s.meta[m.PatternID]
		out = append(out, DFAMatch{
			Pattern:  s.patterns[m.PatternID],
			Start:    m.Start,
			End:      m.End,
			Type:     meta.Type,
			Category: meta.Category,
			Severity: meta.Severity,
		})
	}
	return out
}
