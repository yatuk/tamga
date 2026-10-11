package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/scanner/normalize"
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

// competitorFingerprint names a competitor list by what the scanner makes
// of it: each entry's name, patterns, severity, action and whether it is
// enabled, written so that no two different lists read the same. The order
// of the entries is left out. It is "" for an empty list.
func competitorFingerprint(specs []CompetitorSpec) string {
	if len(specs) == 0 {
		return ""
	}
	rows := make([]string, 0, len(specs))
	for _, c := range specs {
		var b strings.Builder
		b.WriteString(strconv.Quote(c.Name))
		b.WriteString(" " + strconv.Quote(c.Severity) + " " + strconv.Quote(c.Action) + " " + strconv.FormatBool(c.Enabled))
		for _, p := range c.Patterns {
			b.WriteString(" " + strconv.Quote(p))
		}
		rows = append(rows, b.String())
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

// Fingerprint names the competitor list the scanner has compiled and is
// scanning with, by digest: the list itself is not for a cache key.
func (s *CompetitorScanner) Fingerprint() string {
	s.mu.RLock()
	fp := s.fp
	s.mu.RUnlock()
	if fp == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(fp))
	return hex.EncodeToString(sum[:])
}

// snapshot returns the compiled patterns together with the fingerprint of
// the list they were compiled from, read under one lock: the two always
// belong to each other.
func (s *CompetitorScanner) snapshot() (string, []*competitorCompiled) {
	s.mu.RLock()
	fp, compiled := s.fp, s.compiled
	s.mu.RUnlock()

	if compiled == nil {
		s.mu.Lock()
		if s.compiled == nil {
			s.refreshLocked()
		}
		fp, compiled = s.fp, s.compiled
		s.mu.Unlock()
	}
	return fp, compiled
}

// patternSnapshot is what the scan cache keys this scanner's findings on
// (see patternSetScanner).
func (s *CompetitorScanner) patternSnapshot() (string, func(context.Context, []byte) ([]Finding, error)) {
	fp, compiled := s.snapshot()
	if len(compiled) == 0 {
		return "", nil
	}
	return fp, func(ctx context.Context, content []byte) ([]Finding, error) {
		return s.scanWith(ctx, compiled, content)
	}
}

// Scan checks content for competitor mentions and returns findings. The
// match is masked like every other stored match: a pattern is the analyst's,
// and what it matched in a prompt is not theirs to read back. Scan stops on
// the same limits as CustomScanner.Scan and reports them the same way.
func (s *CompetitorScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	_, compiled := s.snapshot()
	return s.scanWith(ctx, compiled, content)
}

// scanWith is Scan with the patterns given.
func (s *CompetitorScanner) scanWith(ctx context.Context, compiled []*competitorCompiled, content []byte) ([]Finding, error) {
	if len(compiled) == 0 {
		return nil, nil
	}

	var findings []Finding
	seen := map[matchedAt]bool{}
	// scanView runs the patterns over one reading of the text. Each reading
	// has the limits of a scan of its own.
	scanView := func(view normalize.View, via string) error {
		text := []byte(view.Text)
		ps := newPatternScan(ctx, s.Name(), text)
		for i := range ps.chunks {
			for _, c := range compiled {
				for _, pat := range c.res {
					ok := ps.each(pat, i, func(start, end int) {
						f := Finding{
							Type:        "competitor",
							Category:    c.name,
							Severity:    c.severity,
							Match:       maskContent(string(text[start:end])),
							StartPos:    start,
							EndPos:      end,
							Confidence:  0.95, // competitor names are distinctive
							ActionTaken: c.action,
						}
						if via != "" {
							locateInView(&f, view, via)
						}
						// The same place found again in another reading is one match.
						at := matchedAt{f.Category, f.StartPos, f.EndPos, viewOnly(f)}
						if seen[at] {
							return
						}
						seen[at] = true
						findings = append(findings, f)
					})
					if !ok {
						return ps.err
					}
				}
			}
		}
		return nil
	}
	text := string(content)
	if err := scanView(normalize.SourceView(text), ""); err != nil {
		return findings, err
	}
	// The same patterns over the text read for what its characters show:
	// a name in look-alike letters is the same name (see patternReadings).
	for _, view := range patternReadings(ctx, text) {
		if err := scanView(view, viaNormalized); err != nil {
			return findings, err
		}
	}
	return findings, nil
}
