package api

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

// handleMetrics exposes a Prometheus text-format snapshot of the proxy.
//
// This is a minimal, dependency-free implementation that emits the key
// counters already tracked by events.Metrics plus a few runtime gauges. We
// intentionally keep it under adminAuth since it can reveal traffic volume.
func (cfg Config) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.1.1; charset=utf-8")

	if cfg.Metrics != nil {
		_, _ = fmt.Fprintln(w, "# HELP tamga_requests_total Total proxied requests scanned.")
		_, _ = fmt.Fprintln(w, "# TYPE tamga_requests_total counter")
		_, _ = fmt.Fprintf(w, "tamga_requests_total %d\n", cfg.Metrics.TotalRequests.Load())
		_, _ = fmt.Fprintln(w, "# HELP tamga_blocked_total Total requests blocked by policy.")
		_, _ = fmt.Fprintln(w, "# TYPE tamga_blocked_total counter")
		_, _ = fmt.Fprintf(w, "tamga_blocked_total %d\n", cfg.Metrics.Blocked.Load())
		_, _ = fmt.Fprintln(w, "# HELP tamga_redacted_total Total requests redacted inline.")
		_, _ = fmt.Fprintln(w, "# TYPE tamga_redacted_total counter")
		_, _ = fmt.Fprintf(w, "tamga_redacted_total %d\n", cfg.Metrics.Redacted.Load())
		_, _ = fmt.Fprintln(w, "# HELP tamga_warned_total Total requests that triggered a WARN action.")
		_, _ = fmt.Fprintln(w, "# TYPE tamga_warned_total counter")
		_, _ = fmt.Fprintf(w, "tamga_warned_total %d\n", cfg.Metrics.Warned.Load())
	}

	// Latency histogram approximation from RecentBuffer — real bucket metrics
	// would be emitted by the proxy hot path. This derives approximate values
	// so a scraping Prometheus can plot p50/p95 without extra wiring.
	if cfg.Recent != nil {
		evs, _ := cfg.Recent.Page(1, 500)
		lat := make([]float64, 0, len(evs))
		for _, e := range evs {
			if e.ScanLatencyMs > 0 {
				lat = append(lat, e.ScanLatencyMs)
			}
		}
		if len(lat) > 0 {
			sort.Float64s(lat)
			p := func(q float64) float64 {
				idx := int(float64(len(lat)-1) * q)
				if idx < 0 {
					idx = 0
				}
				if idx >= len(lat) {
					idx = len(lat) - 1
				}
				return lat[idx]
			}
			_, _ = fmt.Fprintln(w, "# HELP tamga_scan_latency_ms Scan latency in milliseconds (derived).")
			_, _ = fmt.Fprintln(w, "# TYPE tamga_scan_latency_ms summary")
			_, _ = fmt.Fprintf(w, "tamga_scan_latency_ms{quantile=\"0.5\"} %.2f\n", p(0.5))
			_, _ = fmt.Fprintf(w, "tamga_scan_latency_ms{quantile=\"0.9\"} %.2f\n", p(0.9))
			_, _ = fmt.Fprintf(w, "tamga_scan_latency_ms{quantile=\"0.99\"} %.2f\n", p(0.99))
		}
	}

	// DFA build metrics (referenced in PATTERN_UPDATE_RUNBOOK.md alert thresholds).
	buildCount, lastBuildMs, patternBytes, totalPatterns := scanner.DFAStats()
	_, _ = fmt.Fprintln(w, "# HELP tamga_dfa_build_seconds DFA automaton build duration in seconds.")
	_, _ = fmt.Fprintln(w, "# TYPE tamga_dfa_build_seconds gauge")
	_, _ = fmt.Fprintf(w, "tamga_dfa_build_seconds %.3f\n", float64(lastBuildMs)/1000.0)
	_, _ = fmt.Fprintln(w, "# HELP tamga_dfa_pattern_bytes Approximate memory occupied by DFA patterns.")
	_, _ = fmt.Fprintln(w, "# TYPE tamga_dfa_pattern_bytes gauge")
	_, _ = fmt.Fprintf(w, "tamga_dfa_pattern_bytes %d\n", patternBytes)
	_, _ = fmt.Fprintln(w, "# HELP tamga_dfa_patterns_total Total patterns loaded into the DFA.")
	_, _ = fmt.Fprintln(w, "# TYPE tamga_dfa_patterns_total gauge")
	_, _ = fmt.Fprintf(w, "tamga_dfa_patterns_total %d\n", totalPatterns)
	_, _ = fmt.Fprintln(w, "# HELP tamga_dfa_build_total Total DFA rebuilds since process start.")
	_, _ = fmt.Fprintln(w, "# TYPE tamga_dfa_build_total counter")
	_, _ = fmt.Fprintf(w, "tamga_dfa_build_total %d\n", buildCount)

	// Per-scanner detection counters.
	_, _ = fmt.Fprintln(w, "# HELP tamga_scanner_detections_total Total findings per scanner type.")
	_, _ = fmt.Fprintln(w, "# TYPE tamga_scanner_detections_total counter")
	for name, count := range scanner.ScannerDetectionStats() {
		_, _ = fmt.Fprintf(w, "tamga_scanner_detections_total{scanner=\"%s\"} %d\n", name, count)
	}

	// Scanner worker pool metrics (only when pool is enabled).
	if cfg.ScannerPool != nil {
		stats := cfg.ScannerPool.Stats()

		_, _ = fmt.Fprintln(w, "# HELP tamga_scanner_pool_workers Worker count by state.")
		_, _ = fmt.Fprintln(w, "# TYPE tamga_scanner_pool_workers gauge")
		idle := stats.Workers - int(stats.ActiveJobs)
		if idle < 0 {
			idle = 0
		}
		_, _ = fmt.Fprintf(w, "tamga_scanner_pool_workers{state=\"idle\"} %d\n", idle)
		_, _ = fmt.Fprintf(w, "tamga_scanner_pool_workers{state=\"active\"} %d\n", stats.ActiveJobs)

		_, _ = fmt.Fprintln(w, "# HELP tamga_scanner_pool_queue_depth Current jobs in queue.")
		_, _ = fmt.Fprintln(w, "# TYPE tamga_scanner_pool_queue_depth gauge")
		_, _ = fmt.Fprintf(w, "tamga_scanner_pool_queue_depth %d\n", stats.QueueDepth)

		_, _ = fmt.Fprintln(w, "# HELP tamga_scanner_pool_queue_size Maximum queue capacity.")
		_, _ = fmt.Fprintln(w, "# TYPE tamga_scanner_pool_queue_size gauge")
		_, _ = fmt.Fprintf(w, "tamga_scanner_pool_queue_size %d\n", stats.QueueSize)

		if stats.Workers > 0 {
			util := float64(stats.ActiveJobs) / float64(stats.Workers)
			_, _ = fmt.Fprintln(w, "# HELP tamga_scanner_pool_utilization Pool saturation (0.0-1.0).")
			_, _ = fmt.Fprintln(w, "# TYPE tamga_scanner_pool_utilization gauge")
			_, _ = fmt.Fprintf(w, "tamga_scanner_pool_utilization %.4f\n", util)
		}

		_, _ = fmt.Fprintln(w, "# HELP tamga_scanner_pool_jobs_total Job counts by status.")
		_, _ = fmt.Fprintln(w, "# TYPE tamga_scanner_pool_jobs_total counter")
		_, _ = fmt.Fprintf(w, "tamga_scanner_pool_jobs_total{status=\"submitted\"} %d\n", stats.Submitted)
		_, _ = fmt.Fprintf(w, "tamga_scanner_pool_jobs_total{status=\"completed\"} %d\n", stats.Completed)
		_, _ = fmt.Fprintf(w, "tamga_scanner_pool_jobs_total{status=\"failed\"} %d\n", stats.Failed)

		_, _ = fmt.Fprintln(w, "# HELP tamga_scanner_pool_jobs_shed_total Jobs rejected due to queue full.")
		_, _ = fmt.Fprintln(w, "# TYPE tamga_scanner_pool_jobs_shed_total counter")
		_, _ = fmt.Fprintf(w, "tamga_scanner_pool_jobs_shed_total %d\n", stats.Shed)

		// Per-scanner average latency from pool job durations.
		if len(stats.PerScanner) > 0 {
			_, _ = fmt.Fprintln(w, "# HELP tamga_scanner_job_duration_ms_avg Mean job duration per scanner (milliseconds).")
			_, _ = fmt.Fprintln(w, "# TYPE tamga_scanner_job_duration_ms_avg gauge")
			for name, js := range stats.PerScanner {
				_, _ = fmt.Fprintf(w, "tamga_scanner_job_duration_ms_avg{scanner=\"%s\"} %.2f\n", name, js.AvgMs())
			}
		}
	}

	// Dropped events counter — alerts operators when the event bus buffer is
	// undersized for current throughput. Exposed via /health/detailed as
	// "events_dropped" and here for Prometheus scraping.
	if cfg.Bus != nil {
		dropped := cfg.Bus.Dropped()
		_, _ = fmt.Fprintln(w, "# HELP tamga_events_dropped_total Events silently dropped due to full bus buffer.")
		_, _ = fmt.Fprintln(w, "# TYPE tamga_events_dropped_total counter")
		_, _ = fmt.Fprintf(w, "tamga_events_dropped_total %d\n", dropped)
	}

	// Cache hit/miss counters — expose prompt cache efficiency for
	// capacity planning and cache warming diagnostics.
	if cfg.Cache != nil {
		hits, misses, _ := cfg.Cache.Stats()
		_, _ = fmt.Fprintln(w, "# HELP tamga_cache_hits_total Cumulative prompt cache hits.")
		_, _ = fmt.Fprintln(w, "# TYPE tamga_cache_hits_total counter")
		_, _ = fmt.Fprintf(w, "tamga_cache_hits_total %d\n", hits)
		_, _ = fmt.Fprintln(w, "# HELP tamga_cache_misses_total Cumulative prompt cache misses.")
		_, _ = fmt.Fprintln(w, "# TYPE tamga_cache_misses_total counter")
		_, _ = fmt.Fprintf(w, "tamga_cache_misses_total %d\n", misses)
	}

	if !cfg.Started.IsZero() {
		_, _ = fmt.Fprintln(w, "# HELP tamga_uptime_seconds Process uptime in seconds.")
		_, _ = fmt.Fprintln(w, "# TYPE tamga_uptime_seconds gauge")
		_, _ = fmt.Fprintf(w, "tamga_uptime_seconds %.0f\n", time.Since(cfg.Started).Seconds())
	}
}
