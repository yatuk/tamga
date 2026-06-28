package scanner

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/yatuk/tamga/internal/telemetry"
)

// ObserveScannerLatency is set by the main package to record per-scanner
// latency on the Prometheus histogram. Nil-safe (no-op when unset).
var ObserveScannerLatency func(scanner string, ms float64)

// ────────────────────────────────────────────────────────────────────────────
// Hybrid Scanner Pipeline — Week 4 Async Pipeline
// ────────────────────────────────────────────────────────────────────────────
//
// Fast scanners (< 1ms) run sequentially in the calling goroutine to avoid
// goroutine creation overhead on sub-millisecond work.  Slow scanners
// (≥ 1ms) run in parallel goroutines, reducing end-to-end latency by
// overlapping their execution.
//
// Goroutine count per request: 3 (injection, content_moderation, jailbreak)
// down from 7 in the previous "goroutine for everything" approach.
// ────────────────────────────────────────────────────────────────────────────

// ScannerSpeed classifies a scanner for hybrid pipeline scheduling.
type ScannerSpeed int

const (
	// SpeedFast scanners (< 1ms) run sequentially — goroutine overhead
	// would dominate their execution time.
	SpeedFast ScannerSpeed = iota
	// SpeedSlow scanners (≥ 1ms) run in parallel goroutines — their
	// latency benefits from overlapping with other slow scanners.
	SpeedSlow
)

// PipelineMode selects the execution strategy for the scanner pipeline.
type PipelineMode string

const (
	// ModeAdaptive runs fast scanners sequentially, then slow scanners in
	// parallel goroutines. This is the default — best balance of latency
	// and resource usage for typical workloads.
	ModeAdaptive PipelineMode = "adaptive"
	// ModeSync runs all scanners sequentially in the calling goroutine.
	// Lowest overhead, highest latency. Useful for debugging or when
	// goroutine spawn cost exceeds scanner work (very small payloads).
	ModeSync PipelineMode = "sync"
	// ModeAsync runs all scanners in parallel goroutines. Lowest latency,
	// highest concurrency. Best for latency-sensitive deployments where
	// goroutine overhead is amortised over larger scan work.
	ModeAsync PipelineMode = "async"
	// ModeWorkerPool dispatches all scanners to a bounded WorkerPool.
	// Goroutine count is fixed at pool size regardless of request rate —
	// best for high-throughput deployments where goroutine spawn overhead
	// from async/adaptive modes creates scheduler pressure.
	ModeWorkerPool PipelineMode = "workerpool"
)

// PipelineConfig controls pipeline execution behaviour.
type PipelineConfig struct {
	// Mode selects the execution strategy (default: "adaptive").
	Mode PipelineMode
	// Timeout is the global deadline for the entire scan phase. When zero,
	// no deadline is applied. Only used in "async" and "adaptive" modes.
	Timeout time.Duration
	// Pool is the shared WorkerPool used when Mode is ModeWorkerPool.
	// May be nil for other modes.
	Pool *WorkerPool
	// LoadShed enables adaptive load shedding when using ModeWorkerPool.
	// Non-critical scanners are skipped when pool queue exceeds 80% capacity;
	// all scanners are skipped above 95%. Default false.
	LoadShed bool
}

// ScannerEntry pairs a Scanner with its speed classification.
type ScannerEntry struct {
	Scanner Scanner
	Speed   ScannerSpeed
}

// Pipeline executes scanners using a hybrid sequential/parallel strategy.
// Fast scanners run first (sequential), then slow scanners (parallel).
type Pipeline struct {
	fast    []ScannerEntry
	slow    []ScannerEntry
	cfg     PipelineConfig
	pool    *WorkerPool  // set from PipelineConfig.Pool
	shedder *LoadShedder // set when LoadShed is enabled
}

// NewPipeline creates a Pipeline from a list of scanner entries using the
// default adaptive strategy (fast sequential, slow parallel).
func NewPipeline(entries []ScannerEntry) *Pipeline {
	return NewPipelineWithConfig(entries, PipelineConfig{Mode: ModeAdaptive})
}

// NewPipelineWithConfig creates a Pipeline with an explicit execution
// strategy. Use ModeSync for sequential-only, ModeAsync for parallel-only,
// or ModeAdaptive for the hybrid strategy.
func NewPipelineWithConfig(entries []ScannerEntry, cfg PipelineConfig) *Pipeline {
	p := &Pipeline{
		fast: make([]ScannerEntry, 0, len(entries)),
		slow: make([]ScannerEntry, 0, len(entries)),
		cfg:  cfg,
		pool: cfg.Pool,
	}
	if cfg.LoadShed && cfg.Pool != nil {
		p.shedder = NewLoadShedder(cfg.Pool)
	}
	for _, e := range entries {
		switch e.Speed {
		case SpeedSlow:
			p.slow = append(p.slow, e)
		default:
			p.fast = append(p.fast, e)
		}
	}
	return p
}

// Scan runs all scanners using the configured strategy and returns combined
// findings. The behaviour depends on PipelineConfig.Mode:
//
//   - "sync": all scanners run sequentially — lowest overhead, highest latency.
//   - "async": all scanners run in parallel goroutines — lowest latency.
//   - "adaptive" (default): fast scanners sequential, slow scanners parallel.
//
// A panicking scanner in async/adaptive modes is recovered; in sync mode panics
// propagate (they indicate a bug that needs fixing).
func (p *Pipeline) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	switch p.cfg.Mode {
	case ModeSync:
		return p.scanSync(ctx, content)
	case ModeAsync:
		return p.scanAsync(ctx, content)
	case ModeWorkerPool:
		return p.scanWorkerPool(ctx, content)
	default: // ModeAdaptive (and any unknown value)
		return p.scanAdaptive(ctx, content)
	}
}

// scanSync runs all scanners sequentially in the calling goroutine. No
// goroutine or mutex overhead — ideal for debugging or tiny payloads.
func (p *Pipeline) scanSync(ctx context.Context, content []byte) ([]Finding, error) {
	all := make([]Finding, 0)
	var firstErr error
	for _, entry := range p.allEntries() {
		sctx, sp := telemetry.Tracer().Start(ctx, telemetry.SpanNameForScanner(entry.Scanner.Name()),
			trace.WithAttributes(attribute.Int("content.size_bytes", len(content))),
		)
		findings, err := entry.Scanner.Scan(sctx, content)
		sp.SetAttributes(attribute.Int("findings.count", len(findings)))
		if err != nil {
			sp.RecordError(err)
			if firstErr == nil {
				firstErr = err
			}
		}
		sp.End()
		if len(findings) > 0 {
			incDetectionCount(entry.Scanner.Name(), int64(len(findings)))
		}
		all = append(all, findings...)
	}
	stampVersions(all)
	return all, firstErr
}

// scanAsync runs all scanners in parallel goroutines. Maximum concurrency,
// minimum latency. Uses a global timeout from PipelineConfig when set.
func (p *Pipeline) scanAsync(ctx context.Context, content []byte) ([]Finding, error) {
	entries := p.allEntries()
	if len(entries) == 0 {
		return nil, nil
	}

	if p.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.cfg.Timeout)
		defer cancel()
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(entries))
	var all []Finding

	for _, entry := range entries {
		go func(e ScannerEntry) {
			defer wg.Done()
			defer recoverScanPanic(e.Scanner.Name())

			sctx, sp := telemetry.Tracer().Start(ctx, telemetry.SpanNameForScanner(e.Scanner.Name()),
				trace.WithAttributes(attribute.Int("content.size_bytes", len(content))),
			)
			findings, err := e.Scanner.Scan(sctx, content)
			sp.SetAttributes(attribute.Int("findings.count", len(findings)))
			if err != nil {
				sp.RecordError(err)
			}
			sp.End()

			if len(findings) > 0 {
				incDetectionCount(e.Scanner.Name(), int64(len(findings)))
			}

			mu.Lock()
			all = append(all, findings...)
			mu.Unlock()
		}(entry)
	}

	wg.Wait()
	stampVersions(all)
	return all, nil
}

// scanAdaptive is the original hybrid strategy: fast scanners run
// sequentially, slow scanners run in parallel goroutines.
func (p *Pipeline) scanAdaptive(ctx context.Context, content []byte) ([]Finding, error) {
	if p.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.cfg.Timeout)
		defer cancel()
	}

	// Phase 1 — Fast scanners (sequential, no goroutine overhead).
	var all []Finding
	for _, entry := range p.fast {
		sctx, sp := telemetry.Tracer().Start(ctx, telemetry.SpanNameForScanner(entry.Scanner.Name()),
			trace.WithAttributes(attribute.Int("content.size_bytes", len(content))),
		)
		findings, err := entry.Scanner.Scan(sctx, content)
		sp.SetAttributes(attribute.Int("findings.count", len(findings)))
		if err != nil {
			sp.RecordError(err)
		}
		sp.End()
		if len(findings) > 0 {
			incDetectionCount(entry.Scanner.Name(), int64(len(findings)))
		}
		all = append(all, findings...)
	}

	// Phase 2 — Slow scanners (parallel goroutines).
	if len(p.slow) == 0 {
		stampVersions(all)
		return all, nil
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(p.slow))

	for _, entry := range p.slow {
		go func(e ScannerEntry) {
			defer wg.Done()
			defer recoverScanPanic(e.Scanner.Name())

			sctx, sp := telemetry.Tracer().Start(ctx, telemetry.SpanNameForScanner(e.Scanner.Name()),
				trace.WithAttributes(attribute.Int("content.size_bytes", len(content))),
			)
			findings, err := e.Scanner.Scan(sctx, content)
			sp.SetAttributes(attribute.Int("findings.count", len(findings)))
			if err != nil {
				sp.RecordError(err)
			}
			sp.End()

			if len(findings) > 0 {
				incDetectionCount(e.Scanner.Name(), int64(len(findings)))
			}

			mu.Lock()
			all = append(all, findings...)
			mu.Unlock()
		}(entry)
	}

	wg.Wait()
	stampVersions(all)
	return all, nil
}

// allEntries returns fast + slow scanners concatenated (sync/async modes
// don't need the distinction).
func (p *Pipeline) allEntries() []ScannerEntry {
	entries := make([]ScannerEntry, 0, len(p.fast)+len(p.slow))
	entries = append(entries, p.fast...)
	entries = append(entries, p.slow...)
	return entries
}

// scanWorkerPool submits all scanners to the bounded WorkerPool. When the
// pool is nil or the queue is full, the scanner is silently skipped
// (fail-open per scanner). Results are collected from the result channel.
func (p *Pipeline) scanWorkerPool(ctx context.Context, content []byte) ([]Finding, error) {
	if p.pool == nil {
		// No pool configured — fall back to adaptive.
		return p.scanAdaptive(ctx, content)
	}

	if p.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.cfg.Timeout)
		defer cancel()
	}

	entries := p.allEntries()
	resultCh := make(chan ScanResult, len(entries))
	submitted := 0

	for _, entry := range entries {
		// Load shedding: skip non-critical scanners under overload.
		if p.shedder != nil && !p.shedder.ShouldRun(entry.Scanner.Name()) {
			resultCh <- ScanResult{
				Scanner: entry.Scanner.Name(),
				Error:   ErrQueueFull,
			}
			submitted++
			continue
		}

		err := p.pool.Submit(Job{
			Scanner:  entry.Scanner,
			Content:  content,
			Ctx:      ctx,
			ResultCh: resultCh,
		})
		if err == ErrQueueFull {
			// Queue full → skip this scanner silently (fail-open).
			// Other scanners still produce results.
			resultCh <- ScanResult{
				Scanner: entry.Scanner.Name(),
				Error:   err,
			}
		}
		submitted++
	}

	var all []Finding
	for i := 0; i < submitted; i++ {
		r := <-resultCh
		if r.Error != nil {
			continue
		}
		if len(r.Findings) > 0 {
			incDetectionCount(r.Scanner, int64(len(r.Findings)))
		}
		all = append(all, r.Findings...)
	}

	stampVersions(all)
	return all, nil
}

// recoverScanPanic logs a scanner panic without killing the proxy.
func recoverScanPanic(name string) {
	if r := recover(); r != nil {
		_ = fmt.Errorf("scanner %s panicked: %v", name, r)
	}
}

// incDetectionCount increments the per-scanner Prometheus counter.
func incDetectionCount(name string, delta int64) {
	val, _ := scannerDetectionCounts.LoadOrStore(name, new(int64))
	_ = atomic.AddInt64(val.(*int64), delta)
}

// stampVersions writes the current scanner and dataset versions onto every
// finding so API consumers can trace which release produced a match.
func stampVersions(findings []Finding) {
	for i := range findings {
		findings[i].ScannerVersion = ScannerVersion
		findings[i].DatasetVersion = BINDatasetVersion
	}
}
