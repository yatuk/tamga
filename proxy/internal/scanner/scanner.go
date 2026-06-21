package scanner

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Finding represents a single security finding from a scanner.
type Finding struct {
	Type       string  `json:"type"`     // "pii", "secret", "injection"
	Severity   string  `json:"severity"` // "critical", "high", "medium", "low"
	Match      string  `json:"match"`    // The matched content (masked for PII)
	Category   string  `json:"category"` // Subcategory: "credit_card", "aws_key", etc.
	StartPos   int     `json:"start_pos"`
	EndPos     int     `json:"end_pos"`
	Confidence float64 `json:"confidence"` // 0.0 - 1.0
	// ConfidenceScore is the Sprint 5 confidence matrix output (0-100 + action).
	// Kept optional for backward compatibility with existing API consumers.
	ConfidenceScore *ConfidenceScore  `json:"confidence_score,omitempty"`
	ActionTaken     string            `json:"action_taken,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	// ScannerVersion identifies the scanner that produced this finding (semver).
	ScannerVersion string `json:"scanner_version,omitempty"`
	// DatasetVersion identifies the reference dataset version used (e.g. "2026-Q2").
	DatasetVersion string `json:"dataset_version,omitempty"`
	// ProximityBoost records the confidence boost applied by contextual
	// proximity scoring. Zero means no proximity boost was applied.
	ProximityBoost float64 `json:"proximity_boost,omitempty"`
}

// ScanResult wraps a scanner's output with execution metadata. Used by the
// WorkerPool and pipeline to collect per-scanner timing without extra
// allocations.
type ScanResult struct {
	Scanner  string
	Findings []Finding
	Error    error
	Duration time.Duration
}

// Scanner is the interface all scanners must implement.
type Scanner interface {
	Name() string
	Scan(ctx context.Context, content []byte) ([]Finding, error)
}

// scannerDetectionCounts tracks total findings per scanner name for Prometheus export.
var scannerDetectionCounts sync.Map // map[string]*int64

// ScannerDetectionStats returns a snapshot of per-scanner detection counts.
func ScannerDetectionStats() map[string]int64 {
	out := make(map[string]int64)
	scannerDetectionCounts.Range(func(key, value any) bool {
		name := key.(string)
		counter := value.(*int64)
		out[name] = atomic.LoadInt64(counter)
		return true
	})
	return out
}

// Registry holds all registered scanners and runs them.
type Registry struct {
	mu       sync.RWMutex
	scanners []Scanner
	speeds   map[string]ScannerSpeed // per-scanner speed classification
}

// SetSpeed records a speed classification for a scanner. Must be called
// before ScanAll for each registered scanner.
func (r *Registry) SetSpeed(name string, speed ScannerSpeed) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.speeds == nil {
		r.speeds = make(map[string]ScannerSpeed)
	}
	r.speeds[name] = speed
}

// NewRegistry creates a Scanner registry for registering and running security scanners.
func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) Register(s Scanner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scanners = append(r.scanners, s)
}

func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.scanners)
}

// ScanAll runs all registered scanners using the default adaptive pipeline
// strategy and returns combined findings. Fast scanners run sequentially;
// slow scanners run in parallel goroutines with WaitGroup synchronisation.
func (r *Registry) ScanAll(ctx context.Context, content []byte) ([]Finding, error) {
	return r.ScanAllWithConfig(ctx, content, PipelineConfig{Mode: ModeAdaptive})
}

// ScanAllWithConfig runs all registered scanners using the given pipeline
// configuration. Use PipelineConfig{Mode: ModeSync} for sequential-only,
// ModeAsync for parallel-only, or ModeAdaptive for the hybrid strategy.
func (r *Registry) ScanAllWithConfig(ctx context.Context, content []byte, cfg PipelineConfig) ([]Finding, error) {
	r.mu.RLock()
	speeds := r.speeds // snapshot under lock
	entries := make([]ScannerEntry, len(r.scanners))
	for i, s := range r.scanners {
		sp := SpeedFast
		if speeds != nil {
			if v, ok := speeds[s.Name()]; ok {
				sp = v
			}
		}
		entries[i] = ScannerEntry{Scanner: s, Speed: sp}
	}
	r.mu.RUnlock()

	p := NewPipelineWithConfig(entries, cfg)
	return p.Scan(ctx, content)
}
