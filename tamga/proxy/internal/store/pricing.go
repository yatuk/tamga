package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PricingQuerier is the interface for model pricing lookups.
// Both the concrete PricingStore and test mocks implement this.
type PricingQuerier interface {
	ListActive(ctx context.Context) ([]ModelPricing, error)
	Lookup(ctx context.Context, provider, family, version string, effectiveAt time.Time) (*ModelPricing, error)
}

// ModelPricing represents one row in the model_pricing table.
type ModelPricing struct {
	ID            int        `json:"id"`
	Provider      string     `json:"provider"`
	ModelFamily   string     `json:"model_family"`
	ModelVersion  string     `json:"model_version"`
	InputPer1K    float64    `json:"input_per_1k"`
	OutputPer1K   float64    `json:"output_per_1k"`
	Currency      string     `json:"currency"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to,omitempty"`
	Source        string     `json:"source"`
	Notes         *string    `json:"notes,omitempty"`
}

// PricingStore queries the model_pricing table.
type PricingStore struct {
	pool *pgxpool.Pool
}

// NewPricingStore creates a pricing lookup store.
func NewPricingStore(pool *pgxpool.Pool) *PricingStore {
	return &PricingStore{pool: pool}
}

// ListActive returns all currently active pricing entries.
func (s *PricingStore) ListActive(ctx context.Context) ([]ModelPricing, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, provider, model_family, model_version,
		       input_per_1k, output_per_1k, currency,
		       effective_from, effective_to, source, notes
		FROM model_pricing
		WHERE effective_to IS NULL
		ORDER BY provider, model_family, model_version
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ModelPricing
	for rows.Next() {
		var p ModelPricing
		if err := rows.Scan(
			&p.ID, &p.Provider, &p.ModelFamily, &p.ModelVersion,
			&p.InputPer1K, &p.OutputPer1K, &p.Currency,
			&p.EffectiveFrom, &p.EffectiveTo, &p.Source, &p.Notes,
		); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// Lookup returns pricing for a specific model at a given time.
// Returns nil, nil if no pricing found (unknown model).
func (s *PricingStore) Lookup(ctx context.Context, provider, family, version string, effectiveAt time.Time) (*ModelPricing, error) {
	var p ModelPricing
	err := s.pool.QueryRow(ctx, `
		SELECT id, provider, model_family, model_version,
		       input_per_1k, output_per_1k, currency,
		       effective_from, effective_to, source, notes
		FROM model_pricing
		WHERE provider = $1 AND model_family = $2 AND model_version = $3
		  AND effective_from <= $4
		  AND (effective_to IS NULL OR effective_to > $4)
		ORDER BY effective_from DESC
		LIMIT 1
	`, provider, family, version, effectiveAt).Scan(
		&p.ID, &p.Provider, &p.ModelFamily, &p.ModelVersion,
		&p.InputPer1K, &p.OutputPer1K, &p.Currency,
		&p.EffectiveFrom, &p.EffectiveTo, &p.Source, &p.Notes,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}
