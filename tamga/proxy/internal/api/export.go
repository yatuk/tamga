package api

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/events"
)

// handleEventsExport returns filtered recent events as CSV or JSON.
//
// Query params (all optional):
//
//	format=csv|json (default csv)
//	action=BLOCK|REDACT|WARN|LOG|PASS
//	provider=openai|anthropic|...|shadow
//	range=24h|7d|30d (default 7d)
//	request_id=<substring>
func (cfg Config) handleEventsExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := strings.ToLower(q.Get("format"))
	if format == "" {
		format = "csv"
	}
	action := strings.ToUpper(strings.TrimSpace(q.Get("action")))
	provider := strings.ToLower(strings.TrimSpace(q.Get("provider")))
	rng := strings.ToLower(q.Get("range"))
	if rng == "" {
		rng = "7d"
	}
	ridSub := strings.ToLower(strings.TrimSpace(q.Get("request_id")))

	threshold := time.Now().UTC().UnixMilli() - rangeMillis(rng)
	rows := []events.EventJSON{}
	if cfg.Recent != nil {
		evs, _ := cfg.Recent.Page(1, 1000)
		for _, e := range evs {
			if e.Timestamp.IsZero() || e.Timestamp.UnixMilli() < threshold {
				continue
			}
			if action != "" && action != "ALL" && strings.ToUpper(e.Action) != action {
				continue
			}
			if provider != "" && provider != "all" {
				if provider == "shadow" {
					p := strings.ToLower(e.Provider)
					if p == "" || enterpriseProviders[p] {
						continue
					}
				} else if !strings.EqualFold(e.Provider, provider) {
					continue
				}
			}
			if ridSub != "" && !strings.Contains(strings.ToLower(e.RequestID), ridSub) {
				continue
			}
			rows = append(rows, events.EventToJSON(e))
		}
	}

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=tamga-events-%s.json", time.Now().UTC().Format("20060102-150405")))
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"events": rows, "total": len(rows)})
	default:
		var buf bytes.Buffer
		cw := csv.NewWriter(&buf)
		_ = cw.Write([]string{"request_id", "timestamp", "provider", "model", "action", "event_type", "findings_count", "scan_latency_ms", "input_risk_pct"})
		for _, row := range rows {
			_ = cw.Write([]string{
				csvSafe(row.RequestID),
				row.Timestamp.Format(time.RFC3339Nano),
				csvSafe(row.Provider),
				csvSafe(row.Model),
				csvSafe(row.Action),
				csvSafe(row.EventType),
				fmt.Sprintf("%d", row.FindingsCount),
				fmt.Sprintf("%.2f", row.ScanLatencyMs),
				fmt.Sprintf("%d", row.InputRiskPct),
			})
		}
		cw.Flush()
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=tamga-events-%s.csv", time.Now().UTC().Format("20060102-150405")))
		_, _ = w.Write(buf.Bytes())
	}
}

// csvSafe neutralises spreadsheet formula injection for cells that start
// with =, @, +, or - by prefixing them with a single quote.
func csvSafe(v string) string {
	if len(v) > 0 && (v[0] == '=' || v[0] == '@' || v[0] == '+' || v[0] == '-') {
		return "'" + v
	}
	return v
}

// enterpriseProviders mirrors the dashboard "shadow AI" heuristic.
var enterpriseProviders = map[string]bool{
	"openai":        true,
	"anthropic":     true,
	"google":        true,
	"azure":         true,
	"azure_openai":  true,
	"google_vertex": true,
}
