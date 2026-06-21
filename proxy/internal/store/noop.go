package store

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// NoopStore implements Store when no database is configured.
type NoopStore struct{}

// NewNoopStore returns a no-op store and logs once that DB logging is disabled.
func NewNoopStore(log zerolog.Logger) *NoopStore {
	log.Info().Str("component", "noop_store").Msg("database not configured, request logging disabled")
	return &NoopStore{}
}

// NewNoopStoreSilent is a no-op store without startup log (e.g. after Postgres connection failure).
func NewNoopStoreSilent() *NoopStore {
	return &NoopStore{}
}

func (n *NoopStore) SaveRequestLog(_ context.Context, _ RequestLog) error {
	return nil
}

func (n *NoopStore) GetStats(_ context.Context, _ string, _, _ time.Time) (*Stats, error) {
	return &Stats{}, nil
}

func (n *NoopStore) ListSecurityEvents(_ context.Context, _ string, _, _ int) ([]SecurityEvent, int, error) {
	return nil, 0, nil
}

func (n *NoopStore) SearchSecurityEvents(_ context.Context, _ string, _ EventSearchParams) ([]SecurityEvent, int, error) {
	return nil, 0, nil
}

func (n *NoopStore) GetModelTokenUsage(_ context.Context, _ string, _, _ time.Time) ([]ModelTokenUsage, error) {
	return nil, nil
}

func (n *NoopStore) GetDailyTokenUsage(_ context.Context, _ string, _, _ time.Time) ([]DailyTokenUsage, error) {
	return nil, nil
}

func (n *NoopStore) Ping(_ context.Context) error {
	return nil
}

func (n *NoopStore) Close() error {
	return nil
}
