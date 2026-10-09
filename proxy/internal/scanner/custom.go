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
)

// CustomEntitySpec is policy-driven input for CustomScanner (avoids scanner → policy import).
type CustomEntitySpec struct {
	Name        string
	Pattern     string
	Description string
	Severity    string
	Confidence  float64
}

type customCompiled struct {
	name       string
	pat        *scanPattern
	severity   string
	confidence float64
}

// CustomScanner runs regexp patterns from policy custom_entities (hot-reload via getter).
type CustomScanner struct {
	mu       sync.RWMutex
	getSpecs func() []CustomEntitySpec
	fp       string
	compiled []*customCompiled
}

// NewCustomScanner creates a scanner that matches policy-defined custom regex entities.
func NewCustomScanner(getSpecs func() []CustomEntitySpec) *CustomScanner {
	return &CustomScanner{getSpecs: getSpecs}
}

func (s *CustomScanner) Name() string { return "custom" }

func fingerprintSpecs(specs []CustomEntitySpec) string {
	if len(specs) == 0 {
		return ""
	}
	rows := make([]string, 0, len(specs))
	for _, sp := range specs {
		rows = append(rows, sp.Name+"\x00"+sp.Pattern+"\x00"+sp.Severity+"\x00"+strconv.FormatFloat(sp.Confidence, 'g', -1, 64))
	}
	sort.Strings(rows)
	h := sha256.Sum256([]byte(strings.Join(rows, "\n")))
	return hex.EncodeToString(h[:])
}

// compileCustomSpecs compiles the patterns the scanner will run. Validation
// refuses a pattern past the limits before it is installed; one that arrives
// some other way (a row written to the database, a policy file from an older
// release) is left out here and logged, once per reload, and not run.
func compileCustomSpecs(specs []CustomEntitySpec) []*customCompiled {
	out := []*customCompiled{}
	for _, sp := range specs {
		if sp.Name == "" || sp.Pattern == "" {
			continue
		}
		if len(out) == MaxCustomEntities+MaxStoredPatterns {
			log.Warn().Str("component", "custom_scanner").Int("patterns", len(specs)).Int("limit", len(out)).Msg("too many custom patterns; the rest are skipped")
			break
		}
		pat, err := compileScanPattern(sp.Pattern)
		if err != nil {
			log.Warn().Err(err).Str("component", "custom_scanner").Str("entity", sp.Name).Msg("custom entity pattern refused; skipped")
			continue
		}
		conf := sp.Confidence
		if conf <= 0 || conf > 1 {
			conf = 0.85
		}
		sev := strings.TrimSpace(sp.Severity)
		if sev == "" {
			sev = "medium"
		}
		out = append(out, &customCompiled{
			name:       sp.Name,
			pat:        pat,
			severity:   sev,
			confidence: conf,
		})
	}
	return out
}

func (s *CustomScanner) refreshLocked() {
	if s.getSpecs == nil {
		s.fp = ""
		s.compiled = nil
		return
	}
	specs := s.getSpecs()
	fp := fingerprintSpecs(specs)
	if fp == s.fp && s.compiled != nil {
		return
	}
	s.fp = fp
	s.compiled = compileCustomSpecs(specs)
}

// Refresh recompiles custom patterns when the spec source changes.
// Call this from a policy watcher callback rather than on every scan.
func (s *CustomScanner) Refresh() {
	s.mu.Lock()
	s.refreshLocked()
	s.mu.Unlock()
}

// Fingerprint names the patterns the scanner has compiled and is scanning
// with, which for a moment after a change are not yet the ones in the policy.
func (s *CustomScanner) Fingerprint() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fp
}

// Scan searches content for every custom pattern. It stops when the context
// ends, when the time budget is spent or at MaxFindingsPerScan findings, and
// then returns what it found together with an ErrScanDegraded error: the
// text was not searched completely, and the caller must not read the result
// as clean.
func (s *CustomScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	s.mu.RLock()
	compiled := s.compiled
	s.mu.RUnlock()

	// Lazy-init on first scan (tests and callers that don't call Refresh).
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
			if c == nil || c.pat == nil {
				continue
			}
			ok := ps.each(c.pat, i, func(start, end int) {
				findings = append(findings, Finding{
					Type:       "custom",
					Category:   c.name,
					Severity:   c.severity,
					Match:      maskContent(string(content[start:end])),
					StartPos:   start,
					EndPos:     end,
					Confidence: c.confidence,
				})
			})
			if !ok {
				return findings, ps.err
			}
		}
	}
	return findings, nil
}
