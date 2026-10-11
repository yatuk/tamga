package store

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
)

// DBHandler writes request_scanned and request_blocked events to Store (runs on the event bus worker goroutine).
// When getPolicy is non-nil and the active policy has Data.HashFindings enabled, finding matches
// are replaced by a keyed digest before being persisted (KVKK/GDPR data protection). An event the
// bus already hashed (see EventFindingHasher) is stored as it is.
func DBHandler(log zerolog.Logger, s Store, defaultOrgID string, getPolicy func() *policy.Policy) func(events.Event) {
	if s == nil {
		return func(events.Event) {}
	}
	return func(e events.Event) {
		before := UnkeyedMatches()
		rl, ok := RequestLogFromEvent(e, defaultOrgID, getPolicy)
		if !ok {
			return
		}
		if UnkeyedMatches() != before {
			LogUnkeyedMatches(log)
		}
		if err := s.SaveRequestLog(context.Background(), rl); err != nil {
			log.Warn().Err(err).Str("component", "db_handler").Str("request_id", e.RequestID).Msg("save request log")
		}
	}
}
