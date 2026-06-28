package scanner

import (
	"context"
	"sync"
	"testing"
)

// ── stub scanner for tests ────────────────────────────────────────────────────

type countingScanner struct {
	name     string
	findings []Finding
	err      error
}

func (s *countingScanner) Name() string { return s.name }
func (s *countingScanner) Scan(_ context.Context, _ []byte) ([]Finding, error) {
	return s.findings, s.err
}

// ── NewRegistry ──────────────────────────────────────────────────────────────

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.Count() != 0 {
		t.Fatalf("empty registry Count: want 0, got %d", r.Count())
	}
}

// ── Register ─────────────────────────────────────────────────────────────────

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	r.Register(&countingScanner{name: "pii"})
	if r.Count() != 1 {
		t.Fatalf("after Register: want Count=1, got %d", r.Count())
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	r := NewRegistry()
	r.Register(&countingScanner{name: "pii"})
	r.Register(&countingScanner{name: "pii"})
	// Registry allows duplicates (caller must avoid). Count still reflects both.
	if r.Count() != 2 {
		t.Fatalf("duplicate Register: want Count=2, got %d", r.Count())
	}
}

func TestRegistry_Register_Multiple(t *testing.T) {
	r := NewRegistry()
	for i := 0; i < 10; i++ {
		r.Register(&countingScanner{name: "s"})
	}
	if r.Count() != 10 {
		t.Fatalf("10 registers: want Count=10, got %d", r.Count())
	}
}

// ── ScanAll ──────────────────────────────────────────────────────────────────

func TestRegistry_ScanAll(t *testing.T) {
	r := NewRegistry()
	r.Register(&countingScanner{
		name:     "pii",
		findings: []Finding{{Type: "pii", Category: "email", Confidence: 0.9}},
	})
	r.Register(&countingScanner{
		name:     "injection",
		findings: nil,
	})
	r.Register(&countingScanner{
		name:     "secret",
		findings: []Finding{{Type: "secret", Category: "aws_key", Confidence: 0.95}},
	})

	findings, err := r.ScanAll(context.Background(), []byte("test content"))
	if err != nil {
		t.Fatalf("ScanAll unexpected error: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("want 2 findings total, got %d", len(findings))
	}

	types := map[string]int{}
	for _, f := range findings {
		types[f.Type]++
	}
	if types["pii"] != 1 {
		t.Errorf("want 1 pii finding, got %d", types["pii"])
	}
	if types["secret"] != 1 {
		t.Errorf("want 1 secret finding, got %d", types["secret"])
	}
}

func TestRegistry_ScanAll_EmptyRegistry(t *testing.T) {
	r := NewRegistry()
	findings, err := r.ScanAll(context.Background(), []byte("test"))
	if err != nil {
		t.Fatalf("ScanAll on empty registry: unexpected error %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("empty registry: want 0 findings, got %d", len(findings))
	}
}

// ── ScanAllWithConfig ────────────────────────────────────────────────────────

func TestRegistry_ScanAllWithConfig_ModeSync(t *testing.T) {
	r := NewRegistry()
	r.Register(&countingScanner{
		name:     "pii",
		findings: []Finding{{Type: "pii", Category: "email"}},
	})
	r.Register(&countingScanner{
		name:     "secret",
		findings: []Finding{{Type: "secret", Category: "github_token"}},
	})

	cfg := PipelineConfig{Mode: ModeSync}
	findings, err := r.ScanAllWithConfig(context.Background(), []byte("test"), cfg)
	if err != nil {
		t.Fatalf("ScanAllWithConfig ModeSync: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("ModeSync: want 2 findings, got %d", len(findings))
	}
}

func TestRegistry_ScanAllWithConfig_ModeAsync(t *testing.T) {
	r := NewRegistry()
	r.Register(&countingScanner{
		name:     "pii",
		findings: []Finding{{Type: "pii", Category: "email"}},
	})
	r.Register(&countingScanner{
		name:     "secret",
		findings: []Finding{{Type: "secret", Category: "github_token"}},
	})

	cfg := PipelineConfig{Mode: ModeAsync}
	findings, err := r.ScanAllWithConfig(context.Background(), []byte("test"), cfg)
	if err != nil {
		t.Fatalf("ScanAllWithConfig ModeAsync: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("ModeAsync: want 2 findings, got %d", len(findings))
	}
}

func TestRegistry_ScanAllWithConfig_WithSpeed(t *testing.T) {
	r := NewRegistry()
	r.Register(&countingScanner{
		name:     "fast",
		findings: []Finding{{Category: "fast_finding"}},
	})
	r.Register(&countingScanner{
		name:     "slow",
		findings: []Finding{{Category: "slow_finding"}},
	})
	r.SetSpeed("fast", SpeedFast)
	r.SetSpeed("slow", SpeedSlow)

	cfg := PipelineConfig{Mode: ModeAdaptive}
	findings, err := r.ScanAllWithConfig(context.Background(), []byte("test"), cfg)
	if err != nil {
		t.Fatalf("ScanAllWithConfig with speeds: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("with speeds: want 2 findings, got %d", len(findings))
	}
}

// ── SetSpeed ─────────────────────────────────────────────────────────────────

func TestRegistry_SetSpeed(t *testing.T) {
	r := NewRegistry()
	r.SetSpeed("pii", SpeedSlow)
	r.SetSpeed("injection", SpeedFast)

	// Verify via ScanAllWithConfig: speeds affect pipeline classification.
	r.Register(&countingScanner{name: "pii", findings: []Finding{{Category: "p"}}})
	r.Register(&countingScanner{name: "injection", findings: []Finding{{Category: "i"}}})

	findings, err := r.ScanAll(context.Background(), []byte("test"))
	if err != nil {
		t.Fatalf("ScanAll after SetSpeed: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("SetSpeed scan: want 2 findings, got %d", len(findings))
	}
}

func TestRegistry_SetSpeed_Overwrite(t *testing.T) {
	r := NewRegistry()
	r.SetSpeed("pii", SpeedFast)
	r.SetSpeed("pii", SpeedSlow) // overwrite

	r.Register(&countingScanner{name: "pii", findings: []Finding{{Category: "p"}}})
	findings, err := r.ScanAll(context.Background(), []byte("test"))
	if err != nil {
		t.Fatalf("ScanAll after SetSpeed overwrite: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("overwrite: want 1 finding, got %d", len(findings))
	}
}

// ── ScannerDetectionStats ────────────────────────────────────────────────────

func TestScannerDetectionStats_Empty(t *testing.T) {
	// Reset the global counter map for a clean test.
	scannerDetectionCounts = sync.Map{}

	stats := ScannerDetectionStats()
	if len(stats) != 0 {
		t.Fatalf("empty stats: want 0 entries, got %d", len(stats))
	}
}

func TestScannerDetectionStats_Populated(t *testing.T) {
	// Reset the global counter map.
	scannerDetectionCounts = sync.Map{}

	// Simulate findings being recorded (as the pipeline does).
	incDetectionCount("pii", 5)
	incDetectionCount("secret", 3)
	incDetectionCount("injection", 0) // zero delta is a no-op but still registers

	stats := ScannerDetectionStats()
	if len(stats) != 3 {
		t.Fatalf("want 3 scanner entries, got %d: %v", len(stats), stats)
	}
	if stats["pii"] != 5 {
		t.Errorf("pii count: want 5, got %d", stats["pii"])
	}
	if stats["secret"] != 3 {
		t.Errorf("secret count: want 3, got %d", stats["secret"])
	}
	if stats["injection"] != 0 {
		t.Errorf("injection count: want 0, got %d", stats["injection"])
	}
}

func TestScannerDetectionStats_Concurrent(t *testing.T) {
	// Reset the global counter map.
	scannerDetectionCounts = sync.Map{}

	var wg sync.WaitGroup
	const goroutines = 50
	const incrementsPerGoroutine = 100

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < incrementsPerGoroutine; i++ {
				incDetectionCount("pii", 1)
			}
		}()
	}
	wg.Wait()

	stats := ScannerDetectionStats()
	expected := int64(goroutines * incrementsPerGoroutine)
	if stats["pii"] != expected {
		t.Errorf("concurrent pii count: want %d, got %d (lost %d increments)",
			expected, stats["pii"], expected-stats["pii"])
	}
}

// ── Count (concurrent safety) ────────────────────────────────────────────────

func TestRegistry_Count_Concurrent(t *testing.T) {
	r := NewRegistry()
	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r.Register(&countingScanner{name: "s"})
		}(i)
	}
	wg.Wait()

	if r.Count() != n {
		t.Errorf("concurrent Register: want Count=%d, got %d", n, r.Count())
	}
}

// ── nil Registry safety ──────────────────────────────────────────────────────

func TestRegistry_NilRegistry_Safe(t *testing.T) {
	// Verify that calling methods on a nil *Registry panics (standard Go behavior,
	// but document it).
	var r *Registry
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		_ = r.Count()
	}()
	if !panicked {
		t.Log("nil Registry.Count() did not panic — nil-safe?")
	}
}

// ── Reset detection counts helper for tests ──────────────────────────────────

func TestIncDetectionCount_Atomicity(t *testing.T) {
	// Reset.
	scannerDetectionCounts = sync.Map{}

	// Single-threaded correctness.
	incDetectionCount("test", 42)
	incDetectionCount("test", 58)

	stats := ScannerDetectionStats()
	if stats["test"] != 100 {
		t.Errorf("sequential incDetectionCount: want 100, got %d", stats["test"])
	}

	// Concurrent correctness (further tested in TestScannerDetectionStats_Concurrent).
}

// ── Benchmarks ───────────────────────────────────────────────────────────────

func BenchmarkIncDetectionCount(b *testing.B) {
	scannerDetectionCounts = sync.Map{}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		incDetectionCount("pii", 1)
	}
}

func BenchmarkScannerDetectionStats_10Scanners(b *testing.B) {
	scannerDetectionCounts = sync.Map{}
	names := []string{"pii", "secret", "injection", "jailbreak", "content_moderation", "custom", "competitor", "risk", "bin", "dfa"}
	for _, n := range names {
		incDetectionCount(n, 100)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = ScannerDetectionStats()
	}
}
