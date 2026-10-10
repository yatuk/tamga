package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

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
		rl, ok := RequestLogFromEvent(e, defaultOrgID, getPolicy)
		if !ok {
			return
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
