package scanner

import (
	"context"
	"testing"
	"time"
)

// loadShedPool creates a WorkerPool pre-loaded to the given depth for
// LoadShedder.ShouldRun testing. Zero workers so jobs never drain.
func loadShedPool(t *testing.T, queueSize, depth int) *WorkerPool {
	t.Helper()
	p := NewWorkerPool(0, queueSize)

	for i := 0; i < depth; i++ {
		job := Job{
			Scanner:  &countingScanner{name: "fill", findings: []Finding{{Category: "f"}}},
			Content:  []byte("c"),
			Ctx:      context.Background(),
			ResultCh: make(chan ScanResult, 1),
		}
		if err := p.Submit(job); err != nil {
			t.Fatalf("failed to fill queue to depth %d: %v at job %d", depth, err, i)
		}
	}
	return p
}

// ── ShouldRun: normal load (≤ 80%) ───────────────────────────────────────────

func TestLoadShedder_ShouldRun_NormalLoad(t *testing.T) {
	p := loadShedPool(t, 100, 50) // 50% load
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	s := NewLoadShedder(p)

	if !s.ShouldRun("pii") {
		t.Error("pii (critical) should run at 50% load")
	}
	if !s.ShouldRun("injection") {
		t.Error("injection (non-critical) should run at 50% load")
	}
	if !s.ShouldRun("unknown_scanner") {
		t.Error("unknown scanner should run at 50% load")
	}
}

func TestLoadShedder_ShouldRun_EmptyQueue(t *testing.T) {
	p := loadShedPool(t, 100, 0)
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	s := NewLoadShedder(p)
	if !s.ShouldRun("pii") {
		t.Error("pii should run when queue is empty")
	}
	if !s.ShouldRun("injection") {
		t.Error("injection should run when queue is empty")
	}
}

// ── ShouldRun: degraded (>80% to ≤95%) ──────────────────────────────────────

func TestLoadShedder_ShouldRun_HighLoad_Critical(t *testing.T) {
	p := loadShedPool(t, 100, 85) // 85%
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	s := NewLoadShedder(p)

	if !s.ShouldRun("pii") {
		t.Error("pii (critical) should run at 85% load")
	}
	if !s.ShouldRun("secret") {
		t.Error("secret (critical) should run at 85% load")
	}
}

func TestLoadShedder_ShouldRun_HighLoad_NonCritical(t *testing.T) {
	p := loadShedPool(t, 100, 85) // 85%
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	s := NewLoadShedder(p)

	if s.ShouldRun("injection") {
		t.Error("injection (non-critical) should NOT run at 85% load")
	}
	if s.ShouldRun("jailbreak") {
		t.Error("jailbreak (non-critical) should NOT run at 85% load")
	}
	if s.ShouldRun("content_moderation") {
		t.Error("content_moderation (non-critical) should NOT run at 85% load")
	}
}

func TestLoadShedder_ShouldRun_HighLoad_UnknownScanner(t *testing.T) {
	p := loadShedPool(t, 100, 85) // 85%
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	s := NewLoadShedder(p)

	if s.ShouldRun("nonexistent_scanner") {
		t.Error("unknown scanner should NOT run at 85% load")
	}
}

func TestLoadShedder_ShouldRun_Boundary_81Percent(t *testing.T) {
	p := loadShedPool(t, 100, 81) // just above 80%
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	s := NewLoadShedder(p)

	if !s.ShouldRun("pii") {
		t.Error("pii (critical) should run at 81%")
	}
	if s.ShouldRun("injection") {
		t.Error("injection (non-critical) should NOT run at 81%")
	}
}

func TestLoadShedder_ShouldRun_Boundary_80Percent(t *testing.T) {
	p := loadShedPool(t, 100, 80) // exactly 80%
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	s := NewLoadShedder(p)

	if !s.ShouldRun("pii") {
		t.Error("pii should run at 80%")
	}
	if !s.ShouldRun("injection") {
		t.Error("injection should run at 80% (exactly at threshold, not degraded)")
	}
}

// ── ShouldRun: fail-fast (>95%) ──────────────────────────────────────────────

func TestLoadShedder_ShouldRun_ExtremeLoad(t *testing.T) {
	p := loadShedPool(t, 100, 96) // 96%
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	s := NewLoadShedder(p)

	if s.ShouldRun("pii") {
		t.Error("pii (critical) should NOT run at 96% load — fail-fast")
	}
	if s.ShouldRun("secret") {
		t.Error("secret (critical) should NOT run at 96% load — fail-fast")
	}
	if s.ShouldRun("injection") {
		t.Error("injection should NOT run at 96% load")
	}
}

func TestLoadShedder_ShouldRun_Boundary_96Percent(t *testing.T) {
	p := loadShedPool(t, 100, 96)
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	s := NewLoadShedder(p)

	if s.ShouldRun("pii") {
		t.Error("pii should NOT run at 96% (>0.95)")
	}
}

func TestLoadShedder_ShouldRun_Boundary_95Percent(t *testing.T) {
	// 95/100 = 0.95. The check is `load > 0.95`, so 0.95 is NOT > 0.95.
	// Falls through to `load > 0.80` → degraded mode.
	p := loadShedPool(t, 100, 95)
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	s := NewLoadShedder(p)

	if !s.ShouldRun("pii") {
		t.Error("pii should run at exactly 95% (not > 0.95, enters degraded mode)")
	}
	if s.ShouldRun("injection") {
		t.Error("injection should NOT run at 95% (degraded mode, > 0.80)")
	}
}

// ── ShouldRun: queue size zero (unbounded) ───────────────────────────────────

func TestLoadShedder_ShouldRun_QueueSizeZero(t *testing.T) {
	p := NewWorkerPool(1, 0)
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	s := NewLoadShedder(p)

	// Queue size 0 → unbounded → always run.
	if !s.ShouldRun("any_scanner") {
		t.Error("should run when queue size is 0 (unbounded)")
	}
}

// ── ShouldRun: single-slot queue ─────────────────────────────────────────────

func TestLoadShedder_ShouldRun_SingleSlotQueue(t *testing.T) {
	p := loadShedPool(t, 1, 1) // 100% full
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	s := NewLoadShedder(p)

	if s.ShouldRun("pii") {
		t.Error("pii should NOT run when single-slot queue is full (100% > 95%)")
	}
}

// ── Benchmarks ───────────────────────────────────────────────────────────────

func BenchmarkLoadShedder_ShouldRun(b *testing.B) {
	// Create pool and pre-fill to 50% (no helper — b is *testing.B, not *testing.T).
	p := NewWorkerPool(0, 100)
	for i := 0; i < 50; i++ {
		job := Job{
			Scanner:  &countingScanner{name: "fill", findings: []Finding{{Category: "f"}}},
			Content:  []byte("c"),
			Ctx:      context.Background(),
			ResultCh: make(chan ScanResult, 1),
		}
		_ = p.Submit(job)
	}
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	s := NewLoadShedder(p)
	names := []string{"pii", "secret", "injection", "jailbreak", "content_moderation", "unknown"}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		for _, name := range names {
			_ = s.ShouldRun(name)
		}
	}
}
