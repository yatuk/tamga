package api

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/events"
)

// handleTimeseries returns a bucketed time series for the overview page.
//
// Query params:
//
//	range=24h|7d|30d (default 7d)
//	bucket=hour|day  (default: hour for 24h, day otherwise)
//
// The response is:
//
//	{ "range": "...", "bucket": "...", "points": [ { "t": ISO, "total", "blocked", "redacted", "warned", "scan_p95" } ] }
func (cfg Config) handleTimeseries(w http.ResponseWriter, r *http.Request) {
	rng := strings.ToLower(r.URL.Query().Get("range"))
	if rng == "" {
		rng = "7d"
	}
	bucket := strings.ToLower(r.URL.Query().Get("bucket"))
	windowMs := rangeMillis(rng)
	if bucket == "" {
		if rng == "24h" {
			bucket = "hour"
		} else {
			bucket = "day"
		}
	}
	bucketMs := bucketMillis(bucket)
	if bucketMs <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid bucket"})
		return
	}

	// Pull recent events (in-mem) — DB-backed version can replace this block.
	points := map[int64]*tsPoint{}
	nowMs := time.Now().UTC().UnixMilli()
	threshold := nowMs - windowMs
	if cfg.Recent != nil {
		evs, _ := cfg.Recent.Page(1, 1000)
		for _, e := range evs {
			if e.Timestamp.IsZero() {
				continue
			}
			ts := e.Timestamp.UnixMilli()
			if ts < threshold {
				continue
			}
			key := (ts / bucketMs) * bucketMs
			p, ok := points[key]
			if !ok {
				p = &tsPoint{T: time.UnixMilli(key).UTC()}
				points[key] = p
			}
			switch e.EventType {
			case "request_scanned", "request_blocked":
				p.Total++
				switch strings.ToUpper(e.Action) {
				case "BLOCK":
					p.Blocked++
				case "REDACT":
					p.Redacted++
				case "WARN":
					p.Warned++
				}
				p.scanLatencies = append(p.scanLatencies, e.ScanLatencyMs)
			}
		}
	}

	// Ensure the series includes every completed bucket slot in the window
	// so the client can render a continuous axis (no "holes").
	// Snap stop to bucket boundary to exclude the current partial bucket,
	// avoiding an off-by-one extra slot (e.g. 25 pts for a 24h window).
	startMs := (threshold / bucketMs) * bucketMs
	stopMs := (nowMs / bucketMs) * bucketMs
	for t := startMs; t < stopMs; t += bucketMs {
		if _, ok := points[t]; !ok {
			points[t] = &tsPoint{T: time.UnixMilli(t).UTC()}
		}
	}
	out := make([]map[string]interface{}, 0, len(points))
	keys := make([]int64, 0, len(points))
	for k := range points {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		p := points[k]
		out = append(out, map[string]interface{}{
			"t":        p.T,
			"total":    p.Total,
			"blocked":  p.Blocked,
			"redacted": p.Redacted,
			"warned":   p.Warned,
			"scan_p95": percentile(p.scanLatencies, 0.95),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"range":  rng,
		"bucket": bucket,
		"points": out,
	})
}

type tsPoint struct {
	T             time.Time
	Total         int
	Blocked       int
	Redacted      int
	Warned        int
	scanLatencies []float64
}

func rangeMillis(r string) int64 {
	switch r {
	case "1h":
		return int64(time.Hour / time.Millisecond)
	case "24h":
		return int64(24 * time.Hour / time.Millisecond)
	case "30d":
		return int64(30 * 24 * time.Hour / time.Millisecond)
	default:
		return int64(7 * 24 * time.Hour / time.Millisecond)
	}
}

func bucketMillis(b string) int64 {
	switch b {
	case "minute":
		return int64(time.Minute / time.Millisecond)
	case "hour":
		return int64(time.Hour / time.Millisecond)
	case "day":
		return int64(24 * time.Hour / time.Millisecond)
	}
	return 0
}

func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	cp := append([]float64(nil), xs...)
	sort.Float64s(cp)
	idx := int(float64(len(cp)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

// handleBreakdown returns counts grouped by type, category and severity.
func (cfg Config) handleBreakdown(w http.ResponseWriter, r *http.Request) {
	rng := strings.ToLower(r.URL.Query().Get("range"))
	if rng == "" {
		rng = "7d"
	}
	threshold := time.Now().UTC().UnixMilli() - rangeMillis(rng)
	byType := map[string]int64{}
	byCategory := map[string]int64{}
	bySeverity := map[string]int64{}
	typeByCategory := map[string]map[string]int64{}
	if cfg.Recent != nil {
		evs, _ := cfg.Recent.Page(1, 1000)
		for _, e := range evs {
			if e.Timestamp.IsZero() || e.Timestamp.UnixMilli() < threshold {
				continue
			}
			if e.EventType != "request_scanned" && e.EventType != "request_blocked" {
				continue
			}
			for _, f := range e.Findings {
				if f.Type != "" {
					byType[f.Type]++
				}
				if f.Category != "" {
					byCategory[f.Category]++
					m := typeByCategory[f.Type]
					if m == nil {
						m = map[string]int64{}
						typeByCategory[f.Type] = m
					}
					m[f.Category]++
				}
				if f.Severity != "" {
					bySeverity[strings.ToLower(f.Severity)]++
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"range":            rng,
		"by_type":          byType,
		"by_category":      byCategory,
		"by_severity":      bySeverity,
		"type_by_category": typeByCategory,
	})
}

// handleModelStats returns per-model and per-family request counts from the
// in-memory recent buffer, with an optional ?range=24h|7d|30d filter.
func (cfg Config) handleModelStats(w http.ResponseWriter, r *http.Request) {
	rng := strings.ToLower(r.URL.Query().Get("range"))
	if rng == "" {
		rng = "7d"
	}
	threshold := time.Now().UTC().UnixMilli() - rangeMillis(rng)

	byModel := map[string]int64{}
	byFamily := map[string]int64{}

	if cfg.Recent != nil {
		evs, _ := cfg.Recent.Page(1, 1000)
		for _, e := range evs {
			if e.Timestamp.IsZero() || e.Timestamp.UnixMilli() < threshold {
				continue
			}
			if e.EventType != "request_scanned" && e.EventType != "request_blocked" {
				continue
			}
			if e.Model != "" {
				byModel[e.Model]++
			}
			if e.ModelFamily != "" {
				byFamily[e.ModelFamily]++
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"range":     rng,
		"by_model":  byModel,
		"by_family": byFamily,
	})
}

// keep events unused-import-safe if the package ever trims imports.
var _ = events.EventJSON{}
