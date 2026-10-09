package scanner

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// CompetitorSpec is a competitor detection rule from policy (avoids scanner → policy import).
type CompetitorSpec struct {
	Name     string
	Patterns []string
	Severity string
	Action   string
	Enabled  bool
}

// competitorCompiled holds the pre-compiled regex patterns for one competitor.
type competitorCompiled struct {
	name     string
	res      []*scanPattern
	severity string
	action   string
}

// CompetitorScanner detects competitor brand/product mentions in LLM prompts.
// It is policy-driven and supports hot-reload via a getter function.
type CompetitorScanner struct {
	mu       sync.RWMutex
	getSpecs func() []CompetitorSpec
	fp       string
	compiled []*competitorCompiled
}

// NewCompetitorScanner creates a competitor scanner backed by a spec getter.
// The getter is called on each Refresh to pick up policy changes.
func NewCompetitorScanner(getSpecs func() []CompetitorSpec) *CompetitorScanner {
	return &CompetitorScanner{getSpecs: getSpecs}
}

func (s *CompetitorScanner) Name() string { return "competitor" }

func competitorFingerprint(specs []CompetitorSpec) string {
	if len(specs) == 0 {
		return ""
	}
	rows := make([]string, 0, len(specs))
	for _, c := range specs {
		pat := strings.Join(c.Patterns, "\x00")
		rows = append(rows, c.Name+"\x00"+pat+"\x00"+c.Severity+"\x00"+c.Action)
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

// compileCompetitors compiles the patterns the scanner will run. A pattern
// or a competitor past the limits is left out and logged, once per reload,
// and not run; see compileCustomSpecs.
func compileCompetitors(specs []CompetitorSpec) []*competitorCompiled {
	out := []*competitorCompiled{}
	for _, c := range specs {
		if !c.Enabled || c.Name == "" || len(c.Patterns) == 0 {
			continue
		}
		if len(out) == MaxCompetitors {
			log.Warn().Str("component", "competitor_scanner").Int("competitors", len(specs)).Int("limit", MaxCompetitors).Msg("too many competitors; the rest are skipped")
			break
		}
		sev := strings.TrimSpace(c.Severity)
		if sev == "" {
			sev = "low"
		}
		action := strings.TrimSpace(c.Action)
		if action == "" {
			action = "log"
		}
		var compiled []*scanPattern
		for _, p := range c.Patterns {
			if strings.TrimSpace(p) == "" {
				continue
			}
			if len(compiled) == MaxPatternsPerCompetitor {
				log.Warn().Str("component", "competitor_scanner").Str("competitor", c.Name).Int("limit", MaxPatternsPerCompetitor).Msg("too many patterns for one competitor; the rest are skipped")
				break
			}
			// Case-insensitive matching for competitor names.
			pat, err := compileScanPattern(competitorPattern(p))
			if err != nil {
				log.Warn().Err(err).Str("component", "competitor_scanner").Str("competitor", c.Name).Msg("competitor pattern refused; skipped")
				continue
			}
			compiled = append(compiled, pat)
		}
		if len(compiled) == 0 {
			continue
		}
		out = append(out, &competitorCompiled{
			name:     c.Name,
			res:      compiled,
			severity: sev,
			action:   action,
		})
	}
	return out
}

func (s *CompetitorScanner) refreshLocked() {
	if s.getSpecs == nil {
		s.fp = ""
		s.compiled = nil
		return
	}
	specs := s.getSpecs()
	fp := competitorFingerprint(specs)
	if fp == s.fp && s.compiled != nil {
		return
	}
	s.fp = fp
	s.compiled = compileCompetitors(specs)
}

// Refresh recompiles competitor patterns from the current policy.
// Call from a policy reload callback, not on every scan.
func (s *CompetitorScanner) Refresh() {
	s.mu.Lock()
	s.refreshLocked()
	s.mu.Unlock()
}

// Scan checks content for competitor mentions and returns findings. The
// match is masked like every other stored match: a pattern is the analyst's,
// and what it matched in a prompt is not theirs to read back. Scan stops on
// the same limits as CustomScanner.Scan and reports them the same way.
func (s *CompetitorScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	s.mu.RLock()
	compiled := s.compiled
	s.mu.RUnlock()

	if compiled == nil {
		s.Refresh()
		s.mu.RLock()
		compiled = s.compiled
		s.mu.RUnlock()
	}
	if len(compiled) == 0 {
		return nil, nil
	}

	var findings []Finding
	ps := newPatternScan(ctx, s.Name(), content)
	for i := range ps.chunks {
		for _, c := range compiled {
			for _, pat := range c.res {
				ok := ps.each(pat, i, func(start, end int) {
					findings = append(findings, Finding{
						Type:        "competitor",
						Category:    c.name,
						Severity:    c.severity,
						Match:       maskContent(string(content[start:end])),
						StartPos:    start,
						EndPos:      end,
						Confidence:  0.95, // competitor names are distinctive
						ActionTaken: c.action,
					})
				})
				if !ok {
					return findings, ps.err
				}
			}
		}
	}
	return findings, nil
}
