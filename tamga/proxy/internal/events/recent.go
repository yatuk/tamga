package events

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/yatuk/tamga/internal/providers"
	"github.com/yatuk/tamga/internal/scanner"
)

// RecentBuffer keeps the last N events for dashboard fallback when DB is off.
type RecentBuffer struct {
	mu   sync.Mutex
	n    int
	buf  []Event
	byID map[string]Event
}

// NewRecentBuffer creates a ring buffer that holds the most recent N events.
func NewRecentBuffer(capacity int) *RecentBuffer {
	if capacity < 1 {
		capacity = 100
	}
	return &RecentBuffer{n: capacity, byID: make(map[string]Event)}
}

func (b *RecentBuffer) Add(e Event) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.byID == nil {
		b.byID = make(map[string]Event)
	}
	if len(b.buf) >= b.n {
		old := b.buf[0]
		b.buf = append(b.buf[:0], b.buf[1:]...)
		delete(b.byID, old.RequestID)
	}
	b.buf = append(b.buf, e)
	if e.RequestID != "" {
		b.byID[e.RequestID] = e
	}
}

// GetByRequestID returns the most recent buffered event for the given request id.
func (b *RecentBuffer) GetByRequestID(id string) (Event, bool) {
	if b == nil || id == "" {
		return Event{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.byID == nil {
		return Event{}, false
	}
	e, ok := b.byID[id]
	return e, ok
}

// Page returns newest-first events for page (1-based) and page size.
func (b *RecentBuffer) Page(page, limit int) ([]Event, int) {
	if b == nil {
		return nil, 0
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	total := len(b.buf)
	if total == 0 {
		return nil, 0
	}
	offset := (page - 1) * limit
	if offset >= total {
		return nil, total
	}
	// newest at end — walk backwards
	var out []Event
	for i := total - 1 - offset; i >= 0 && len(out) < limit; i-- {
		out = append(out, b.buf[i])
	}
	return out, total
}

// Search returns newest-first events matching match, then paginates.
// Used when PostgreSQL is unavailable but the dashboard still needs
// GET /events?action=&provider= filters against the in-memory ring.
func (b *RecentBuffer) Search(page, limit int, match func(Event) bool) ([]Event, int) {
	if b == nil || match == nil {
		return nil, 0
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	totalBuf := len(b.buf)
	if totalBuf == 0 {
		return nil, 0
	}
	var matched []Event
	for i := totalBuf - 1; i >= 0; i-- {
		e := b.buf[i]
		if match(e) {
			matched = append(matched, e)
		}
	}
	tn := len(matched)
	offset := (page - 1) * limit
	if offset >= tn {
		return nil, tn
	}
	end := offset + limit
	if end > tn {
		end = tn
	}
	return matched[offset:end], tn
}

// MatchEventSearch mirrors store.EventSearchParams semantics for RAM buffer.
func MatchEventSearch(e Event, action, provider string, shadowOnly bool, findingType, severity, category, technique, q string, since, until time.Time) bool {
	if !since.IsZero() && e.Timestamp.Before(since) {
		return false
	}
	if !until.IsZero() && e.Timestamp.After(until) {
		return false
	}
	if a := strings.TrimSpace(strings.ToUpper(action)); a != "" && strings.ToUpper(strings.TrimSpace(e.Action)) != a {
		return false
	}
	p := strings.ToLower(strings.TrimSpace(provider))
	if shadowOnly || p == "shadow" {
		if e.Provider == "" || providers.IsEnterprise(e.Provider) {
			return false
		}
	} else if p != "" && p != "all" {
		if strings.ToLower(e.Provider) != p {
			return false
		}
	}
	if ft := strings.TrimSpace(findingType); ft != "" {
		ok := false
		for _, f := range e.Findings {
			if strings.EqualFold(f.Type, ft) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if sev := strings.TrimSpace(severity); sev != "" {
		ok := false
		for _, f := range e.Findings {
			if strings.EqualFold(f.Severity, sev) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if cat := strings.TrimSpace(category); cat != "" {
		ok := false
		for _, f := range e.Findings {
			if strings.Contains(strings.ToLower(f.Category), strings.ToLower(cat)) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if tech := strings.TrimSpace(technique); tech != "" {
		raw, _ := json.Marshal(e.Findings)
		if !strings.Contains(strings.ToLower(string(raw)), strings.ToLower(tech)) {
			return false
		}
	}
	if qq := strings.TrimSpace(q); qq != "" {
		ql := strings.ToLower(qq)
		raw, _ := json.Marshal(e.Findings)
		if !strings.Contains(strings.ToLower(e.RequestID), ql) && !strings.Contains(strings.ToLower(string(raw)), ql) {
			return false
		}
	}
	return true
}

// RecentBufferHandler appends every event to the buffer.
func RecentBufferHandler(b *RecentBuffer) func(Event) {
	return func(e Event) {
		b.Add(e)
	}
}

// EventJSON is a JSON-serializable view of Event for APIs.
type EventJSON struct {
	RequestID      string             `json:"request_id"`
	OrgID          string             `json:"org_id,omitempty"`
	Provider       string             `json:"provider,omitempty"`
	Model          string             `json:"model,omitempty"`
	EventType      string             `json:"event_type"`
	Action         string             `json:"action,omitempty"`
	Findings       []scanner.Finding  `json:"findings"`
	FindingsCount  int                `json:"findings_count"`
	Endpoint       string             `json:"endpoint,omitempty"`
	ScanLatencyMs  float64            `json:"scan_latency_ms,omitempty"`
	TotalLatencyMs float64            `json:"total_latency_ms,omitempty"`
	ContentType    string             `json:"content_type,omitempty"`
	Timestamp      time.Time          `json:"timestamp"`
	InputRiskPct   int                `json:"input_risk_pct,omitempty"`
	RiskLevel      string             `json:"risk_level,omitempty"`
	InputRisk      *scanner.RiskScore `json:"input_risk,omitempty"`
	OutputRisk     *scanner.RiskScore `json:"output_risk,omitempty"`
}

// EventToJSON converts an internal Event to its JSON-serialisable representation.
func EventToJSON(e Event) EventJSON {
	j := EventJSON{
		RequestID:      e.RequestID,
		OrgID:          e.OrgID,
		Provider:       e.Provider,
		Model:          e.Model,
		EventType:      e.EventType,
		Action:         e.Action,
		Findings:       e.Findings,
		FindingsCount:  len(e.Findings),
		Endpoint:       e.Endpoint,
		ScanLatencyMs:  e.ScanLatencyMs,
		TotalLatencyMs: e.TotalLatencyMs,
		ContentType:    e.ContentType,
		Timestamp:      e.Timestamp,
	}
	if e.EventType == "request_scanned" || e.EventType == "request_blocked" {
		j.InputRiskPct = e.InputRisk.Percentage
		j.RiskLevel = e.InputRisk.Level
		if j.RiskLevel == "" && e.InputRisk.Percentage == 0 {
			j.RiskLevel = "none"
		}
		ir := e.InputRisk
		or := e.OutputRisk
		j.InputRisk = &ir
		j.OutputRisk = &or
	}
	return j
}

// MarshalEventsJSON serialises a slice of events to a JSON byte array.
func MarshalEventsJSON(events []Event) ([]byte, error) {
	out := make([]EventJSON, 0, len(events))
	for _, e := range events {
		out = append(out, EventToJSON(e))
	}
	return json.Marshal(out)
}
