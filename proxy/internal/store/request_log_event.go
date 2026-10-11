package store

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// A request is stored once, from its own event. That event is published when
// the request is over (after the provider answered and the answer was
// relayed), so it already carries what the response told the proxy: tokens,
// cost, and what the scan of the response found. There are two ways to the
// table, the in-process bus (DBHandler) and the NATS consumer; both build
// their row here, so the row does not depend on the way it took.

// isRequestEvent reports whether an event of this type is a request's own.
// Output hints, response scans and provider state changes travel on the same
// bus and are not rows.
func isRequestEvent(eventType string) bool {
	return eventType == "request_scanned" || eventType == "request_blocked"
}

// RequestLogFromEvent builds the request_logs row for a request's event. It
// returns false for any other event, and for one that names no organisation
// when there is no default.
//
// When the active policy has data.hash_findings on, the match of every
// finding, of the request and of the response, is replaced by its keyed
// digest (KVKK/GDPR, see FindingHasher): the text cannot be recovered from
// the row.
func RequestLogFromEvent(e events.Event, defaultOrgID string, getPolicy func() *policy.Policy) (RequestLog, bool) {
	if !isRequestEvent(e.EventType) {
		return RequestLog{}, false
	}
	org := strings.TrimSpace(e.OrgID)
	if org == "" {
		org = strings.TrimSpace(defaultOrgID)
	}
	if org == "" {
		return RequestLog{}, false
	}
	// The table names organisations by UUID. Without a key the organisation
	// is only what the caller sent in a header, and a value that is not a
	// UUID names none: the row goes to the default organisation, like that
	// of a request that says nothing. Otherwise a caller could keep its
	// requests, the blocked ones included, out of the record by sending any
	// made-up value. A key's organisation is not replaced: if it is not a
	// UUID the row cannot be written, and the writer counts and logs that.
	if !e.OrgVerified {
		if _, err := uuid.Parse(org); err != nil {
			if def := strings.TrimSpace(defaultOrgID); def != "" {
				org = def
			}
		}
	}
	hash := false
	if getPolicy != nil {
		if pol := getPolicy(); pol != nil && pol.Data != nil && pol.Data.HashFindings {
			hash = true
		}
	}
	rl := RequestLog{
		RequestID:     e.RequestID,
		OrgID:         org,
		Provider:      e.Provider,
		Model:         e.Model,
		ModelFamily:   e.ModelFamily,
		InputTokens:   e.InputTokens,
		OutputTokens:  e.OutputTokens,
		Findings:      findingsJSON(e.Findings, hash),
		FindingsCount: len(e.Findings),
		// Lower case, as the column's default is. Rows written by older
		// versions hold either case, so the readers compare without it.
		ActionTaken:    strings.ToLower(strings.TrimSpace(e.Action)),
		ScanLatencyMs:  e.ScanLatencyMs,
		TotalLatencyMs: e.TotalLatencyMs,
		UserID:         e.UserID,
		Endpoint:       e.Endpoint,
		// The row is written some time after the request, in batches; it is
		// dated by the request.
		At:            e.Timestamp,
		CacheStatus:   strings.ToLower(strings.TrimSpace(e.CacheStatus)),
		OutputAction:  strings.ToLower(strings.TrimSpace(e.OutputAction)),
		PolicyName:    e.PolicyName,
		PolicyVersion: e.PolicyVersion,
	}
	if e.CostKnown {
		cost := e.CostUSD
		rl.CostUSD = &cost
	}
	if e.OutputScanned || len(e.OutputFindings) > 0 {
		rl.OutputFindings = findingsJSON(e.OutputFindings, hash)
	}
	if e.OutputScanned {
		if b, err := json.Marshal(e.OutputRisk); err == nil {
			rl.OutputRisk = b
		}
	}
	return rl, true
}

// RequestLogFromV2 builds the same row from the envelope the NATS bridge
// published for the event.
func RequestLogFromV2(ev events.EventV2, defaultOrgID string, getPolicy func() *policy.Policy) (RequestLog, bool) {
	e, ok := eventFromV2(ev)
	if !ok {
		return RequestLog{}, false
	}
	return RequestLogFromEvent(e, defaultOrgID, getPolicy)
}

// eventFromV2 reads back, from a NATS envelope, the fields of the event that
// a row is built from. It returns false when the envelope is not a request's.
func eventFromV2(ev events.EventV2) (events.Event, bool) {
	p := ev.Payload
	eventType := payloadString(p, "event_type")
	if eventType == "" {
		// An envelope published before the payload named its event. The
		// stream keeps a week of them. Its subject tells a blocked or a
		// redacted request; scan.completed was shared with events that are
		// not requests, which carry no action.
		switch ev.Type {
		case events.EventBlockTriggered:
			eventType = "request_blocked"
		case events.EventRedactApplied:
			eventType = "request_scanned"
		case events.EventScanCompleted:
			if payloadString(p, "action") != "" && ev.RequestID != "" {
				eventType = "request_scanned"
			}
		}
	}
	if !isRequestEvent(eventType) {
		return events.Event{}, false
	}
	e := events.Event{
		RequestID:      ev.RequestID,
		OrgID:          ev.OrgID,
		EventType:      eventType,
		Timestamp:      ev.Timestamp,
		Provider:       payloadString(p, "provider"),
		Model:          payloadString(p, "model"),
		ModelFamily:    payloadString(p, "model_family"),
		Action:         payloadString(p, "action"),
		Endpoint:       payloadString(p, "endpoint"),
		UserID:         payloadString(p, "user_id"),
		ScanLatencyMs:  payloadFloat(p, "scan_latency_ms"),
		TotalLatencyMs: payloadFloat(p, "total_latency_ms"),
		InputTokens:    int(payloadFloat(p, "input_tokens")),
		OutputTokens:   int(payloadFloat(p, "output_tokens")),
		CacheStatus:    payloadString(p, "cache_status"),
		OutputAction:   payloadString(p, "output_action"),
		PolicyName:     payloadString(p, "policy_name"),
		PolicyVersion:  payloadString(p, "policy_version"),
	}
	// Older envelopes always said cost_usd: 0. Only one that says the cost
	// is known is believed.
	e.OrgVerified, _ = p["org_verified"].(bool)
	if known, _ := p["cost_known"].(bool); known {
		e.CostKnown = true
		e.CostUSD = payloadFloat(p, "cost_usd")
	}
	payloadInto(p, "findings", &e.Findings)
	payloadInto(p, "output_findings", &e.OutputFindings)
	if scanned, _ := p["output_scanned"].(bool); scanned {
		e.OutputScanned = true
		payloadInto(p, "output_risk", &e.OutputRisk)
	}
	return e, true
}

func payloadString(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func payloadFloat(m map[string]any, key string) float64 {
	switch n := m[key].(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

// payloadInto decodes a nested value of the payload into dst. A value that
// is absent or does not fit leaves dst as it was.
func payloadInto(m map[string]any, key string, dst any) {
	raw, ok := m[key]
	if !ok || raw == nil {
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, dst)
}

// findingsJSON is the stored form of a list of findings: a JSON array, never
// null, with the matches hashed when asked.
func findingsJSON(findings []scanner.Finding, hash bool) []byte {
	if hash {
		findings = hashFindingMatches(findings)
	}
	if len(findings) == 0 {
		return []byte("[]")
	}
	b, err := json.Marshal(findings)
	if err != nil {
		return []byte("[]")
	}
	return b
}
