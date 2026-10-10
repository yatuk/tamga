package store

import (
	"context"
	"encoding/json"
	"time"
)

// RequestLog maps to the request_logs table (see deploy/migrations/001_init.sql).
type RequestLog struct {
	RequestID      string
	OrgID          string
	Provider       string
	Model          string
	ModelFamily    string // coarse family for grouping (e.g. "claude-4", "gpt-4o")
	InputTokens    int
	OutputTokens   int
	Findings       []byte // JSON
	FindingsCount  int
	ActionTaken    string
	ScanLatencyMs  float64
	TotalLatencyMs float64
	UserID         string
	Endpoint       string
	// At is when the request happened. Zero means now, at the time of the insert.
	// A row with the organisation, request id and time of one already stored
	// is the same event delivered again, and is not stored twice.
	At time.Time
	// InputTokens and OutputTokens, above, are what the provider reported.
	// Both zero means the counts are not known, and the row then holds NULL:
	// a real count is never zero on both sides.
	//
	// CostUSD is what the request was charged; nil when no cost is known.
	CostUSD *float64
	// CacheStatus is what the response cache did: "hit", "miss", "bypass",
	// or empty when the cache is off.
	CacheStatus string
	// OutputFindings (JSON), OutputAction and OutputRisk (JSON) describe the
	// scan of the response. OutputFindings and OutputRisk are nil when the
	// response was not scanned; OutputAction is empty when nothing was
	// decided about it.
	OutputFindings []byte
	OutputAction   string
	OutputRisk     []byte
	// PolicyName and PolicyVersion name the policy the request was decided with.
	PolicyName    string
	PolicyVersion string
}

// SecurityEvent is a row from request_logs for dashboard listing.
type SecurityEvent struct {
	RequestID     string          `json:"request_id"`
	Provider      string          `json:"provider,omitempty"`
	Model         string          `json:"model,omitempty"`
	ActionTaken   string          `json:"action_taken"`
	Findings      json.RawMessage `json:"findings"`
	FindingsCount int             `json:"findings_count"`
	CreatedAt     time.Time       `json:"created_at"`
}

// EventSearchParams filters request_logs for GET /api/v1/events. Empty
// string fields are ignored (no filter). ShadowOnly means providers that
// are not in the enterprise list (see internal/providers).
type EventSearchParams struct {
	Page, Limit int
	Action      string
	Provider    string // lowercase; "shadow" = non-enterprise providers only
	ShadowOnly  bool   // if true, Provider is ignored
	FindingType string
	Severity    string
	Category    string // substring match on finding category
	Technique   string // substring match on findings JSON (OWASP codes, metadata)
	Q           string // request_id ILIKE or findings text
	RequestID   string // substring of the request id only (exports)
	Since       time.Time
	Until       time.Time
}

// ModelTokenUsage aggregates input/output tokens per provider+model from
// request_logs for cost breakdown computation.
type ModelTokenUsage struct {
	Provider     string
	Model        string
	ModelFamily  string
	InputTokens  int64
	OutputTokens int64
}

// DailyTokenUsage is a per-day token usage entry for billing breakdowns.
type DailyTokenUsage struct {
	Date         time.Time `json:"date"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	ModelFamily  string    `json:"model_family"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
}

// Store persists request telemetry (PostgreSQL or no-op when disabled).
type Store interface {
	SaveRequestLog(ctx context.Context, log RequestLog) error
	ListSecurityEvents(ctx context.Context, orgID string, page, limit int) ([]SecurityEvent, int, error)
	SearchSecurityEvents(ctx context.Context, orgID string, p EventSearchParams) ([]SecurityEvent, int, error)
	// GetModelTokenUsage returns token sums grouped by provider+model for a time window.
	GetModelTokenUsage(ctx context.Context, orgID string, from, to time.Time) ([]ModelTokenUsage, error)
	// GetDailyTokenUsage returns daily token sums grouped by date+provider+model.
	GetDailyTokenUsage(ctx context.Context, orgID string, from, to time.Time) ([]DailyTokenUsage, error)
	Ping(ctx context.Context) error
	Close() error
}
