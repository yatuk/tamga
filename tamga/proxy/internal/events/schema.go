package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ── Versioned Event Schema for NATS JetStream ──────────────────────────────

// EventType categorises events for NATS subject routing.
type EventType string

const (
	EventScanCompleted   EventType = "scan.completed"
	EventBlockTriggered  EventType = "block.triggered"
	EventRedactApplied   EventType = "redact.applied"
	EventPolicyViolation EventType = "policy.violation"
	EventOutputScanned   EventType = "output.scanned"
)

// EventV2 is the versioned, self-contained event envelope for NATS JetStream.
// The legacy Event struct (bus.go) remains for the in-memory fast path used
// by dashboard SSE, metrics, and logging.
type EventV2 struct {
	ID        string         `json:"id"`
	Type      EventType      `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	RequestID string         `json:"request_id"`
	OrgID     string         `json:"org_id"`
	Payload   map[string]any `json:"payload"`
	Metadata  EventMetadata  `json:"metadata"`
}

// EventMetadata carries tracing and source information.
type EventMetadata struct {
	Source  string `json:"source"`  // "tamga-proxy"
	Version string `json:"version"` // "2.0"
	TraceID string `json:"trace_id"`
}

// eventToV2 converts a legacy Event to the versioned EventV2 envelope.
func eventToV2(e Event) EventV2 {
	payload := map[string]any{
		"provider":         e.Provider,
		"model":            e.Model,
		"model_family":     e.ModelFamily,
		"action":           e.Action,
		"endpoint":         e.Endpoint,
		"scan_latency_ms":  e.ScanLatencyMs,
		"total_latency_ms": e.TotalLatencyMs,
		"input_tokens":     e.InputTokens,
		"output_tokens":    e.OutputTokens,
		"cost_usd":         e.CostUSD,
		"cache_status":     e.CacheStatus,
		"user_id":          e.UserID,
		"findings_count":   len(e.Findings),
		"input_risk_level": e.InputRisk.Level,
		"input_risk_score": e.InputRisk.Score,
	}
	// Embed findings as JSON — downstream consumers can deserialise if needed.
	if len(e.Findings) > 0 {
		if fj, err := json.Marshal(e.Findings); err == nil {
			payload["findings"] = json.RawMessage(fj)
		}
	}
	if len(e.OutputFindings) > 0 {
		if fj, err := json.Marshal(e.OutputFindings); err == nil {
			payload["output_findings"] = json.RawMessage(fj)
		}
	}

	var traceID string
	if e.TraceContext != nil {
		traceID = e.TraceContext["traceparent"]
	}

	return EventV2{
		ID:        uuid.NewString(),
		Type:      subjectToEventType(e.EventType, e.Action),
		Timestamp: e.Timestamp,
		RequestID: e.RequestID,
		OrgID:     e.OrgID,
		Payload:   payload,
		Metadata: EventMetadata{
			Source:  "tamga-proxy",
			Version: "2.0",
			TraceID: traceID,
		},
	}
}

// subjectForEvent maps a legacy event type + action to a NATS subject.
func subjectForEvent(eventType, action string) string {
	switch {
	case eventType == "request_blocked":
		return "block.triggered"
	case eventType == "request_scanned" && action == "REDACT":
		return "redact.applied"
	case eventType == "output_scanned":
		return "output.scanned"
	default:
		return "scan.completed"
	}
}

func subjectToEventType(eventType, action string) EventType {
	switch {
	case eventType == "request_blocked":
		return EventBlockTriggered
	case eventType == "request_scanned" && action == "REDACT":
		return EventRedactApplied
	case eventType == "output_scanned":
		return EventOutputScanned
	default:
		return EventScanCompleted
	}
}
