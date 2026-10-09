package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/store"
)

// Postgres is the record of what the proxy scanned; the recent buffer is a
// short tail of it, kept in memory, empty after a restart and different on
// every replica. The handlers that count requests or show one therefore ask
// the database when there is one, and use the buffer only when there is none
// or it does not answer. A response is built from one of the two, never from
// both: the buffer's events are also in the database (within a flush
// interval), so adding them would count them twice.
//
// The database is behind the buffer by the request-log writer's flush
// interval (5 seconds, or 100 rows) plus the event bus, so the newest
// requests reach these numbers a few seconds after they reach the live feed.

// requestHistory is what the handlers need from a store that keeps
// request_logs. *store.PostgresStore implements it; the no-op store does not,
// which is how a proxy without a database stays on the recent buffer.
type requestHistory interface {
	GetRequestRecord(ctx context.Context, orgID, requestID string, since time.Time) (*store.RequestRecord, error)
	RequestCounts(ctx context.Context, orgID string, from, to time.Time) ([]store.RequestCount, error)
	RequestTimeseries(ctx context.Context, orgID string, from, to time.Time, bucket string) ([]store.RequestBucket, error)
	FindingCounts(ctx context.Context, orgID string, from, to time.Time) ([]store.FindingCount, error)
	ModelCounts(ctx context.Context, orgID string, from, to time.Time) ([]store.ModelCount, error)
	RiskProfiles(ctx context.Context, orgID string, from, to time.Time) ([]store.RiskProfile, error)
	ExportRequestRecords(ctx context.Context, orgID string, p store.EventSearchParams, max int) ([]store.RequestRecord, bool, error)
}

const (
	// dataSourceHeader says where the numbers in a response came from:
	// "database" (request_logs) or "memory" (the recent buffer, which holds
	// about the last thousand events since this process started).
	dataSourceHeader = "X-Tamga-Data-Source"
	sourceDatabase   = "database"
	sourceMemory     = "memory"
	// exportTruncatedHeader is set to "true" on an export that matched more
	// rows than store.ExportMaxRows.
	exportTruncatedHeader = "X-Tamga-Export-Truncated"

	// historyTimeout bounds the database work of one console request, so a
	// slow database cannot hang the console.
	historyTimeout = 10 * time.Second
	exportTimeout  = 60 * time.Second
	// detailLookback is how far back a request is looked up by id. Request
	// logs are kept 30 days by default.
	detailLookback = 400 * 24 * time.Hour
	// catalogWindow is the window the provider catalog counts models over.
	catalogWindow = 30 * 24 * time.Hour
	// historyLogEvery spaces the warnings about a database that does not answer.
	historyLogEvery = time.Minute
)

// history returns the request history to answer from, or nil when the proxy
// has no database (or no organisation to read it for).
func (cfg Config) history() requestHistory {
	if cfg.DatabaseURL == "" || cfg.DefaultOrgID == "" || cfg.Store == nil {
		return nil
	}
	h, _ := cfg.Store.(requestHistory)
	return h
}

var historyLoggedAt atomic.Int64

// historyUnavailable records that the database did not answer and the recent
// buffer was used instead, at most once per historyLogEvery.
func historyUnavailable(endpoint string, err error) {
	now := time.Now().UnixNano()
	last := historyLoggedAt.Load()
	if last != 0 && now-last < int64(historyLogEvery) {
		return
	}
	if !historyLoggedAt.CompareAndSwap(last, now) {
		return
	}
	log.Warn().Err(err).Str("component", "api").Str("endpoint", endpoint).
		Msg("request history unavailable; answering from the in-memory buffer, which holds recent events only")
}

// requestStats is what GET /stats reports about a window.
type requestStats struct {
	total, blocked, redacted, warned int64
	providers                        map[string]int64
	findingTypes                     map[string]int64
	categories                       map[string]int64
	scanAvgMs                        float64
	// avgRiskPct is nil when it could not be worked out.
	avgRiskPct *int
}

// loadRequestStats reads a window's statistics from the request history.
func loadRequestStats(ctx context.Context, h requestHistory, orgID string, from, to time.Time) (*requestStats, error) {
	counts, err := h.RequestCounts(ctx, orgID, from, to)
	if err != nil {
		return nil, err
	}
	st := &requestStats{providers: map[string]int64{}}
	var latencySum float64
	for _, c := range counts {
		st.total += c.Requests
		switch c.Action {
		case "BLOCK":
			st.blocked += c.Requests
		case "REDACT":
			st.redacted += c.Requests
		case "WARN":
			st.warned += c.Requests
		}
		st.providers[c.Provider] += c.Requests
		latencySum += c.ScanLatencySumMs
	}
	if st.total > 0 {
		st.scanAvgMs = latencySum / float64(st.total)
	}

	findings, err := h.FindingCounts(ctx, orgID, from, to)
	if err != nil {
		return nil, err
	}
	st.findingTypes, st.categories, _, _ = findingBreakdown(findings)

	// The input risk is not stored. It is a function of the findings, which
	// are, so it is worked out from them with the function the proxy used.
	profiles, err := h.RiskProfiles(ctx, orgID, from, to)
	switch {
	case errors.Is(err, store.ErrTooManyGroups):
		// Left out, not guessed.
	case err != nil:
		return nil, err
	default:
		avg := 0
		if st.total > 0 {
			var sum int64
			for _, p := range profiles {
				sum += int64(scanner.CalculateRisk(p.Findings).Percentage) * p.Requests
			}
			avg = int(float64(sum)/float64(st.total) + 0.5)
		}
		st.avgRiskPct = &avg
	}
	return st, nil
}

// findingBreakdown turns grouped finding counts into the maps the breakdown
// and stats responses carry.
func findingBreakdown(rows []store.FindingCount) (byType, byCategory, bySeverity map[string]int64, typeByCategory map[string]map[string]int64) {
	byType = map[string]int64{}
	byCategory = map[string]int64{}
	bySeverity = map[string]int64{}
	typeByCategory = map[string]map[string]int64{}
	for _, f := range rows {
		if f.Type != "" {
			byType[f.Type] += f.Count
		}
		if f.Category != "" {
			byCategory[f.Category] += f.Count
			m := typeByCategory[f.Type]
			if m == nil {
				m = map[string]int64{}
				typeByCategory[f.Type] = m
			}
			m[f.Category] += f.Count
		}
		if f.Severity != "" {
			bySeverity[strings.ToLower(f.Severity)] += f.Count
		}
	}
	return byType, byCategory, bySeverity, typeByCategory
}

// storedAction is a stored row's action as the API spells it.
func storedAction(raw string) string {
	action := strings.ToUpper(strings.TrimSpace(raw))
	if action == "" {
		return "PASS"
	}
	return action
}

// storedEventType names the event a stored row was written for. Only scanned
// and blocked requests are stored, and a blocked request is the one whose
// action is BLOCK.
func storedEventType(action string) string {
	if action == "BLOCK" {
		return "request_blocked"
	}
	return "request_scanned"
}

func storedFindings(raw json.RawMessage) []scanner.Finding {
	findings := []scanner.Finding{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &findings)
	}
	if findings == nil {
		findings = []scanner.Finding{}
	}
	return findings
}

// storedEventJSON is a stored row in the shape the event list and the export
// use. The input risk is worked out from the stored findings; the output
// risk is not stored and is left out.
func storedEventJSON(rec store.RequestRecord) events.EventJSON {
	findings := storedFindings(rec.Findings)
	action := storedAction(rec.ActionTaken)
	risk := scanner.CalculateRisk(findings)
	return events.EventJSON{
		RequestID:      rec.RequestID,
		Provider:       rec.Provider,
		Model:          rec.Model,
		EventType:      storedEventType(action),
		Action:         action,
		Findings:       findings,
		FindingsCount:  rec.FindingsCount,
		Endpoint:       rec.Endpoint,
		ScanLatencyMs:  rec.ScanLatencyMs,
		TotalLatencyMs: rec.TotalLatencyMs,
		Timestamp:      rec.CreatedAt.UTC(),
		InputRiskPct:   risk.Percentage,
		RiskLevel:      risk.Level,
		InputRisk:      &risk,
	}
}

// storedEventDetail is the detail of a request read from request_logs. It
// holds what the row holds. Left out because the row does not record them:
// the output risk, and the name and version of the policy that decided (the
// policy loaded now may be a different one). Each finding's action_taken is
// the request's own action, for the same reason.
func storedEventDetail(rec store.RequestRecord) map[string]interface{} {
	findings := storedFindings(rec.Findings)
	action := storedAction(rec.ActionTaken)
	findingsOut := make([]map[string]interface{}, 0, len(findings))
	for _, f := range findings {
		findingsOut = append(findingsOut, eventFindingDetail(nil, f, policy.Action(action)))
	}
	return map[string]interface{}{
		"request_id":       rec.RequestID,
		"timestamp":        rec.CreatedAt.UTC(),
		"provider":         rec.Provider,
		"model":            rec.Model,
		"action":           action,
		"input_risk":       scanner.CalculateRisk(findings),
		"findings":         findingsOut,
		"scan_latency_ms":  rec.ScanLatencyMs,
		"total_latency_ms": rec.TotalLatencyMs,
		"input_tokens":     rec.InputTokens,
		"output_tokens":    rec.OutputTokens,
		"endpoint":         rec.Endpoint,
		"event_type":       storedEventType(action),
	}
}

// requestEvent reports whether e is the event of a scanned request, the kind
// that is also stored.
func requestEvent(e events.Event) bool {
	return e.EventType == "request_scanned" || e.EventType == "request_blocked"
}

// writeSourced writes a JSON body and says which source it was built from.
func writeSourced(w http.ResponseWriter, source string, v interface{}) {
	w.Header().Set(dataSourceHeader, source)
	writeJSON(w, http.StatusOK, v)
}
