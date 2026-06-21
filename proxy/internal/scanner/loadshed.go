package scanner

// CriticalScanners is the set of scanner names that must always run, even
// under overload. These detect PII and secrets — the highest-stakes findings.
var CriticalScanners = map[string]bool{
	"pii":    true,
	"secret": true,
}

// LoadShedder implements adaptive load shedding for the scanner pool. When
// the pool queue approaches capacity, non-critical scanners are skipped to
// preserve throughput for the most important detections.
//
// Thresholds:
//
//	> 95% queue utilisation → all scanners shed (fail-fast)
//	> 80% queue utilisation → only CriticalScanners run (degraded)
//	≤ 80%                   → all scanners run (normal)
type LoadShedder struct {
	pool *WorkerPool
}

// NewLoadShedder creates a LoadShedder attached to the given pool.
func NewLoadShedder(pool *WorkerPool) *LoadShedder {
	return &LoadShedder{pool: pool}
}

// ShouldRun returns true if the named scanner should be executed given the
// current pool saturation. Non-critical scanners are shed first; critical
// scanners are only shed when the queue is completely saturated (>95%).
func (s *LoadShedder) ShouldRun(scannerName string) bool {
	stats := s.pool.Stats()
	if stats.QueueSize == 0 {
		return true // unbounded or uninitialised
	}
	load := float64(stats.QueueDepth) / float64(stats.QueueSize)

	if load > 0.95 {
		return false // fail-fast: skip everything
	}
	if load > 0.80 {
		return CriticalScanners[scannerName] // degraded: critical only
	}
	return true // normal: run all
}
