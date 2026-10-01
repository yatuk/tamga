package scanner

import (
	"errors"
	"sync"
	"sync/atomic"
)

// Reasons a scan ran with less coverage than configured. Exported as the
// "reason" label of tamga_scan_degraded_total.
const (
	DegradedPanic          = "panic"
	DegradedError          = "error"
	DegradedQueueFull      = "queue_full"
	DegradedLoadShed       = "load_shed"
	DegradedOutputTooLarge = "output_too_large"
)

// ErrScanDegraded is returned (wrapped) by the pipeline when at least one
// scanner did not produce a result. Findings from the scanners that did run
// are still returned alongside it.
var ErrScanDegraded = errors.New("scan degraded")

// scanDegradedCounts tracks how often a scan lost coverage, keyed by reason.
var scanDegradedCounts sync.Map // map[string]*int64

// RecordDegraded counts one scanner (or one response) that was not scanned.
// The proxy fails open in these cases, so the counter is the only place the
// loss of coverage shows up unless someone is reading logs.
func RecordDegraded(reason string) {
	val, _ := scanDegradedCounts.LoadOrStore(reason, new(int64))
	atomic.AddInt64(val.(*int64), 1)
}

// ScanDegradedStats returns a snapshot of degraded-scan counts by reason.
func ScanDegradedStats() map[string]int64 {
	out := make(map[string]int64)
	scanDegradedCounts.Range(func(key, value any) bool {
		out[key.(string)] = atomic.LoadInt64(value.(*int64))
		return true
	})
	return out
}
