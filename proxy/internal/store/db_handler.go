package store

import (
	"context"
	"sync"

	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// DBHandler writes request_scanned and request_blocked events to Store (runs on the event bus worker goroutine).
// When getPolicy is non-nil and the active policy has Data.HashFindings enabled, finding matches
// are replaced by a keyed digest before being persisted (KVKK/GDPR data protection).
func DBHandler(log zerolog.Logger, s Store, defaultOrgID string, getPolicy func() *policy.Policy) func(events.Event) {
	if s == nil {
		return func(events.Event) {}
	}
	var warnUnkeyed sync.Once
	return func(e events.Event) {
		before := UnkeyedMatches()
		rl, ok := RequestLogFromEvent(e, defaultOrgID, getPolicy)
		if !ok {
			return
		}
		if UnkeyedMatches() != before {
			warnUnkeyed.Do(func() { LogUnkeyedMatches(log) })
		}
		if err := s.SaveRequestLog(context.Background(), rl); err != nil {
			log.Warn().Err(err).Str("component", "db_handler").Str("request_id", e.RequestID).Msg("save request log")
		}
	}
}

// hashFindingMatches returns a deep copy of findings with each Match field
// replaced by its keyed digest (see FindingHasher). Without a key the match
// is replaced by UnkeyedMatch: the row keeps the finding's type, category
// and severity and nothing derived from the value.
func hashFindingMatches(findings []scanner.Finding) []scanner.Finding {
	h := CurrentFindingHasher()
	out := make([]scanner.Finding, len(findings))
	for i, f := range findings {
		out[i] = f
		if f.Match == "" {
			continue
		}
		if h == nil {
			out[i].Match = UnkeyedMatch
			unkeyedMatches.Add(1)
			continue
		}
		out[i].Match = h.Hash(f.Match)
	}
	return out
}

// LogUnkeyedMatches says, at error level, that data.hash_findings is on and
// there is no key to hash with. The writers call it once per process.
func LogUnkeyedMatches(log zerolog.Logger) {
	log.Error().Str("component", "request_log").
		Msg("data.hash_findings is on and no finding hash key is set: matches are stored as \"" + UnkeyedMatch +
			"\" and cannot be looked up; set " + FindingHashKeyEnv + " or TAMGA_VAULT_KEY")
}
