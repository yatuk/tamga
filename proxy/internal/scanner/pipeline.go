package scanner

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/yatuk/tamga/internal/errkind"
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
	// RequestCtx carries per-request operator-state context to scanners
	// implementing ContextualScanner. Nil for context-free scans.
	RequestCtx *RequestContext
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
	// What one scanner works out about the text and another needs (its
	// normalised readings, its digest for the scan cache) is worked out once
	// for this run. One scanner has nobody to share with.
	if len(p.fast)+len(p.slow) > 1 {
		ctx = withScanShared(ctx, content)
	}
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
		findings, err := scanEntry(sctx, entry.Scanner, content, p.cfg.RequestCtx)
		sp.SetAttributes(attribute.Int("findings.count", len(findings)))
		if err != nil {
			recordScanError(sp, err)
			if firstErr == nil {
				firstErr = &scannerError{scanner: entry.Scanner.Name(), cause: err}
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
	var deg degradation

	for _, entry := range entries {
		go func(e ScannerEntry) {
			defer wg.Done()
			defer recoverScanPanic(e.Scanner.Name(), &deg)

			sctx, sp := telemetry.Tracer().Start(ctx, telemetry.SpanNameForScanner(e.Scanner.Name()),
				trace.WithAttributes(attribute.Int("content.size_bytes", len(content))),
			)
			findings, err := scanEntry(sctx, e.Scanner, content, p.cfg.RequestCtx)
			sp.SetAttributes(attribute.Int("findings.count", len(findings)))
			if err != nil {
				recordScanError(sp, err)
				deg.note(e.Scanner.Name(), DegradedError, err)
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
	return all, deg.result()
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
	var deg degradation
	for _, entry := range p.fast {
		findings := p.scanFast(ctx, entry, content, &deg)
		if len(findings) > 0 {
			incDetectionCount(entry.Scanner.Name(), int64(len(findings)))
		}
		all = append(all, findings...)
	}

	// Phase 2 — Slow scanners (parallel goroutines).
	if len(p.slow) == 0 {
		stampVersions(all)
		return all, deg.result()
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(p.slow))

	for _, entry := range p.slow {
		go func(e ScannerEntry) {
			defer wg.Done()
			defer recoverScanPanic(e.Scanner.Name(), &deg)

			sctx, sp := telemetry.Tracer().Start(ctx, telemetry.SpanNameForScanner(e.Scanner.Name()),
				trace.WithAttributes(attribute.Int("content.size_bytes", len(content))),
			)
			findings, err := scanEntry(sctx, e.Scanner, content, p.cfg.RequestCtx)
			sp.SetAttributes(attribute.Int("findings.count", len(findings)))
			if err != nil {
				recordScanError(sp, err)
				deg.note(e.Scanner.Name(), DegradedError, err)
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
	return all, deg.result()
}

// scanFast runs one fast scanner in the calling goroutine. A panic is
// contained here so it cannot take the request handler down with it.
func (p *Pipeline) scanFast(ctx context.Context, entry ScannerEntry, content []byte, deg *degradation) (findings []Finding) {
	defer recoverScanPanic(entry.Scanner.Name(), deg)

	sctx, sp := telemetry.Tracer().Start(ctx, telemetry.SpanNameForScanner(entry.Scanner.Name()),
		trace.WithAttributes(attribute.Int("content.size_bytes", len(content))),
	)
	defer sp.End()
	findings, err := scanEntry(sctx, entry.Scanner, content, p.cfg.RequestCtx)
	sp.SetAttributes(attribute.Int("findings.count", len(findings)))
	if err != nil {
		recordScanError(sp, err)
		deg.note(entry.Scanner.Name(), DegradedError, err)
	}
	return findings
}

// allEntries returns fast + slow scanners concatenated (sync/async modes
// don't need the distinction).
func (p *Pipeline) allEntries() []ScannerEntry {
	entries := make([]ScannerEntry, 0, len(p.fast)+len(p.slow))
	entries = append(entries, p.fast...)
	entries = append(entries, p.slow...)
	return entries
}

// scanWorkerPool submits all scanners to the bounded WorkerPool and collects
// one result for each job the pool accepted. A scanner the pool refused (the
// queue is full, the pool is shut down) did not run: that is noted as a
// degraded scan, and no result is waited for.
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
	var deg degradation

	for _, entry := range entries {
		// Load shedding: skip non-critical scanners under overload.
		if p.shedder != nil && !p.shedder.ShouldRun(entry.Scanner.Name()) {
			deg.note(entry.Scanner.Name(), DegradedLoadShed, nil)
			continue
		}

		err := p.pool.Submit(Job{
			Scanner:  entry.Scanner,
			Content:  content,
			Ctx:      ctx,
			ReqCtx:   p.cfg.RequestCtx,
			ResultCh: resultCh,
		})
		if err != nil {
			// Any refusal means the job is not in the queue and no result
			// will come for it. The other scanners still produce theirs.
			if errors.Is(err, ErrQueueFull) {
				deg.note(entry.Scanner.Name(), DegradedQueueFull, nil)
			} else {
				deg.note(entry.Scanner.Name(), DegradedError, err)
			}
			continue
		}
		submitted++
	}

	var all []Finding
	for i := 0; i < submitted; i++ {
		r := <-resultCh
		if r.Error != nil {
			// What the scanner found before it failed still counts, as in
			// the other modes: a scan cut short is not a clean one.
			deg.note(r.Scanner, degradedReason(r.Error), r.Error)
		}
		if len(r.Findings) > 0 {
			incDetectionCount(r.Scanner, int64(len(r.Findings)))
		}
		all = append(all, r.Findings...)
	}

	stampVersions(all)
	return all, deg.result()
}

// degradedReason is the reason a pool result's error is counted under. The
// pool recovers a scanner's panic and returns it as a panicError; that is a
// panic here as it is in the modes that recover one themselves, and counted
// once. Anything else is an error.
func degradedReason(err error) string {
	if errkind.Of(err) == errkind.Panic {
		return DegradedPanic
	}
	return DegradedError
}

// degradation records scanners that did not report during one pipeline run.
// The pipeline keeps going (other scanners' findings still count), but the
// loss of coverage is logged, counted and returned to the caller instead of
// being swallowed.
type degradation struct {
	mu  sync.Mutex
	err error
}

func (d *degradation) note(scannerName, reason string, cause error) {
	RecordDegraded(reason)
	// Shedding happens per scanner per request while the pool is saturated;
	// logging each one at warn would add load exactly when there is none to
	// spare. The counter carries that signal instead.
	ev := log.Warn()
	switch reason {
	case DegradedPanic:
		ev = log.Error()
	case DegradedLoadShed, DegradedQueueFull:
		ev = log.Debug()
	}
	ev = ev.Str("component", "scanner").Str("scanner", scannerName).Str("reason", reason)
	if cause != nil {
		// The kind of error and never its text: a scanner's error can quote
		// what it read, and what it read is the request.
		ev = ev.Str("error_kind", errkind.Of(cause))
	}
	ev.Msg("scanner did not report; request evaluated with reduced coverage")

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err == nil {
		d.err = &scannerError{scanner: scannerName, reason: reason, cause: cause}
	}
}

// scannerError is the error of a run in which a scanner did not report. Its
// message names the scanner, the reason and the kind of the cause. The cause
// stays in the chain, so a caller still tells a scan that was cut short from
// one that failed (IncompleteReason), but its text is in no message: whoever
// logs this error logs nothing a scanner quoted.
//
// The one cause whose own words are kept is a stopError, the built-in
// scanners' report of a cap or a budget: its message is written here, from
// a scanner's name and a number, and policy simulation shows it to the
// person whose pattern hit the cap.
type scannerError struct {
	scanner string
	// reason is one of the Degraded constants, or "" for sync mode, which
	// counts nothing.
	reason string
	cause  error
}

func (e *scannerError) Error() string {
	msg := ErrScanDegraded.Error() + ": " + e.scanner
	if e.reason != "" {
		msg += " (" + e.reason + ")"
	}
	var stop *stopError
	switch {
	case errors.As(e.cause, &stop):
		msg += ": " + stop.msg
	case e.cause != nil:
		msg += ": " + errkind.Of(e.cause)
	}
	return msg
}

func (e *scannerError) Unwrap() []error {
	if e.cause == nil {
		return []error{ErrScanDegraded}
	}
	return []error{ErrScanDegraded, e.cause}
}

// panicError is a scanner's panic as an error. The recovered value is kept
// out of the message for the same reason: it can be the text.
type panicError struct {
	value any
}

func (e *panicError) Error() string     { return "panic" }
func (e *panicError) ErrorKind() string { return errkind.Panic }

// recordScanError marks a scanner's span as failed by the kind of the error.
// Span.RecordError would export the error's text to the trace backend.
func recordScanError(sp trace.Span, err error) {
	kind := errkind.Of(err)
	sp.SetAttributes(attribute.String("error.type", kind))
	sp.SetStatus(codes.Error, kind)
}

func (d *degradation) result() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.err
}

// recoverScanPanic keeps a panicking scanner from killing the proxy and
// reports it as a degraded scan.
func recoverScanPanic(name string, d *degradation) {
	if r := recover(); r != nil {
		d.note(name, DegradedPanic, &panicError{value: r})
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
