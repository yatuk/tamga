package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// DBHandler writes request_scanned and request_blocked events to Store (runs on the event bus worker goroutine).
// When getPolicy is non-nil and the active policy has Data.HashFindings enabled, finding matches
// are hashed with SHA-256 before being persisted (KVKK/GDPR data protection).
func DBHandler(log zerolog.Logger, s Store, defaultOrgID string, getPolicy func() *policy.Policy) func(events.Event) {
	if s == nil {
		return func(events.Event) {}
	}
	return func(e events.Event) {
		if e.EventType != "request_scanned" && e.EventType != "request_blocked" {
			return
		}
		org := strings.TrimSpace(e.OrgID)
		if org == "" {
			org = strings.TrimSpace(defaultOrgID)
		}
		if org == "" {
			return
		}
		// Hash finding matches when data protection is enabled (KVKK/GDPR).
		// Hashing is irreversible — finding content cannot be recovered from the log.
		findings := e.Findings
		if getPolicy != nil {
			if pol := getPolicy(); pol != nil && pol.Data != nil && pol.Data.HashFindings {
				findings = hashFindingMatches(findings)
			}
		}
		findingsJSON, err := json.Marshal(findings)
		if err != nil {
			log.Warn().Err(err).Str("component", "db_handler").Str("request_id", e.RequestID).Msg("marshal findings for request log")
			findingsJSON = []byte("[]")
		}
		rl := RequestLog{
			RequestID:      e.RequestID,
			OrgID:          org,
			Provider:       e.Provider,
			Model:          e.Model,
			ModelFamily:    e.ModelFamily,
			InputTokens:    e.InputTokens,
			OutputTokens:   e.OutputTokens,
			Findings:       findingsJSON,
			FindingsCount:  len(e.Findings),
			ActionTaken:    strings.ToLower(strings.TrimSpace(e.Action)),
			ScanLatencyMs:  e.ScanLatencyMs,
			TotalLatencyMs: e.TotalLatencyMs,
			UserID:         e.UserID,
			Endpoint:       e.Endpoint,
		}
		if err := s.SaveRequestLog(context.Background(), rl); err != nil {
			log.Warn().Err(err).Str("component", "db_handler").Str("request_id", e.RequestID).Msg("save request log")
		}
	}
}

// hashFindingMatches returns a deep copy of findings with each Match field
// replaced by its SHA-256 hex digest, prefixed with "sha256:".
func hashFindingMatches(findings []scanner.Finding) []scanner.Finding {
	out := make([]scanner.Finding, len(findings))
	for i, f := range findings {
		out[i] = f
		if f.Match != "" {
			h := sha256.Sum256([]byte(f.Match))
			out[i].Match = "sha256:" + hex.EncodeToString(h[:])
		}
	}
	return out
}
