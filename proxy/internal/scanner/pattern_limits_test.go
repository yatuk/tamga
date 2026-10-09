package scanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/extract"
)

// The patterns of the review: each compiled and ran before the limits.
var refusedPatterns = []struct {
	name, pattern, limit string
}{
	{"twenty thousand alternations", strings.Repeat("(?:a|b)", 20000), "the limit is 1024"},
	{"bounded dot before a word", `(?s).{0,1000}secret`, "the limit is 500"},
	{"a thousand letters", `\p{L}{1000}`, "the limit is 500"},
	{"star", `x*`, "matches the empty string"},
	{"optional group", `(?:secret)?`, "matches the empty string"},
	{"empty alternative", `a|`, "matches the empty string"},
}

func TestCheckPatternRefusesCostlyPatterns(t *testing.T) {
	for _, tc := range refusedPatterns {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckPattern(tc.pattern)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), tc.limit) {
				t.Errorf("message does not name the limit %q: %v", tc.limit, err)
			}
			if len(err.Error()) > 300 {
				t.Errorf("message is %d bytes; it must not carry the pattern", len(err.Error()))
			}
		})
	}
}

func TestCheckPatternSyntaxErrorDoesNotQuoteThePattern(t *testing.T) {
	bad := "(" + strings.Repeat("abcdefgh", 100)
	err := CheckPattern(bad)
	if err == nil {
		t.Fatal("accepted")
	}
	if !strings.Contains(err.Error(), "missing closing )") || len(err.Error()) > 200 {
		t.Errorf("got %q", err)
	}
}

func TestCheckPatternAcceptsOrdinaryPatterns(t *testing.T) {
	for _, p := range []string{
		`MN-\d{8}`, `DSY-\d{4}/\d{6}`, `SN\d{6}`, `(?i)(ACME|PROJ)[-_][A-Z0-9]{6,}`,
		`\b[A-Z]{2}\d{6}\b`, `[A-Za-z0-9_-]{100,200}`, `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,
		`^secret`, `\p{L}{3,20}-\d{4}`, `(?s).{0,240}secret`,
	} {
		if err := CheckPattern(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	// A matcher pattern may match the empty string: it is tried against a
	// decision id, not searched for in text.
	if err := CheckMatcherPattern(`.*`); err != nil {
		t.Errorf("matcher .*: %v", err)
	}
	if err := CheckMatcherPattern(`\p{L}{1000}`); err == nil {
		t.Error("matcher pattern past the program limit accepted")
	}
}

func TestPatternLengthLimitBoundary(t *testing.T) {
	// A class of MaxPatternBytes characters is one instruction: only its
	// length can refuse it.
	at := "[" + strings.Repeat("a", MaxPatternBytes-2) + "]"
	if err := CheckPattern(at); err != nil {
		t.Errorf("at the length limit: %v", err)
	}
	over := "[" + strings.Repeat("a", MaxPatternBytes-1) + "]"
	if err := CheckPattern(over); err == nil || !strings.Contains(err.Error(), "1025 bytes, the limit is 1024") {
		t.Errorf("over the length limit: %v", err)
	}
}

func TestPatternProgramLimitBoundary(t *testing.T) {
	// "a{n}" compiles to n+2 instructions.
	if err := CheckPattern(fmt.Sprintf("a{%d}", MaxPatternProgram-2)); err != nil {
		t.Errorf("at the limit: %v", err)
	}
	if err := CheckPattern(fmt.Sprintf("a{%d}", MaxPatternProgram-1)); err == nil {
		t.Error("one instruction over the limit accepted")
	}
}

// each must report what FindAllIndex reports, empty matches aside: it
// searches match by match so that it can stop, and continuing after a match
// must not change what "^" or "\b" see.
func TestPatternScanMatchesFindAll(t *testing.T) {
	patterns := []string{
		`^a`, `\b\d{2}`, `\d{2}`, `(?i)foo`, `(a)(b)?`, `(?s)a.*b|a`, `\Bx`, `(?m)^\w+$`, `x*\b\w`,
		`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`, `ş+`, `(?i)(ACME|PROJ)[-_][A-Z0-9]{6,}`, `a\z`, `\w+\b`,
		`(?i)lakera(\s+guard)?`, `a|ab`, `(?U)a+`, `\Qa.b\E`,
	}
	inputs := []string{
		"", "a", "aaaa", "1234 56", "123456", "FOO foo fOo", "ab a abab", "aXbaa", "xx x\nline two\nx3",
		"a.b@example.com, c@d.org", "şşş aşş", "ACME-ABC123 proj_zzzzzzz", "\xff12\xfe34", "aaa\naab",
		"Lakera guard, LAKERA  Guard and lakera", "a.b axb a.b",
	}
	for _, p := range patterns {
		pat, err := compileScanPattern(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		re := regexp.MustCompile(p)
		for _, in := range inputs {
			var want [][2]int
			for _, loc := range re.FindAllIndex([]byte(in), -1) {
				if loc[1] > loc[0] {
					want = append(want, [2]int{loc[0], loc[1]})
				}
			}
			var got [][2]int
			ps := newPatternScan(context.Background(), "test", []byte(in))
			ps.each(pat, 0, func(s, e int) { got = append(got, [2]int{s, e}) })
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("%s on %q: got %v, want %v", p, in, got, want)
			}
		}
	}
}

func TestPatternScanFindsMatchesAcrossChunks(t *testing.T) {
	// One id every 100 bytes over 200 KiB: every one is found once, also
	// those that lie where two chunks overlap.
	var b strings.Builder
	n := 0
	for b.Len() < 200<<10 {
		fmt.Fprintf(&b, "MN-%08d %s\n", n, strings.Repeat("x", 87))
		n++
	}
	s := NewCustomScanner(func() []CustomEntitySpec {
		return []CustomEntitySpec{{Name: "musteri_no", Pattern: `MN-\d{8}`}}
	})
	// More ids than one scan may report; search the text in parts.
	text := b.String()
	const perPart = 900
	total := 0
	for from := 0; from < n; from += perPart {
		to := from + perPart
		if to > n {
			to = n
		}
		part := text[from*100 : to*100]
		found, err := s.Scan(context.Background(), []byte(part))
		if err != nil {
			t.Fatal(err)
		}
		seen := map[int]bool{}
		for _, f := range found {
			if seen[f.StartPos] {
				t.Fatalf("offset %d reported twice", f.StartPos)
			}
			seen[f.StartPos] = true
			if part[f.StartPos:f.StartPos+3] != "MN-" || f.EndPos-f.StartPos != 11 {
				t.Fatalf("finding at %d..%d is not an id", f.StartPos, f.EndPos)
			}
		}
		total += len(found)
	}
	if total != n {
		t.Errorf("found %d ids, want %d", total, n)
	}
}

// captureLog collects what the scanners log while fn runs.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	old := log.Logger
	log.Logger = zerolog.New(&buf)
	defer func() { log.Logger = old }()
	fn()
	return buf.String()
}

func TestCustomScannerSkipsPatternPastTheLimits(t *testing.T) {
	for _, tc := range refusedPatterns {
		t.Run(tc.name, func(t *testing.T) {
			s := NewCustomScanner(func() []CustomEntitySpec {
				return []CustomEntitySpec{
					{Name: "costly", Pattern: tc.pattern},
					{Name: "musteri_no", Pattern: `MN-\d{8}`},
				}
			})
			text := []byte("secret xxx " + strings.Repeat("ab", 2000) + " MN-12345678")
			var found []Finding
			var err error
			logged := captureLog(t, func() {
				for i := 0; i < 3; i++ {
					found, err = s.Scan(context.Background(), text)
				}
			})
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(found) != 1 || found[0].Category != "musteri_no" {
				t.Fatalf("findings: %+v", found)
			}
			if n := strings.Count(logged, "custom entity pattern refused"); n != 1 {
				t.Errorf("logged %d times over three scans, want once: %s", n, logged)
			}
			if len(tc.pattern) > patternPrefixBytes && strings.Contains(logged, tc.pattern) {
				t.Error("the log line carries the whole pattern")
			}
		})
	}
}

func TestCompetitorScannerSkipsPatternPastTheLimits(t *testing.T) {
	s := NewCompetitorScanner(func() []CompetitorSpec {
		return []CompetitorSpec{{Name: "Rival", Enabled: true, Patterns: []string{`x*`, `(?s).{0,1000}rival`, `rival`}}}
	})
	var found []Finding
	logged := captureLog(t, func() {
		for i := 0; i < 3; i++ {
			found, _ = s.Scan(context.Background(), []byte("we looked at Rival last year"))
		}
	})
	if len(found) != 1 {
		t.Fatalf("findings: %+v", found)
	}
	if n := strings.Count(logged, "competitor pattern refused"); n != 2 {
		t.Errorf("logged %d lines, want one for each of the two refused patterns: %s", n, logged)
	}
}

func TestScannerCountLimitsAtCompile(t *testing.T) {
	limit := MaxCustomEntities + MaxStoredPatterns
	specs := make([]CustomEntitySpec, limit+1)
	for i := range specs {
		specs[i] = CustomEntitySpec{Name: fmt.Sprintf("e%d", i), Pattern: fmt.Sprintf(`ID%d-\d{4}`, i)}
	}
	var got []*customCompiled
	logged := captureLog(t, func() { got = compileCustomSpecs(specs[:limit]) })
	if len(got) != limit || logged != "" {
		t.Errorf("at the limit: %d compiled, want %d; log %q", len(got), limit, logged)
	}
	logged = captureLog(t, func() { got = compileCustomSpecs(specs) })
	if len(got) != limit || !strings.Contains(logged, "too many custom patterns") {
		t.Errorf("over the limit: %d compiled, log %q", len(got), logged)
	}

	comps := make([]CompetitorSpec, MaxCompetitors+1)
	for i := range comps {
		comps[i] = CompetitorSpec{Name: fmt.Sprintf("c%d", i), Enabled: true, Patterns: []string{fmt.Sprintf("rival%d", i)}}
	}
	var cgot []*competitorCompiled
	captureLog(t, func() { cgot = compileCompetitors(comps[:MaxCompetitors]) })
	if len(cgot) != MaxCompetitors {
		t.Errorf("competitors at the limit: %d compiled", len(cgot))
	}
	logged = captureLog(t, func() { cgot = compileCompetitors(comps) })
	if len(cgot) != MaxCompetitors || !strings.Contains(logged, "too many competitors") {
		t.Errorf("competitors: %d compiled, want %d; log %q", len(cgot), MaxCompetitors, logged)
	}
	many := CompetitorSpec{Name: "c", Enabled: true}
	for i := 0; i <= MaxPatternsPerCompetitor; i++ {
		many.Patterns = append(many.Patterns, fmt.Sprintf("name%d", i))
	}
	captureLog(t, func() { cgot = compileCompetitors([]CompetitorSpec{many}) })
	if len(cgot) != 1 || len(cgot[0].res) != MaxPatternsPerCompetitor {
		t.Errorf("patterns per competitor: %+v", cgot)
	}
}

func TestCompetitorFindingCarriesMaskedMatch(t *testing.T) {
	s := NewCompetitorScanner(func() []CompetitorSpec {
		return []CompetitorSpec{{Name: "Rival", Enabled: true, Patterns: []string{`rival\s+guard\s+\w+`}}}
	})
	text := "we evaluated Rival Guard Enterprise last year"
	found, err := s.Scan(context.Background(), []byte(text))
	if err != nil || len(found) != 1 {
		t.Fatalf("findings %+v, err %v", found, err)
	}
	f := found[0]
	if f.Match != "Ri*** ***** ********se" {
		t.Errorf("match = %q", f.Match)
	}
	if strings.Contains(f.Match, "Guard") || strings.Contains(f.Match, "Enterprise") {
		t.Errorf("match carries the text: %q", f.Match)
	}
	if text[f.StartPos:f.EndPos] != "Rival Guard Enterprise" {
		t.Errorf("position %d..%d", f.StartPos, f.EndPos)
	}
}

// matchEverything returns a custom scanner whose one pattern matches every
// letter of the text.
func matchEverything() *CustomScanner {
	return NewCustomScanner(func() []CustomEntitySpec {
		return []CustomEntitySpec{{Name: "every_a", Pattern: `a`, Severity: "high"}}
	})
}

func TestFindingsCapTruncatesAndReportsDegraded(t *testing.T) {
	text := []byte(strings.Repeat("a", MaxFindingsPerScan))
	found, err := matchEverything().Scan(context.Background(), text)
	if err != nil || len(found) != MaxFindingsPerScan {
		t.Fatalf("at the cap: %d findings, err %v", len(found), err)
	}
	text = append(text, 'a')
	found, err = matchEverything().Scan(context.Background(), text)
	if len(found) != MaxFindingsPerScan {
		t.Errorf("over the cap: %d findings, want %d", len(found), MaxFindingsPerScan)
	}
	if !errors.Is(err, ErrScanDegraded) {
		t.Errorf("over the cap: err = %v, want ErrScanDegraded", err)
	}

	comp := NewCompetitorScanner(func() []CompetitorSpec {
		return []CompetitorSpec{{Name: "A", Enabled: true, Patterns: []string{"a"}}}
	})
	found, err = comp.Scan(context.Background(), text)
	if len(found) != MaxFindingsPerScan || !errors.Is(err, ErrScanDegraded) {
		t.Errorf("competitor over the cap: %d findings, err %v", len(found), err)
	}
}

// A truncated scan must reach the caller as findings plus a degraded error
// in every pipeline mode. The proxy enforces the findings and marks the
// response degraded (or refuses it under scan.on_error: block); the scan API
// answers 503. Neither reads it as a clean text.
func TestFindingsCapReachesThePipelineCaller(t *testing.T) {
	text := []byte(strings.Repeat("a", MaxFindingsPerScan+50))
	pool := NewWorkerPool(2, 16)
	defer func() { _ = pool.Shutdown(time.Second) }()
	for _, cfg := range []PipelineConfig{
		{Mode: ModeAdaptive}, {Mode: ModeSync}, {Mode: ModeAsync},
		{Mode: ModeWorkerPool, Pool: pool},
	} {
		t.Run(string(cfg.Mode), func(t *testing.T) {
			reg := NewRegistry()
			reg.Register(matchEverything())
			found, err := reg.ScanAllWithConfig(context.Background(), text, cfg)
			if !errors.Is(err, ErrScanDegraded) {
				t.Errorf("raw scan: err = %v, want ErrScanDegraded", err)
			}
			if len(found) != MaxFindingsPerScan {
				t.Errorf("raw scan: %d findings, want %d", len(found), MaxFindingsPerScan)
			}

			segs := []extract.Segment{{Role: "user", Path: "messages[0].content", Text: string(text)}}
			found, err = reg.ScanSegments(context.Background(), segs, cfg)
			if !errors.Is(err, ErrScanDegraded) {
				t.Errorf("segment scan: err = %v, want ErrScanDegraded", err)
			}
			if len(found) != MaxFindingsPerScan {
				t.Errorf("segment scan: %d findings, want %d", len(found), MaxFindingsPerScan)
			}
		})
	}
}

// worstAcceptedSpecs is the costliest set of custom patterns validation
// lets through: the most patterns, each at the program limit, each keeping
// every thread alive on any text.
func worstAcceptedSpecs(t *testing.T) []CustomEntitySpec {
	t.Helper()
	specs := make([]CustomEntitySpec, MaxCustomEntities+MaxStoredPatterns)
	for i := range specs {
		p := fmt.Sprintf(`(?s).{0,243}secret%03d`, i)
		if err := CheckPattern(p); err != nil {
			t.Fatalf("the worst pattern is not an accepted one: %v", err)
		}
		specs[i] = CustomEntitySpec{Name: fmt.Sprintf("e%d", i), Pattern: p}
	}
	return specs
}

func TestWorstAcceptedPatternSetIsBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("spends the scanner's whole time budget")
	}
	specs := worstAcceptedSpecs(t)
	s := NewCustomScanner(func() []CustomEntitySpec { return specs })
	s.Refresh()
	text := []byte(strings.Repeat("lorem ipsum dolor sit amet secre ", (1<<20)/33+1)[:1<<20])

	start := time.Now()
	_, err := s.Scan(context.Background(), text)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrScanDegraded) {
		t.Errorf("err = %v, want ErrScanDegraded: this set cannot finish 1 MiB inside the budget", err)
	}
	// The budget, plus the one search that was running when it ran out.
	if limit := patternScanBudgetMax + time.Second; elapsed > limit {
		t.Errorf("1 MiB took %v, the bound is %v", elapsed, limit)
	}
	t.Logf("worst accepted set, 1 MiB, no deadline: %v", elapsed)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start = time.Now()
	_, err = s.Scan(ctx, text)
	elapsed = time.Since(start)
	if !errors.Is(err, ErrScanDegraded) {
		t.Errorf("cancelled: err = %v, want ErrScanDegraded", err)
	}
	if elapsed > time.Second {
		t.Errorf("a context that ended after 100 ms stopped the scan after %v", elapsed)
	}
	t.Logf("worst accepted set, 1 MiB, 100 ms deadline: %v", elapsed)
}

func TestCancelledContextStopsPatternScan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	found, err := matchEverything().Scan(ctx, []byte("aaaa"))
	if len(found) != 0 || !errors.Is(err, ErrScanDegraded) {
		t.Errorf("findings %d, err %v", len(found), err)
	}
}
