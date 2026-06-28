package operator_state

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// RedisStore persists decision and note state to Redis via the existing
// redisx.Client interface (Get/Set/Del only, JSON-serialized values).
// When Redis is unavailable, operations are no-ops — the in-memory
// projection serves as the authoritative fallback.
type RedisStore struct {
	client    RedisClient
	keyPrefix string
	ttl       time.Duration
}

// RedisClient is the subset of redisx.Client used by the operator-state store.
// Uses the same interface pattern as the rest of the proxy (events, budget, cache).
type RedisClient interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

// NewRedisStore creates a Redis-backed state store. If client is nil or
// not enabled, all operations are no-ops.
func NewRedisStore(client RedisClient) *RedisStore {
	if client == nil {
		return &RedisStore{} // no-op store
	}
	return &RedisStore{
		client:    client,
		keyPrefix: "tamga:opstate",
		ttl:       0, // no expiry — audit log is the source of truth
	}
}

// decisionKey builds the Redis key for a decision record.
func (s *RedisStore) decisionKey(id string) string {
	return fmt.Sprintf("%s:decision:%s", s.keyPrefix, id)
}

// noteKey builds the Redis key for a note record.
func (s *RedisStore) noteKey(id string) string {
	return fmt.Sprintf("%s:note:%s", s.keyPrefix, id)
}

// IsEnabled reports whether Redis is available.
func (s *RedisStore) IsEnabled() bool {
	return s.client != nil
}

// GetDecision retrieves a decision record from Redis. Returns nil if not found.
func (s *RedisStore) GetDecision(ctx context.Context, id string) *DecisionRecord {
	if s.client == nil {
		return nil
	}

	data, found, err := s.client.Get(ctx, s.decisionKey(id))
	if err != nil {
		log.Warn().Err(err).Str("decision_id", id).Msg("opstate: redis GET failed, falling back to memory")
		return nil
	}
	if !found {
		return nil
	}

	var rec DecisionRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		log.Warn().Err(err).Str("decision_id", id).Msg("opstate: redis deserialization failed")
		return nil
	}
	return &rec
}

// SetDecision persists a decision record to Redis (write-through).
func (s *RedisStore) SetDecision(ctx context.Context, rec *DecisionRecord) {
	if s.client == nil {
		return
	}

	data, err := json.Marshal(rec)
	if err != nil {
		log.Warn().Err(err).Str("decision_id", rec.ID).Msg("opstate: redis serialization failed")
		return
	}

	if err := s.client.Set(ctx, s.decisionKey(rec.ID), data, s.ttl); err != nil {
		log.Warn().Err(err).Str("decision_id", rec.ID).Msg("opstate: redis SET failed")
	}
}

// GetNote retrieves a note record from Redis. Returns nil if not found.
func (s *RedisStore) GetNote(ctx context.Context, id string) *NoteRecord {
	if s.client == nil {
		return nil
	}

	data, found, err := s.client.Get(ctx, s.noteKey(id))
	if err != nil {
		log.Warn().Err(err).Str("note_id", id).Msg("opstate: redis GET failed, falling back to memory")
		return nil
	}
	if !found {
		return nil
	}

	var rec NoteRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		log.Warn().Err(err).Str("note_id", id).Msg("opstate: redis deserialization failed")
		return nil
	}
	return &rec
}

// SetNote persists a note record to Redis (write-through).
func (s *RedisStore) SetNote(ctx context.Context, rec *NoteRecord) {
	if s.client == nil {
		return
	}

	data, err := json.Marshal(rec)
	if err != nil {
		log.Warn().Err(err).Str("note_id", rec.ID).Msg("opstate: redis serialization failed")
		return
	}

	if err := s.client.Set(ctx, s.noteKey(rec.ID), data, s.ttl); err != nil {
		log.Warn().Err(err).Str("note_id", rec.ID).Msg("opstate: redis SET failed")
	}
}

// DeleteDecision removes a decision from Redis.
func (s *RedisStore) DeleteDecision(ctx context.Context, id string) {
	if s.client == nil {
		return
	}
	if err := s.client.Del(ctx, s.decisionKey(id)); err != nil {
		log.Warn().Err(err).Str("decision_id", id).Msg("opstate: redis DEL failed")
	}
}

// SeedFromProjection bulk-writes all in-memory state to Redis. Called after
// initial replay to populate Redis for the first time.
func (s *RedisStore) SeedFromProjection(ctx context.Context, p *Projection) {
	if s.client == nil {
		return
	}

	snap := p.Snapshot()
	for _, rec := range snap.Decisions {
		s.SetDecision(ctx, rec)
	}
	for _, rec := range snap.Notes {
		s.SetNote(ctx, rec)
	}
	log.Info().
		Int("decisions", len(snap.Decisions)).
		Int("notes", len(snap.Notes)).
		Msg("opstate: redis seeded from projection")
}
