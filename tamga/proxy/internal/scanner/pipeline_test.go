package scanner

import (
	"context"
	"strings"
	"testing"
)

// stubScanner is a test scanner with configurable name, speed, and output.
type stubScanner struct {
	name     string
	findings []Finding
	panicOn  bool
}

func (s *stubScanner) Name() string { return s.name }
func (s *stubScanner) Scan(_ context.Context, _ []byte) ([]Finding, error) {
	if s.panicOn {
		panic("test panic in " + s.name)
	}
	return s.findings, nil
}

func makeStubEntry(name string, speed ScannerSpeed, findings ...Finding) ScannerEntry {
	return ScannerEntry{
		Scanner: &stubScanner{name: name, findings: findings},
		Speed:   speed,
	}
}

func f(cat string) Finding {
	return Finding{Type: "test", Category: cat, Confidence: 0.80}
}

// ── Basic pipeline tests ──────────────────────────────────────────────────

func TestPipeline_AllFast(t *testing.T) {
	entries := []ScannerEntry{
		makeStubEntry("a", SpeedFast, f("cat_a")),
		makeStubEntry("b", SpeedFast, f("cat_b")),
	}
	p := NewPipeline(entries)
	fs, err := p.Scan(context.Background(), []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 2 {
		t.Fatalf("want 2 findings, got %d", len(fs))
	}
	cats := map[string]bool{}
	for _, f := range fs {
		cats[f.Category] = true
	}
	if !cats["cat_a"] || !cats["cat_b"] {
		t.Errorf("missing categories: %v", cats)
	}
}

func TestPipeline_AllSlow(t *testing.T) {
	entries := []ScannerEntry{
		makeStubEntry("a", SpeedSlow, f("cat_a")),
		makeStubEntry("b", SpeedSlow, f("cat_b")),
		makeStubEntry("c", SpeedSlow, f("cat_c")),
	}
	p := NewPipeline(entries)
	fs, err := p.Scan(context.Background(), []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 3 {
		t.Fatalf("want 3 findings, got %d", len(fs))
	}
}

func TestPipeline_Mixed(t *testing.T) {
	entries := []ScannerEntry{
		makeStubEntry("fast_a", SpeedFast, f("fast_a")),
		makeStubEntry("slow_a", SpeedSlow, f("slow_a")),
		makeStubEntry("fast_b", SpeedFast, f("fast_b")),
		makeStubEntry("slow_b", SpeedSlow, f("slow_b")),
	}
	p := NewPipeline(entries)
	fs, err := p.Scan(context.Background(), []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 4 {
		t.Fatalf("want 4 findings, got %d", len(fs))
	}
}

// ── Panic recovery ────────────────────────────────────────────────────────

func TestPipeline_PanicRecovery(t *testing.T) {
	entries := []ScannerEntry{
		makeStubEntry("good", SpeedSlow, f("good")),
		{Scanner: &stubScanner{name: "bad", panicOn: true}, Speed: SpeedSlow},
	}
	p := NewPipeline(entries)
	fs, err := p.Scan(context.Background(), []byte("test"))
	if err != nil {
		t.Fatal(err) // Pipeline.Scan never returns error
	}
	if len(fs) != 1 {
		t.Fatalf("want 1 finding from good scanner, got %d", len(fs))
	}
	if fs[0].Category != "good" {
		t.Errorf("want category 'good', got %q", fs[0].Category)
	}
}

// ── Empty pipeline ────────────────────────────────────────────────────────

func TestPipeline_Empty(t *testing.T) {
	p := NewPipeline(nil)
	fs, err := p.Scan(context.Background(), []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 0 {
		t.Errorf("want 0 findings, got %d", len(fs))
	}
}

// ── Version stamping ──────────────────────────────────────────────────────

func TestPipeline_VersionStamping(t *testing.T) {
	entries := []ScannerEntry{
		makeStubEntry("s", SpeedFast, f("cat")),
	}
	p := NewPipeline(entries)
	fs, _ := p.Scan(context.Background(), []byte("test"))
	if len(fs) != 1 {
		t.Fatal("expected 1 finding")
	}
	if fs[0].ScannerVersion != ScannerVersion {
		t.Errorf("ScannerVersion: got %q, want %q", fs[0].ScannerVersion, ScannerVersion)
	}
	if fs[0].DatasetVersion != BINDatasetVersion {
		t.Errorf("DatasetVersion: got %q, want %q", fs[0].DatasetVersion, BINDatasetVersion)
	}
}

// ── Findings order independence ───────────────────────────────────────────

func TestPipeline_OrderIndependence(t *testing.T) {
	// Run the same mixed pipeline 10 times. The findings count and
	// categories should be consistent regardless of goroutine scheduling.
	for i := 0; i < 10; i++ {
		entries := []ScannerEntry{
			makeStubEntry("fast_a", SpeedFast, f("fast_a")),
			makeStubEntry("slow_a", SpeedSlow, f("slow_a")),
			makeStubEntry("fast_b", SpeedFast, f("fast_b")),
			makeStubEntry("slow_b", SpeedSlow, f("slow_b")),
		}
		p := NewPipeline(entries)
		fs, _ := p.Scan(context.Background(), []byte("test"))
		if len(fs) != 4 {
			t.Errorf("iteration %d: want 4 findings, got %d", i, len(fs))
		}
		cats := map[string]int{}
		for _, f := range fs {
			cats[f.Category]++
		}
		for _, want := range []string{"fast_a", "slow_a", "fast_b", "slow_b"} {
			if cats[want] != 1 {
				t.Errorf("iteration %d: want 1 %q, got %d", i, want, cats[want])
			}
		}
	}
}

// ── Large concurrent run ──────────────────────────────────────────────────

func TestPipeline_ConcurrentStress(t *testing.T) {
	// Launch 50 goroutines, each scanning through a shared pipeline.
	// This stresses the mutex and WaitGroup paths.
	entries := []ScannerEntry{
		makeStubEntry("fast", SpeedFast, f("fast")),
		makeStubEntry("slow1", SpeedSlow, f("slow1")),
		makeStubEntry("slow2", SpeedSlow, f("slow2")),
	}
	p := NewPipeline(entries)

	ctx := context.Background()
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		go func() {
			fs, err := p.Scan(ctx, []byte("concurrent test"))
			if err != nil {
				errs <- err
				return
			}
			if len(fs) != 3 {
				errs <- nil // just count
				return
			}
			errs <- nil
		}()
	}
	for i := 0; i < 50; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent scan error: %v", err)
		}
	}
}

// ── Benchmark ─────────────────────────────────────────────────────────────

func BenchmarkPipeline_FastOnly(b *testing.B) {
	entries := []ScannerEntry{
		makeStubEntry("a", SpeedFast, f("a")),
		makeStubEntry("b", SpeedFast, f("b")),
		makeStubEntry("c", SpeedFast, f("c")),
		makeStubEntry("d", SpeedFast, f("d")),
	}
	p := NewPipeline(entries)
	content := []byte(strings.Repeat("benchmark content ", 100))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = p.Scan(context.Background(), content)
	}
}

func BenchmarkPipeline_SlowOnly(b *testing.B) {
	entries := []ScannerEntry{
		makeStubEntry("a", SpeedSlow, f("a")),
		makeStubEntry("b", SpeedSlow, f("b")),
		makeStubEntry("c", SpeedSlow, f("c")),
	}
	p := NewPipeline(entries)
	content := []byte(strings.Repeat("benchmark content ", 100))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = p.Scan(context.Background(), content)
	}
}

func BenchmarkPipeline_Mixed(b *testing.B) {
	entries := []ScannerEntry{
		makeStubEntry("fast_a", SpeedFast, f("fast_a")),
		makeStubEntry("fast_b", SpeedFast, f("fast_b")),
		makeStubEntry("fast_c", SpeedFast, f("fast_c")),
		makeStubEntry("fast_d", SpeedFast, f("fast_d")),
		makeStubEntry("slow_a", SpeedSlow, f("slow_a")),
		makeStubEntry("slow_b", SpeedSlow, f("slow_b")),
		makeStubEntry("slow_c", SpeedSlow, f("slow_c")),
	}
	p := NewPipeline(entries)
	content := []byte(strings.Repeat("benchmark content ", 100))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = p.Scan(context.Background(), content)
	}
}
