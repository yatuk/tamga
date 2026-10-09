package scanner

import (
	"context"
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

// Why a scanner stopped before the end of the text. An error that wraps one
// of these also wraps ErrScanDegraded.
var (
	ErrFindingsCap = errors.New("findings cap reached")
	ErrTimeBudget  = errors.New("time budget spent")
)

// Reasons IncompleteReason gives. They are part of the refusal a caller
// reads and of the "reason" label of tamga_scan_degraded_total.
const (
	IncompleteFindingsCap   = "findings_cap"
	IncompleteTimeBudget    = "time_budget"
	IncompleteDeadline      = "deadline"
	IncompleteCancelled     = "cancelled"
	IncompleteScannerFailed = "scanner_failed"
)

// IncompleteReason names why a scan that returned err did not cover the
// whole text. A scanner that was cut short (the findings cap, its time
// budget, the deadline) is told apart from one that failed outright; when
// several scanners stopped, the reason is that of the first one reported.
func IncompleteReason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrFindingsCap):
		return IncompleteFindingsCap
	case errors.Is(err, ErrTimeBudget):
		return IncompleteTimeBudget
	case errors.Is(err, context.DeadlineExceeded):
		return IncompleteDeadline
	case errors.Is(err, context.Canceled):
		return IncompleteCancelled
	}
	return IncompleteScannerFailed
}

// stopError is what a scanner returns when it stops early: ErrScanDegraded,
// and the reason it stopped.
type stopError struct {
	msg    string
	reason error
}

func (e *stopError) Error() string   { return ErrScanDegraded.Error() + ": " + e.msg }
func (e *stopError) Unwrap() []error { return []error{ErrScanDegraded, e.reason} }

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
