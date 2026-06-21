package api

import (
	"net/http"
	"sort"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	dto "github.com/prometheus/client_model/go"
)

// ── Hot-Path Prometheus Histograms ────────────────────────────────────────
// These replace the RecentBuffer-derived latency quantiles in metrics.go
// with true Prometheus histograms that support quantile() queries, SLO
// tracking, and Grafana dashboards.

var (
	// ScanLatencyMs records the combined scan phase duration (all scanners).
	ScanLatencyMs = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "tamga_scan_latency_ms",
		Help: "Scan phase latency in milliseconds (hot path, all scanners).",
		Buckets: []float64{
			0.5, 1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 2500,
		},
	})

	// TotalLatencyMs records the full request duration from arrival to response.
	TotalLatencyMs = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "tamga_total_latency_ms",
		Help: "Total request latency in milliseconds (hot path).",
		Buckets: []float64{
			1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000,
		},
	})

	// ScannerLatencyMs records per-scanner execution time, labeled by scanner name.
	ScannerLatencyMs = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "tamga_scanner_latency_ms",
		Help: "Per-scanner latency in milliseconds.",
		Buckets: []float64{
			0.1, 0.5, 1, 2, 5, 10, 25, 50, 100,
		},
	}, []string{"scanner"})

	// PoolUtilization tracks the worker pool saturation ratio (0.0-1.0).
	PoolUtilization = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "tamga_scanner_pool_utilization",
		Help: "Worker pool saturation (active workers / total workers).",
	})
)

// ObserveScan records scan latency on the hot-path histogram.
func ObserveScan(ms float64) { ScanLatencyMs.Observe(ms) }

// ObserveTotal records total request latency on the hot-path histogram.
func ObserveTotal(ms float64) { TotalLatencyMs.Observe(ms) }

// ObserveScanner records per-scanner latency with the scanner name label.
func ObserveScanner(scanner string, ms float64) {
	ScannerLatencyMs.WithLabelValues(scanner).Observe(ms)
}

// histogramBucketJSON is the JSON representation of a single histogram bucket.
type histogramBucketJSON struct {
	Le         float64 `json:"le"`
	Cumulative uint64  `json:"cumulative"`
}

// histogramMetricJSON is the JSON representation of one histogram metric.
type histogramMetricJSON struct {
	Name    string                `json:"name"`
	Help    string                `json:"help"`
	Labels  map[string]string     `json:"labels,omitempty"`
	Count   uint64                `json:"count"`
	Sum     float64               `json:"sum"`
	Buckets []histogramBucketJSON `json:"buckets"`
}

// handleMetricsHistograms exposes Prometheus histogram metrics as JSON.
// GET /api/v1/metrics/histograms
func (cfg Config) handleMetricsHistograms(w http.ResponseWriter, r *http.Request) {
	// Ensure at least one observation exists so the histograms appear in the
	// gather output even when the proxy just started.
	ScanLatencyMs.Observe(0)
	TotalLatencyMs.Observe(0)

	gathered, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	out := make([]histogramMetricJSON, 0, len(gathered))
	histogramNames := map[string]bool{
		"tamga_scan_latency_ms":    true,
		"tamga_total_latency_ms":   true,
		"tamga_scanner_latency_ms": true,
	}

	for _, mf := range gathered {
		name := mf.GetName()
		if !histogramNames[name] {
			// Only expose the known histograms; skip gauges and counters.
			continue
		}
		for _, m := range mf.GetMetric() {
			h := m.GetHistogram()
			if h == nil {
				continue
			}
			entry := histogramMetricJSON{
				Name:   name,
				Help:   mf.GetHelp(),
				Count:  h.GetSampleCount(),
				Sum:    h.GetSampleSum(),
				Labels: labelPairsToMap(m.GetLabel()),
			}
			for _, b := range h.GetBucket() {
				entry.Buckets = append(entry.Buckets, histogramBucketJSON{
					Le:         b.GetUpperBound(),
					Cumulative: b.GetCumulativeCount(),
				})
			}
			sort.Slice(entry.Buckets, func(i, j int) bool {
				return entry.Buckets[i].Le < entry.Buckets[j].Le
			})
			out = append(out, entry)
		}
	}
	if out == nil {
		out = []histogramMetricJSON{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"histograms": out,
	})
}

// labelPairsToMap converts []*dto.LabelPair to a map[string]string.
func labelPairsToMap(labels []*dto.LabelPair) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	m := make(map[string]string, len(labels))
	for _, lp := range labels {
		if lp.GetName() != "" {
			m[lp.GetName()] = lp.GetValue()
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}
