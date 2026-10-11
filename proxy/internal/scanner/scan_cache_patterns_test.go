package scanner

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/yatuk/tamga/internal/scanner/normalize"
)

// specBox is a policy whose entries a test changes while a scanner reads it.
type specBox[T any] struct {
	mu    sync.Mutex
	specs []T
}

func (b *specBox[T]) get() []T {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]T(nil), b.specs...)
}

func (b *specBox[T]) set(specs ...T) {
	b.mu.Lock()
	b.specs = specs
	b.mu.Unlock()
}

// patternText is long enough to be remembered and holds one value for each
// of two patterns.
func patternText() []byte {
	return []byte(proseText(3<<10, 7) + " customer CUST-123456 placed order ORD-9911 with Globex. " + proseText(1<<10, 8))
}

func scanWith(t *testing.T, reg *Registry, text []byte) []Finding {
	t.Helper()
	found, err := reg.ScanAllWithConfig(context.Background(), text, PipelineConfig{Mode: ModeSync})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func catSevAct(found []Finding) string {
	var out []string
	for _, f := range found {
		out = append(out, f.Category+"/"+f.Severity+"/"+f.ActionTaken)
	}
	return strings.Join(out, " ")
}

// The same text under the same patterns is answered from memory.
func TestScanCache_UnchangedPatternSetIsServedFromTheCache(t *testing.T) {
	ResetScanCache()
	custom := &specBox[CustomEntitySpec]{}
	custom.set(CustomEntitySpec{Name: "customer_no", Pattern: `\bCUST-\d{6}\b`, Severity: "high"})
	comp := &specBox[CompetitorSpec]{}
	comp.set(CompetitorSpec{Name: "globex", Patterns: []string{`globex`}, Severity: "low", Action: "log", Enabled: true})
	reg := NewRegistry()
	reg.Register(NewCustomScanner(custom.get))
	reg.Register(NewCompetitorScanner(comp.get))
	text := patternText()

	h0, m0 := ScanCacheStats()
	first := scanWith(t, reg, text)
	h1, m1 := ScanCacheStats()
	if h1 != h0 || m1 != m0+2 {
		t.Fatalf("first scan: hits %d->%d, misses %d->%d, want two misses", h0, h1, m0, m1)
	}
	second := scanWith(t, reg, text)
	h2, m2 := ScanCacheStats()
	if h2 != h1+2 || m2 != m1 {
		t.Fatalf("second scan: hits %d->%d, misses %d->%d, want two hits", h1, h2, m1, m2)
	}
	if catSevAct(first) != "customer_no/high/ globex/low/log" || !reflect.DeepEqual(first, second) {
		t.Fatalf("findings differ:\n%+v\n%+v", first, second)
	}

	// A refresh that finds the same entries, in another order or not, keeps
	// the key.
	for _, s := range []Refreshable{reg.scanners[0].(Refreshable), reg.scanners[1].(Refreshable)} {
		s.Refresh()
	}
	scanWith(t, reg, text)
	if h3, m3 := ScanCacheStats(); h3 != h2+2 || m3 != m2 {
		t.Fatalf("after a refresh without a change: hits %d->%d, misses %d->%d", h2, h3, m2, m3)
	}
}

// One pattern changed: the next scan of the same text is made with the new
// pattern, not answered with what the old one found.
func TestScanCache_AChangedPatternIsNotServedFromTheStaleEntry(t *testing.T) {
	ResetScanCache()
	box := &specBox[CustomEntitySpec]{}
	keep := CustomEntitySpec{Name: "order_no", Pattern: `\bORD-\d{4}\b`, Severity: "medium"}
	box.set(CustomEntitySpec{Name: "customer_no", Pattern: `\bCUST-\d{6}\b`, Severity: "high"}, keep)
	s := NewCustomScanner(box.get)
	reg := NewRegistry()
	reg.Register(s)
	text := patternText()

	if got := catSevAct(scanWith(t, reg, text)); got != "customer_no/high/ order_no/medium/" {
		t.Fatalf("before the change: %s", got)
	}
	scanWith(t, reg, text) // now in the cache
	hBefore, mBefore := ScanCacheStats()

	steps := []struct {
		name string
		spec CustomEntitySpec
		want string
	}{
		{"pattern", CustomEntitySpec{Name: "customer_no", Pattern: `\bCUST-\d{9}\b`, Severity: "high"}, "order_no/medium/"},
		{"severity", CustomEntitySpec{Name: "customer_no", Pattern: `\bCUST-\d{6}\b`, Severity: "critical"}, "customer_no/critical/ order_no/medium/"},
		{"name", CustomEntitySpec{Name: "musteri_no", Pattern: `\bCUST-\d{6}\b`, Severity: "high"}, "musteri_no/high/ order_no/medium/"},
		{"confidence", CustomEntitySpec{Name: "customer_no", Pattern: `\bCUST-\d{6}\b`, Severity: "high", Confidence: 0.5}, "customer_no/high/ order_no/medium/"},
	}
	for _, step := range steps {
		box.set(step.spec, keep)
		s.Refresh()
		found := scanWith(t, reg, text)
		h, m := ScanCacheStats()
		if h != hBefore || m != mBefore+1 {
			t.Fatalf("%s changed: hits %d->%d, misses %d->%d, want a miss", step.name, hBefore, h, mBefore, m)
		}
		if got := catSevAct(found); got != step.want {
			t.Fatalf("%s changed: findings %s, want %s", step.name, got, step.want)
		}
		if step.name == "confidence" && found[0].Confidence != 0.5 {
			t.Fatalf("confidence = %v", found[0].Confidence)
		}
		mBefore = m
	}

	// An entry added, and one removed.
	box.set(keep)
	s.Refresh()
	if got := catSevAct(scanWith(t, reg, text)); got != "order_no/medium/" {
		t.Fatalf("entry removed: %s", got)
	}
	box.set()
	s.Refresh()
	if found := scanWith(t, reg, text); len(found) != 0 {
		t.Fatalf("no entries: %+v", found)
	}

	// Back to the first set: what was found under it is still right for it.
	box.set(CustomEntitySpec{Name: "customer_no", Pattern: `\bCUST-\d{6}\b`, Severity: "high"}, keep)
	s.Refresh()
	h0, _ := ScanCacheStats()
	if got := catSevAct(scanWith(t, reg, text)); got != "customer_no/high/ order_no/medium/" {
		t.Fatalf("first set again: %s", got)
	}
	if h1, _ := ScanCacheStats(); h1 != h0+1 {
		t.Fatal("the first set, in force again, was scanned again")
	}
}

// The key names what the scanner has compiled. Between a change of the policy
// and the refresh that follows it, the scanner still scans with the old
// patterns; what it finds then is stored under the old fingerprint and never
// under the new one.
func TestScanCache_KeyFollowsTheCompiledSetNotThePolicy(t *testing.T) {
	ResetScanCache()
	box := &specBox[CustomEntitySpec]{}
	box.set(CustomEntitySpec{Name: "customer_no", Pattern: `\bCUST-\d{6}\b`, Severity: "high"})
	s := NewCustomScanner(box.get)
	reg := NewRegistry()
	reg.Register(s)
	text := patternText()
	scanWith(t, reg, text)

	box.set(CustomEntitySpec{Name: "order_no", Pattern: `\bORD-\d{4}\b`, Severity: "medium"})
	if got := catSevAct(scanWith(t, reg, text)); got != "customer_no/high/" {
		t.Fatalf("before the refresh the old patterns are in force: %s", got)
	}
	s.Refresh()
	if got := catSevAct(scanWith(t, reg, text)); got != "order_no/medium/" {
		t.Fatalf("after the refresh: %s", got)
	}
}

func TestScanCache_CompetitorListChanges(t *testing.T) {
	ResetScanCache()
	box := &specBox[CompetitorSpec]{}
	globex := CompetitorSpec{Name: "globex", Patterns: []string{`globex`}, Severity: "low", Action: "log", Enabled: true}
	box.set(globex)
	s := NewCompetitorScanner(box.get)
	reg := NewRegistry()
	reg.Register(s)
	text := patternText()

	if got := catSevAct(scanWith(t, reg, text)); got != "globex/low/log" {
		t.Fatalf("start: %s", got)
	}
	change := func(name, want string, edit func(c *CompetitorSpec)) {
		t.Helper()
		c := globex
		c.Patterns = append([]string(nil), globex.Patterns...)
		edit(&c)
		box.set(c)
		s.Refresh()
		h0, _ := ScanCacheStats()
		got := catSevAct(scanWith(t, reg, text))
		if h1, _ := ScanCacheStats(); h1 != h0 {
			t.Fatalf("%s changed and the scan was answered from the cache", name)
		}
		if got != want {
			t.Fatalf("%s changed: %q, want %q", name, got, want)
		}
	}
	change("action", "globex/low/block", func(c *CompetitorSpec) { c.Action = "block" })
	change("severity", "globex/high/log", func(c *CompetitorSpec) { c.Severity = "high" })
	change("pattern", "", func(c *CompetitorSpec) { c.Patterns[0] = `globex\s+ai` })
	change("pattern added", "globex/low/log globex/low/log", func(c *CompetitorSpec) { c.Patterns = append(c.Patterns, `ORD-\d+`) })
	// Switching an entry off used to leave its patterns compiled: the
	// fingerprint did not include the switch.
	change("enabled", "", func(c *CompetitorSpec) { c.Enabled = false })

	box.set(globex)
	s.Refresh()
	h0, _ := ScanCacheStats()
	if got := catSevAct(scanWith(t, reg, text)); got != "globex/low/log" {
		t.Fatalf("first list again: %s", got)
	}
	if h1, _ := ScanCacheStats(); h1 != h0+1 {
		t.Fatal("the first list, in force again, was scanned again")
	}
}

// Two sets that differ only in where a separator falls are two sets.
func TestPatternFingerprints_NoTwoSetsReadTheSame(t *testing.T) {
	a := fingerprintSpecs([]CustomEntitySpec{{Name: "a", Pattern: "b\x00c"}})
	b := fingerprintSpecs([]CustomEntitySpec{{Name: "a\x00b", Pattern: "c"}})
	if a == b {
		t.Fatal("custom: two different sets have one fingerprint")
	}
	c := competitorFingerprint([]CompetitorSpec{{Name: "x", Patterns: []string{"a", "b"}, Enabled: true}})
	d := competitorFingerprint([]CompetitorSpec{{Name: "x", Patterns: []string{"a\x00b"}, Enabled: true}})
	e := competitorFingerprint([]CompetitorSpec{{Name: "x", Patterns: []string{"a", "b"}}})
	if c == d || c == e {
		t.Fatal("competitor: two different lists have one fingerprint")
	}
	// The order of the entries is not part of it.
	one := []CustomEntitySpec{{Name: "a", Pattern: "x"}, {Name: "b", Pattern: "y"}}
	two := []CustomEntitySpec{one[1], one[0]}
	if fingerprintSpecs(one) != fingerprintSpecs(two) {
		t.Fatal("custom: the order of the entries changed the fingerprint")
	}
}

// What the scanners take from the pipeline run is what the normaliser gives
// them when asked directly.
func TestReadingsFor_SharedReadingsAreTheNormalisersOwn(t *testing.T) {
	pii := normalize.Default()
	pii.Deleet = false
	for _, text := range []string{
		"plain ascii text with nothing in it",
		"Müşteri numarası ＣＵＳＴ-１２３４５６ ve e-posta ayse@example.com",
		"zero\u200bwidth and &#49;&#48; entities, bir iki üç dört",
		"payload MTAwMDAwMDAxNDY= end, 1gn0r3 4ll pr3v10us, i g n o r e",
		"",
	} {
		ctx := withScanShared(context.Background(), []byte(text))
		// Asked for in every order a pipeline can ask in.
		for _, opts := range []normalize.Options{patternReadingOptions(), normalize.Default(), pii, patternReadingOptions(), pii} {
			got := readingsFor(ctx, text, opts)
			want := normalize.ApplyViews(text, opts)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%q under %+v:\n got %+v\nwant %+v", text, opts, got, want)
			}
		}
		// Another text under the same context gets its own readings.
		other := text + " x"
		if got, want := readingsFor(ctx, other, pii), normalize.ApplyViews(other, pii); !reflect.DeepEqual(got, want) {
			t.Fatalf("another text was answered with this one's readings")
		}
	}
}

// With every scanner of a request on one text, the findings are the ones
// each scanner makes on its own.
func TestPipeline_SharedReadingsChangeNoFinding(t *testing.T) {
	ResetScanCache()
	text := []byte("Müşteri ＣＵＳＴ-１２３４５６, TC 1000\u200b0000146, anahtar AKIAIOSFODNN7EXAMPLE, rakip Glοbex, e-posta ayse@example.com")
	scanners := []Scanner{
		NewPIIScanner(), NewSecretScanner(),
		NewCustomScanner(func() []CustomEntitySpec {
			return []CustomEntitySpec{{Name: "customer_no", Pattern: `CUST-\d{6}`, Severity: "high"}}
		}),
		NewCompetitorScanner(func() []CompetitorSpec {
			return []CompetitorSpec{{Name: "globex", Patterns: []string{`globex`}, Enabled: true}}
		}),
	}
	all := NewRegistry()
	var want []Finding
	for _, s := range scanners {
		all.Register(s)
		found, err := s.Scan(context.Background(), text)
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, found...)
	}
	stampVersions(want)
	for _, mode := range []PipelineMode{ModeSync, ModeAsync} {
		got, err := all.ScanAllWithConfig(context.Background(), text, PipelineConfig{Mode: mode})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) || len(got) < 4 {
			t.Fatalf("%s: %d findings together, %d apart", mode, len(got), len(want))
		}
		if mode == ModeSync && !reflect.DeepEqual(got, want) {
			t.Fatalf("%s:\n got %+v\nwant %+v", mode, got, want)
		}
	}
}
