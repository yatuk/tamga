package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/store"
)

// parseEventSearchQuery builds store.EventSearchParams from GET /events query.
func parseEventSearchQuery(r *http.Request) (store.EventSearchParams, error) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	p := store.EventSearchParams{
		Page:        page,
		Limit:       limit,
		Action:      strings.TrimSpace(q.Get("action")),
		Provider:    strings.TrimSpace(q.Get("provider")),
		FindingType: strings.TrimSpace(q.Get("finding_type")),
		Severity:    strings.TrimSpace(q.Get("severity")),
		Category:    strings.TrimSpace(q.Get("category")),
		Technique:   strings.TrimSpace(q.Get("technique")),
		Q:           strings.TrimSpace(q.Get("q")),
	}
	if q.Get("shadow") == "1" || q.Get("shadow") == "true" {
		p.ShadowOnly = true
	}

	rng := strings.ToLower(strings.TrimSpace(q.Get("range")))
	if rng == "" {
		rng = "7d"
	}
	until := time.Now().UTC()
	var since time.Time
	switch rng {
	case "24h", "1h":
		since = until.Add(-24 * time.Hour)
	case "30d":
		since = until.AddDate(0, 0, -30)
	case "7d":
		fallthrough
	default:
		since = until.AddDate(0, 0, -7)
	}
	if s := strings.TrimSpace(q.Get("since")); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t.UTC()
		}
	}
	if u := strings.TrimSpace(q.Get("until")); u != "" {
		if t, err := time.Parse(time.RFC3339, u); err == nil {
			until = t.UTC()
		}
	}
	p.Since = since
	p.Until = until
	return p, nil
}

func storeSecurityEventToJSON(ev store.SecurityEvent) events.EventJSON {
	var findings []scanner.Finding
	if len(ev.Findings) > 0 {
		_ = json.Unmarshal(ev.Findings, &findings)
	}
	action := strings.ToUpper(strings.TrimSpace(ev.ActionTaken))
	if action == "" {
		action = "PASS"
	}
	return events.EventJSON{
		RequestID:     ev.RequestID,
		Provider:      ev.Provider,
		Model:         ev.Model,
		EventType:     "request_scanned",
		Action:        action,
		Findings:      findings,
		FindingsCount: ev.FindingsCount,
		Timestamp:     ev.CreatedAt,
	}
}
