package scanner

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"
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
	res      []*regexp.Regexp
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

func compileCompetitors(specs []CompetitorSpec) []*competitorCompiled {
	var out []*competitorCompiled
	for _, c := range specs {
		if !c.Enabled || c.Name == "" || len(c.Patterns) == 0 {
			continue
		}
		sev := strings.TrimSpace(c.Severity)
		if sev == "" {
			sev = "low"
		}
		action := strings.TrimSpace(c.Action)
		if action == "" {
			action = "log"
		}
		var compiled []*regexp.Regexp
		for _, p := range c.Patterns {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			// Case-insensitive matching for competitor names.
			re, err := regexp.Compile("(?i)" + p)
			if err != nil {
				continue
			}
			compiled = append(compiled, re)
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

// Scan checks content for competitor mentions and returns findings.
func (s *CompetitorScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	_ = ctx
	s.mu.RLock()
	compiled := s.compiled
	s.mu.RUnlock()

	if compiled == nil {
		s.Refresh()
		s.mu.RLock()
		compiled = s.compiled
		s.mu.RUnlock()
	}

	var findings []Finding
	for _, c := range compiled {
		for _, re := range c.res {
			matches := re.FindAllIndex(content, -1)
			for _, loc := range matches {
				if len(loc) != 2 {
					continue
				}
				matched := string(content[loc[0]:loc[1]])
				findings = append(findings, Finding{
					Type:        "competitor",
					Category:    c.name,
					Severity:    c.severity,
					Match:       matched,
					StartPos:    loc[0],
					EndPos:      loc[1],
					Confidence:  0.95, // competitor names are distinctive
					ActionTaken: c.action,
				})
			}
		}
	}
	return findings, nil
}
