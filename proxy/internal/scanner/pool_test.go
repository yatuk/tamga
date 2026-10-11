package scanner

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/errkind"
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

// ── Panic containment ────────────────────────────────────────────────────────

// panickingScanner panics with a value that quotes what it was scanning, as
// an index out of range on the text would.
type panickingScanner struct{ name string }

func (s *panickingScanner) Name() string { return s.name }
func (s *panickingScanner) Scan(_ context.Context, content []byte) ([]Finding, error) {
	panic("cannot handle: " + string(content))
}

// A worker is not a request's goroutine, so nothing recovers above it: a
// scanner that panicked there took the whole process down. The job is now
// answered with a failure of kind "panic", and the worker lives on.
func TestWorkerPool_PanickingScannerIsAnswered(t *testing.T) {
	p := NewWorkerPool(1, 4)
	defer func() { _ = p.Shutdown(time.Second) }()
	before := ScanDegradedStats()[DegradedPanic]

	results := make(chan ScanResult, 2)
	submit := func(s Scanner) {
		t.Helper()
		if err := p.Submit(Job{Scanner: s, Content: []byte("secret-text-10000000146"), Ctx: context.Background(), ResultCh: results}); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	receive := func() ScanResult {
		t.Helper()
		select {
		case r := <-results:
			return r
		case <-time.After(5 * time.Second):
			t.Fatal("the job was never answered: its request would wait for good")
			return ScanResult{}
		}
	}

	submit(&panickingScanner{name: "bad"})
	r := receive()
	if r.Scanner != "bad" || r.Error == nil || len(r.Findings) != 0 {
		t.Fatalf("result of a panicking scanner: %+v", r)
	}
	if kind := errkind.Of(r.Error); kind != errkind.Panic {
		t.Errorf("error kind = %q, want %q", kind, errkind.Panic)
	}
	// The recovered value can be the text; it is in no message.
	if strings.Contains(r.Error.Error(), "secret-text") || strings.Contains(r.Error.Error(), "10000000146") {
		t.Errorf("the error repeats the panic's value: %v", r.Error)
	}

	// The only worker is still there for the next job.
	submit(&delayedScanner{name: "good", findings: []Finding{{Type: "pii"}}})
	if r := receive(); r.Scanner != "good" || r.Error != nil || len(r.Findings) != 1 {
		t.Fatalf("the job after the panic: %+v", r)
	}

	stats := p.Stats()
	if stats.Failed != 1 || stats.Completed != 1 || stats.ActiveJobs != 0 {
		t.Errorf("stats after one panic and one scan: %+v", stats)
	}
	if got := ScanDegradedStats()[DegradedPanic] - before; got != 1 {
		t.Errorf("panics counted: %d, want 1", got)
	}
}

// The same through the pipeline in workerpool mode: the scan comes back,
// degraded by a panic, with what the other scanners found.
func TestPipeline_WorkerPoolContainsAPanic(t *testing.T) {
	pool := NewWorkerPool(2, 8)
	defer func() { _ = pool.Shutdown(time.Second) }()
	pl := NewPipelineWithConfig([]ScannerEntry{
		{Scanner: &delayedScanner{name: "good", findings: []Finding{{Type: "pii", Category: "email"}}}, Speed: SpeedFast},
		{Scanner: &panickingScanner{name: "bad"}, Speed: SpeedSlow},
	}, PipelineConfig{Mode: ModeWorkerPool, Pool: pool})

	type outcome struct {
		findings []Finding
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		findings, err := pl.Scan(context.Background(), []byte("secret-text-10000000146"))
		done <- outcome{findings, err}
	}()
	var got outcome
	select {
	case got = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the scan did not come back")
	}
	if !errors.Is(got.err, ErrScanDegraded) || errkind.Of(got.err) != errkind.Panic {
		t.Fatalf("err = %v (kind %q), want a degraded scan of kind %q", got.err, errkind.Of(got.err), errkind.Panic)
	}
	if strings.Contains(got.err.Error(), "secret-text") {
		t.Errorf("the error repeats the panic's value: %v", got.err)
	}
	if len(got.findings) != 1 || got.findings[0].Category != "email" {
		t.Errorf("findings = %+v, want the other scanner's", got.findings)
	}
}

// A job without a scanner is answered too, not left to panic in the worker.
func TestWorkerPool_NilScannerIsAnswered(t *testing.T) {
	p := NewWorkerPool(1, 1)
	defer func() { _ = p.Shutdown(time.Second) }()
	results := make(chan ScanResult, 1)
	if err := p.Submit(Job{Ctx: context.Background(), ResultCh: results}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case r := <-results:
		if r.Error == nil || errkind.Of(r.Error) != errkind.Panic {
			t.Fatalf("result: %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the job was never answered")
	}
}

// A scan whose context is already over comes back with that error. Submit
// used to refuse such a job at random, and the pipeline then waited for a
// result nobody was going to send: the request hung.
func TestPipeline_WorkerPoolAnswersACancelledScan(t *testing.T) {
	for i := 0; i < 50; i++ {
		pool := NewWorkerPool(2, 8)
		ran := &ranScanner{}
		pl := NewPipelineWithConfig([]ScannerEntry{
			{Scanner: ran}, {Scanner: &countingScanner{name: "b"}}, {Scanner: &countingScanner{name: "c"}},
		}, PipelineConfig{Mode: ModeWorkerPool, Pool: pool})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		errc := make(chan error, 1)
		go func() {
			_, err := pl.Scan(ctx, []byte("text"))
			errc <- err
		}()
		select {
		case err := <-errc:
			if !errors.Is(err, ErrScanDegraded) || !errors.Is(err, context.Canceled) {
				t.Fatalf("round %d: err = %v, want a degraded scan that was cancelled", i, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("round %d: the scan did not come back", i)
		}
		if n := ran.count.Load(); n != 0 {
			t.Fatalf("round %d: a scanner ran %d times on a cancelled scan", i, n)
		}
		_ = pool.Shutdown(time.Second)
	}
}

// Submit answers for an accepted job only: with a full queue the job is
// refused, whatever its context, and nothing is sent.
func TestWorkerPool_SubmitRefusesOnlyWhenFull(t *testing.T) {
	p := NewWorkerPool(1, 1)
	defer func() { _ = p.Shutdown(time.Second) }()
	release := make(chan struct{})
	busy := make(chan ScanResult, 2)
	// One job holds the worker, one fills the queue.
	for i := 0; i < 2; i++ {
		if err := p.Submit(Job{Scanner: &blockingScanner{release: release}, Ctx: context.Background(), ResultCh: busy}); err != nil {
			t.Fatalf("Submit %d: %v", i, err)
		}
		if i == 0 {
			waitFor(t, func() bool { return p.Stats().ActiveJobs == 1 })
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results := make(chan ScanResult, 1)
	for i := 0; i < 20; i++ {
		if err := p.Submit(Job{Scanner: &countingScanner{name: "late"}, Ctx: ctx, ResultCh: results}); err != ErrQueueFull {
			t.Fatalf("Submit to a full queue = %v, want ErrQueueFull", err)
		}
	}
	close(release)
	<-busy
	<-busy
	select {
	case r := <-results:
		t.Fatalf("a refused job was answered: %+v", r)
	case <-time.After(50 * time.Millisecond):
	}
}

// ranScanner counts how often it was run.
type ranScanner struct{ count atomic.Int64 }

func (s *ranScanner) Name() string { return "ran" }
func (s *ranScanner) Scan(context.Context, []byte) ([]Finding, error) {
	s.count.Add(1)
	return nil, nil
}

// blockingScanner holds its worker until release is closed.
type blockingScanner struct{ release chan struct{} }

func (s *blockingScanner) Name() string { return "blocking" }
func (s *blockingScanner) Scan(context.Context, []byte) ([]Finding, error) {
	<-s.release
	return nil, nil
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(time.Millisecond)
	}
}
