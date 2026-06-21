package scanner

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Job is a unit of scan work submitted to the worker pool.
type Job struct {
	Scanner  Scanner
	Content  []byte
	Ctx      context.Context
	ResultCh chan ScanResult
}

// Sentinel errors returned by WorkerPool.
var (
	ErrQueueFull       = errors.New("scanner pool queue full")
	ErrShutdownTimeout = errors.New("scanner pool shutdown timeout")
)

// WorkerPool is a bounded pool of goroutines that execute scan jobs.
// Workers are started at creation time and live until Shutdown.
type WorkerPool struct {
	workers   int
	queueSize int
	jobs      chan Job
	wg        sync.WaitGroup
	quit      chan struct{}

	// Hot-path metrics updated atomically (no mutex contention).
	submitted  atomic.Int64
	completed  atomic.Int64
	failed     atomic.Int64
	shed       atomic.Int64
	activeJobs atomic.Int64

	// Per-scanner cumulative job duration (nanoseconds) and count for avg latency.
	durationMu sync.Mutex
	durationNs map[string]int64
	durationN  map[string]int64
}

// NewWorkerPool creates a pool with `workers` goroutines and a job queue of
// `queueSize`. Workers begin processing immediately.
func NewWorkerPool(workers, queueSize int) *WorkerPool {
	p := &WorkerPool{
		workers:    workers,
		queueSize:  queueSize,
		jobs:       make(chan Job, queueSize),
		quit:       make(chan struct{}),
		durationNs: make(map[string]int64),
		durationN:  make(map[string]int64),
	}
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
	return p
}

// worker is the per-goroutine event loop. It blocks on the job channel until
// quit is closed or the channel is drained.
func (p *WorkerPool) worker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.quit:
			return
		case job, ok := <-p.jobs:
			if !ok {
				return
			}
			p.process(job)
		}
	}
}

// process executes a single scan job, respecting context cancellation.
func (p *WorkerPool) process(job Job) {
	p.activeJobs.Add(1)
	defer p.activeJobs.Add(-1)

	// Check context before starting work.
	select {
	case <-job.Ctx.Done():
		p.failed.Add(1)
		job.ResultCh <- ScanResult{
			Scanner: job.Scanner.Name(),
			Error:   job.Ctx.Err(),
		}
		return
	default:
	}

	start := time.Now()
	findings, err := job.Scanner.Scan(job.Ctx, job.Content)
	elapsed := time.Since(start)

	if err != nil {
		p.failed.Add(1)
	} else {
		p.completed.Add(1)
	}

	// Track per-scanner duration for Prometheus avg latency.
	name := job.Scanner.Name()
	p.durationMu.Lock()
	p.durationNs[name] += int64(elapsed)
	p.durationN[name]++
	p.durationMu.Unlock()

	job.ResultCh <- ScanResult{
		Scanner:  name,
		Findings: findings,
		Error:    err,
		Duration: elapsed,
	}
}

// Submit enqueues a job. Returns ErrQueueFull if the queue is at capacity
// (backpressure). Returns context error if ctx is already cancelled.
func (p *WorkerPool) Submit(job Job) error {
	p.submitted.Add(1)
	select {
	case p.jobs <- job:
		return nil
	case <-job.Ctx.Done():
		p.failed.Add(1)
		return job.Ctx.Err()
	default:
		p.shed.Add(1)
		p.failed.Add(1)
		return ErrQueueFull
	}
}

// Shutdown closes the quit channel and waits up to timeout for all workers
// to finish their current job and exit. Returns ErrShutdownTimeout if workers
// do not drain within the deadline.
func (p *WorkerPool) Shutdown(timeout time.Duration) error {
	close(p.quit)
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return ErrShutdownTimeout
	}
}

// Stats returns a point-in-time snapshot of pool utilisation.
func (p *WorkerPool) Stats() PoolStats {
	p.durationMu.Lock()
	perScanner := make(map[string]ScannerJobStats, len(p.durationNs))
	for name, totalNs := range p.durationNs {
		perScanner[name] = ScannerJobStats{
			TotalDuration: time.Duration(totalNs),
			Count:         p.durationN[name],
		}
	}
	p.durationMu.Unlock()

	return PoolStats{
		Workers:    p.workers,
		QueueSize:  p.queueSize,
		QueueDepth: len(p.jobs),
		ActiveJobs: p.activeJobs.Load(),
		Submitted:  p.submitted.Load(),
		Completed:  p.completed.Load(),
		Failed:     p.failed.Load(),
		Shed:       p.shed.Load(),
		PerScanner: perScanner,
	}
}

// PoolStats is a snapshot of WorkerPool utilisation.
type PoolStats struct {
	Workers    int
	QueueSize  int
	QueueDepth int
	ActiveJobs int64
	Submitted  int64
	Completed  int64
	Failed     int64
	Shed       int64
	PerScanner map[string]ScannerJobStats
}

// ScannerJobStats holds cumulative duration and count for one scanner.
type ScannerJobStats struct {
	TotalDuration time.Duration
	Count         int64
}

// AvgMs returns the mean job duration in milliseconds, or 0.
func (s ScannerJobStats) AvgMs() float64 {
	if s.Count == 0 {
		return 0
	}
	return float64(s.TotalDuration.Milliseconds()) / float64(s.Count)
}
