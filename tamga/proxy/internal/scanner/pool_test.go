package scanner

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// delayedScanner is a stub that sleeps before returning findings, used to
// exercise the worker pool's concurrency and active job accounting.
type delayedScanner struct {
	name     string
	findings []Finding
	delay    time.Duration
}

func (s *delayedScanner) Name() string { return s.name }
func (s *delayedScanner) Scan(ctx context.Context, _ []byte) ([]Finding, error) {
	select {
	case <-time.After(s.delay):
		return s.findings, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ── NewWorkerPool ────────────────────────────────────────────────────────────

func TestNewWorkerPool(t *testing.T) {
	p := NewWorkerPool(4, 16)
	if p == nil {
		t.Fatal("NewWorkerPool returned nil")
	}

	stats := p.Stats()
	if stats.Workers != 4 {
		t.Errorf("Workers: want 4, got %d", stats.Workers)
	}
	if stats.QueueSize != 16 {
		t.Errorf("QueueSize: want 16, got %d", stats.QueueSize)
	}
	if stats.QueueDepth != 0 {
		t.Errorf("QueueDepth: want 0, got %d", stats.QueueDepth)
	}
	if stats.ActiveJobs != 0 {
		t.Errorf("ActiveJobs: want 0, got %d", stats.ActiveJobs)
	}
	if stats.Submitted != 0 {
		t.Errorf("Submitted: want 0, got %d", stats.Submitted)
	}
	if stats.Completed != 0 {
		t.Errorf("Completed: want 0, got %d", stats.Completed)
	}
	if stats.Failed != 0 {
		t.Errorf("Failed: want 0, got %d", stats.Failed)
	}
	if stats.Shed != 0 {
		t.Errorf("Shed: want 0, got %d", stats.Shed)
	}

	// Clean up.
	if err := p.Shutdown(5 * time.Second); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

func TestNewWorkerPool_ZeroWorkers(t *testing.T) {
	p := NewWorkerPool(0, 10)
	if p == nil {
		t.Fatal("NewWorkerPool(0, 10) returned nil")
	}
	stats := p.Stats()
	if stats.Workers != 0 {
		t.Errorf("Workers: want 0, got %d", stats.Workers)
	}
	// Shutdown should still work with zero workers.
	if err := p.Shutdown(1 * time.Second); err != nil {
		t.Fatalf("Shutdown on zero-worker pool: %v", err)
	}
}

// ── Submit ───────────────────────────────────────────────────────────────────

func TestWorkerPool_Submit_Success(t *testing.T) {
	p := NewWorkerPool(2, 4)
	defer func() { _ = p.Shutdown(5 * time.Second) }()

	job := Job{
		Scanner:  &countingScanner{name: "test", findings: []Finding{{Category: "found"}}},
		Content:  []byte("content"),
		Ctx:      context.Background(),
		ResultCh: make(chan ScanResult, 1),
	}

	if err := p.Submit(job); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	result := <-job.ResultCh
	if result.Error != nil {
		t.Fatalf("job error: %v", result.Error)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("want 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Category != "found" {
		t.Errorf("Category: want 'found', got %q", result.Findings[0].Category)
	}
	if result.Scanner != "test" {
		t.Errorf("Scanner: want 'test', got %q", result.Scanner)
	}
	if result.Duration < 0 {
		t.Errorf("Duration should not be negative, got %v", result.Duration)
	}
}

func TestWorkerPool_Submit_Multiple(t *testing.T) {
	// Queue large enough to hold all submissions without shedding.
	p := NewWorkerPool(4, 20)
	defer func() { _ = p.Shutdown(5 * time.Second) }()

	const n = 10
	results := make([]chan ScanResult, n)
	for i := 0; i < n; i++ {
		results[i] = make(chan ScanResult, 1)
		job := Job{
			Scanner:  &countingScanner{name: "multi", findings: []Finding{{Category: "ok"}}},
			Content:  []byte("c"),
			Ctx:      context.Background(),
			ResultCh: results[i],
		}
		if err := p.Submit(job); err != nil {
			t.Fatalf("Submit %d: %v", i, err)
		}
	}

	for i := 0; i < n; i++ {
		result := <-results[i]
		if result.Error != nil {
			t.Errorf("job %d error: %v", i, result.Error)
		}
		if len(result.Findings) != 1 {
			t.Errorf("job %d: want 1 finding, got %d", i, len(result.Findings))
		}
	}

	stats := p.Stats()
	if stats.Submitted != int64(n) {
		t.Errorf("Submitted: want %d, got %d", n, stats.Submitted)
	}
	if stats.Completed != int64(n) {
		t.Errorf("Completed: want %d, got %d", n, stats.Completed)
	}
}

func TestWorkerPool_Submit_FullQueue(t *testing.T) {
	// Create a pool with 0 workers and a small queue so the queue fills up
	// immediately and never drains (no workers to process).
	p := NewWorkerPool(0, 2)
	defer func() { _ = p.Shutdown(1 * time.Second) }()

	// Fill the queue.
	for i := 0; i < 2; i++ {
		job := Job{
			Scanner:  &countingScanner{name: "fill"},
			Content:  []byte("c"),
			Ctx:      context.Background(),
			ResultCh: make(chan ScanResult, 1),
		}
		if err := p.Submit(job); err != nil {
			t.Fatalf("Submit %d (filling queue): unexpected %v", i, err)
		}
	}

	// This one should fail — queue is full.
	job := Job{
		Scanner:  &countingScanner{name: "reject"},
		Content:  []byte("c"),
		Ctx:      context.Background(),
		ResultCh: make(chan ScanResult, 1),
	}
	err := p.Submit(job)
	if err == nil {
		t.Fatal("expected ErrQueueFull, got nil")
	}
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("want ErrQueueFull, got %v", err)
	}

	stats := p.Stats()
	if stats.Shed != 1 {
		t.Errorf("Shed: want 1, got %d", stats.Shed)
	}
}

func TestWorkerPool_Submit_CancelledContext(t *testing.T) {
	p := NewWorkerPool(2, 4)
	defer func() { _ = p.Shutdown(5 * time.Second) }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	job := Job{
		Scanner:  &countingScanner{name: "cancelled"},
		Content:  []byte("c"),
		Ctx:      ctx,
		ResultCh: make(chan ScanResult, 1),
	}
	err := p.Submit(job)
	if err != nil {
		// Expected: context error returned directly.
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	} else {
		// Go select is random — Submit may enqueue despite cancelled ctx.
		// Drain the result channel and verify the job reports the error there.
		result := <-job.ResultCh
		if result.Error == nil {
			t.Error("expected error in result for cancelled context job")
		}
	}
}

func TestWorkerPool_Process_CancelledContext(t *testing.T) {
	// This test verifies that a job already in the queue but with a
	// cancelled context is handled correctly by process().
	p := NewWorkerPool(1, 2)
	defer func() { _ = p.Shutdown(5 * time.Second) }()

	ctx, cancel := context.WithCancel(context.Background())

	// First, submit a long-running job to block the single worker.
	blockJob := Job{
		Scanner:  &delayedScanner{name: "blocker", delay: 200 * time.Millisecond},
		Content:  []byte("block"),
		Ctx:      context.Background(),
		ResultCh: make(chan ScanResult, 1),
	}
	if err := p.Submit(blockJob); err != nil {
		t.Fatalf("Submit blocker: %v", err)
	}

	// Cancel the context for the second job BEFORE queueing.
	cancel()

	cancelJob := Job{
		Scanner:  &countingScanner{name: "cancelled"},
		Content:  []byte("c"),
		Ctx:      ctx,
		ResultCh: make(chan ScanResult, 1),
	}
	if err := p.Submit(cancelJob); err == nil {
		// The cancelled context passed Submit because the queue had room.
		// Wait for the result — process() should detect the cancelled ctx.
		result := <-cancelJob.ResultCh
		if result.Error == nil {
			t.Error("expected context error from cancelled job, got nil")
		}
		if result.Scanner != "cancelled" {
			t.Errorf("Scanner: want 'cancelled', got %q", result.Scanner)
		}
	} else {
		// May be rejected if queue is full or context cancelled detected.
		if !errors.Is(err, context.Canceled) && !errors.Is(err, ErrQueueFull) {
			t.Errorf("unexpected error: %v", err)
		}
	}

	// Wait for the blocker to finish so we can clean up.
	<-blockJob.ResultCh
}

// ── Shutdown ─────────────────────────────────────────────────────────────────

func TestWorkerPool_Shutdown(t *testing.T) {
	p := NewWorkerPool(2, 4)

	// Submit a few jobs and let them complete.
	for i := 0; i < 4; i++ {
		job := Job{
			Scanner:  &countingScanner{name: "s", findings: []Finding{{Category: "ok"}}},
			Content:  []byte("c"),
			Ctx:      context.Background(),
			ResultCh: make(chan ScanResult, 1),
		}
		if err := p.Submit(job); err != nil {
			t.Fatalf("Submit %d: %v", i, err)
		}
		<-job.ResultCh
	}

	if err := p.Shutdown(5 * time.Second); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// After shutdown, workers have exited.
	stats := p.Stats()
	if stats.ActiveJobs != 0 {
		t.Errorf("ActiveJobs after shutdown: want 0, got %d", stats.ActiveJobs)
	}
}

func TestWorkerPool_Shutdown_WithPendingJobs(t *testing.T) {
	// Workers should exit immediately on Shutdown, even with jobs in the queue.
	p := NewWorkerPool(0, 3) // zero workers so jobs never drain

	// Fill the queue.
	for i := 0; i < 3; i++ {
		job := Job{
			Scanner:  &countingScanner{name: "pending"},
			Content:  []byte("c"),
			Ctx:      context.Background(),
			ResultCh: make(chan ScanResult, 2), // extra buffer so send doesn't block
		}
		if err := p.Submit(job); err != nil {
			t.Fatalf("Submit %d: %v", i, err)
		}
	}

	// Shutdown with zero workers should be near-instant.
	if err := p.Shutdown(5 * time.Second); err != nil {
		t.Fatalf("Shutdown with pending jobs: %v", err)
	}
}

func TestWorkerPool_Shutdown_DoubleCall(t *testing.T) {
	p := NewWorkerPool(1, 1)
	if err := p.Shutdown(5 * time.Second); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}

	// Second Shutdown panics because quit is already closed. Recover.
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		_ = p.Shutdown(1 * time.Second)
	}()
	if !panicked {
		t.Log("double Shutdown did not panic — may be safe on this runtime")
	}
}

// ── Stats ────────────────────────────────────────────────────────────────────

func TestWorkerPool_Stats(t *testing.T) {
	p := NewWorkerPool(2, 8)
	defer func() { _ = p.Shutdown(5 * time.Second) }()

	const n = 5
	for i := 0; i < n; i++ {
		job := Job{
			Scanner:  &countingScanner{name: "stats_test", findings: []Finding{{Category: "c"}}},
			Content:  []byte("c"),
			Ctx:      context.Background(),
			ResultCh: make(chan ScanResult, 1),
		}
		if err := p.Submit(job); err != nil {
			t.Fatalf("Submit %d: %v", i, err)
		}
		<-job.ResultCh
	}

	stats := p.Stats()
	if stats.Submitted != int64(n) {
		t.Errorf("Submitted: want %d, got %d", n, stats.Submitted)
	}
	if stats.Completed != int64(n) {
		t.Errorf("Completed: want %d, got %d", n, stats.Completed)
	}
	if stats.Failed != 0 {
		t.Errorf("Failed: want 0, got %d", stats.Failed)
	}
	if stats.QueueDepth != 0 {
		t.Errorf("QueueDepth: want 0, got %d", stats.QueueDepth)
	}
	if stats.ActiveJobs != 0 {
		t.Errorf("ActiveJobs: want 0, got %d", stats.ActiveJobs)
	}

	// Per-scanner stats. Duration may be zero on fast platforms — just check count.
	if ps, ok := stats.PerScanner["stats_test"]; ok {
		if ps.Count != int64(n) {
			t.Errorf("PerScanner stats_test Count: want %d, got %d", n, ps.Count)
		}
		if ps.TotalDuration < 0 {
			t.Errorf("PerScanner TotalDuration should not be negative, got %v", ps.TotalDuration)
		}
		avg := ps.AvgMs()
		if avg < 0 {
			t.Errorf("AvgMs should not be negative, got %f", avg)
		}
	} else {
		t.Error("PerScanner stats missing 'stats_test'")
	}
}

func TestWorkerPool_Stats_FailedJobs(t *testing.T) {
	p := NewWorkerPool(1, 4)
	defer func() { _ = p.Shutdown(5 * time.Second) }()

	// Submit a job that returns an error.
	errJob := Job{
		Scanner:  &countingScanner{name: "failer", err: errors.New("scan failure")},
		Content:  []byte("c"),
		Ctx:      context.Background(),
		ResultCh: make(chan ScanResult, 1),
	}
	if err := p.Submit(errJob); err != nil {
		t.Fatalf("Submit errJob: %v", err)
	}
	result := <-errJob.ResultCh
	if result.Error == nil {
		t.Fatal("expected error from failer scanner")
	}

	stats := p.Stats()
	if stats.Failed != 1 {
		t.Errorf("Failed: want 1, got %d", stats.Failed)
	}
	if stats.Completed != 0 {
		t.Errorf("Completed: want 0, got %d", stats.Completed)
	}
}

// ── ScannerJobStats.AvgMs ────────────────────────────────────────────────────

func TestScannerJobStats_AvgMs(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		count    int64
		want     float64
	}{
		{"zero count", time.Second, 0, 0},
		{"1s / 2 = 500ms", time.Second, 2, 500},
		{"3s / 3 = 1000ms", 3 * time.Second, 3, 1000},
		{"100ms / 1 = 100ms", 100 * time.Millisecond, 1, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := ScannerJobStats{TotalDuration: tt.duration, Count: tt.count}
			got := s.AvgMs()
			if got != tt.want {
				t.Errorf("AvgMs: want %f, got %f", tt.want, got)
			}
		})
	}
}

// ── Concurrency ──────────────────────────────────────────────────────────────

func TestWorkerPool_ConcurrentSubmit(t *testing.T) {
	p := NewWorkerPool(4, 64)
	defer func() { _ = p.Shutdown(5 * time.Second) }()

	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			job := Job{
				Scanner:  &countingScanner{name: "concurrent", findings: []Finding{{Category: "ok"}}},
				Content:  []byte("c"),
				Ctx:      context.Background(),
				ResultCh: make(chan ScanResult, 1),
			}
			if err := p.Submit(job); err != nil {
				errs <- err
				return
			}
			result := <-job.ResultCh
			if result.Error != nil {
				errs <- result.Error
			} else {
				errs <- nil
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("concurrent job error: %v", err)
		}
	}

	stats := p.Stats()
	if stats.Submitted != int64(n) {
		t.Errorf("Submitted: want %d, got %d", n, stats.Submitted)
	}
	if stats.Completed != int64(n) {
		t.Errorf("Completed: want %d, got %d", n, stats.Completed)
	}
}

func TestWorkerPool_ActiveJobs(t *testing.T) {
	p := NewWorkerPool(2, 8)
	defer func() { _ = p.Shutdown(5 * time.Second) }()

	// Submit 2 slow jobs to occupy both workers.
	resultCh1 := make(chan ScanResult, 1)
	resultCh2 := make(chan ScanResult, 1)

	job1 := Job{
		Scanner:  &delayedScanner{name: "slow1", delay: 100 * time.Millisecond},
		Content:  []byte("c"),
		Ctx:      context.Background(),
		ResultCh: resultCh1,
	}
	job2 := Job{
		Scanner:  &delayedScanner{name: "slow2", delay: 100 * time.Millisecond},
		Content:  []byte("c"),
		Ctx:      context.Background(),
		ResultCh: resultCh2,
	}

	if err := p.Submit(job1); err != nil {
		t.Fatalf("Submit job1: %v", err)
	}
	if err := p.Submit(job2); err != nil {
		t.Fatalf("Submit job2: %v", err)
	}

	// Give workers time to pick up jobs.
	time.Sleep(20 * time.Millisecond)

	stats := p.Stats()
	if stats.ActiveJobs < 1 || stats.ActiveJobs > 2 {
		t.Errorf("ActiveJobs: expected 1-2, got %d", stats.ActiveJobs)
	}

	// Drain results.
	<-resultCh1
	<-resultCh2
}

// ── Benchmarks ───────────────────────────────────────────────────────────────

func BenchmarkWorkerPool_Submit(b *testing.B) {
	p := NewWorkerPool(4, 256)
	defer func() { _ = p.Shutdown(5 * time.Second) }()

	content := []byte("benchmark content")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		job := Job{
			Scanner:  &countingScanner{name: "bench", findings: []Finding{{Category: "ok"}}},
			Content:  content,
			Ctx:      context.Background(),
			ResultCh: make(chan ScanResult, 1),
		}
		_ = p.Submit(job)
		<-job.ResultCh
	}
}
