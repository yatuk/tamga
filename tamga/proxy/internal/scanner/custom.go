package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
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
	re         *regexp.Regexp
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

func compileCustomSpecs(specs []CustomEntitySpec) []*customCompiled {
	var out []*customCompiled
	for _, sp := range specs {
		if sp.Name == "" || sp.Pattern == "" {
			continue
		}
		re, err := regexp.Compile(sp.Pattern)
		if err != nil {
			log.Warn().Err(err).Str("component", "custom_scanner").Str("entity", sp.Name).Msg("custom entity regex invalid; skipped")
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
			re:         re,
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

func (s *CustomScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	_ = ctx
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

	var findings []Finding
	for _, c := range compiled {
		if c == nil || c.re == nil {
			continue
		}
		matches := c.re.FindAllIndex(content, -1)
		for _, loc := range matches {
			if len(loc) != 2 {
				continue
			}
			matched := string(content[loc[0]:loc[1]])
			findings = append(findings, Finding{
				Type:       "custom",
				Category:   c.name,
				Severity:   c.severity,
				Match:      maskContent(matched),
				StartPos:   loc[0],
				EndPos:     loc[1],
				Confidence: c.confidence,
			})
		}
	}
	return findings, nil
}
