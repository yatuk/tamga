package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SavedHunt is a named threat-hunting query saved by a user.
type SavedHunt struct {
	ID        string          `json:"id"`
	OrgID     string          `json:"org_id"`
	Name      string          `json:"name"`
	Query     json.RawMessage `json:"query"`
	CreatedBy string          `json:"created_by,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// ErrSavedHuntNotFound is returned when a saved hunt does not exist.
var ErrSavedHuntNotFound = errors.New("saved hunt not found")

// SavedHuntStore persists threat-hunting queries with org-scoped isolation.
type SavedHuntStore interface {
	List(ctx context.Context, orgID string) ([]SavedHunt, error)
	Create(ctx context.Context, hunt *SavedHunt) error
	Update(ctx context.Context, hunt *SavedHunt) error
	Delete(ctx context.Context, orgID, id string) error
}

// savedHuntPostgres is the PostgreSQL implementation of SavedHuntStore.
type savedHuntPostgres struct {
	pool *pgxpool.Pool
}

// compile-time interface check
var _ SavedHuntStore = (*savedHuntPostgres)(nil)

// NewSavedHuntStore creates a PostgreSQL-backed SavedHuntStore.
// Returns nil when pool is nil.
func NewSavedHuntStore(pool *pgxpool.Pool) SavedHuntStore {
	if pool == nil {
		return nil
	}
	return &savedHuntPostgres{pool: pool}
}

func (s *savedHuntPostgres) List(ctx context.Context, orgID string) ([]SavedHunt, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, name, query_json, COALESCE(created_by, ''), created_at, updated_at
		FROM saved_hunts
		WHERE org_id = $1
		ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list saved hunts: %w", err)
	}
	defer rows.Close()

	var out []SavedHunt
	for rows.Next() {
		var h SavedHunt
		if err := rows.Scan(&h.ID, &h.OrgID, &h.Name, &h.Query, &h.CreatedBy, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan saved hunt: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows saved hunts: %w", err)
	}
	return out, nil
}

func (s *savedHuntPostgres) Create(ctx context.Context, hunt *SavedHunt) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if hunt.Query == nil {
		hunt.Query = json.RawMessage("{}")
	}
	if hunt.CreatedAt.IsZero() {
		hunt.CreatedAt = time.Now().UTC()
	}
	if hunt.UpdatedAt.IsZero() {
		hunt.UpdatedAt = hunt.CreatedAt
	}

	err := s.pool.QueryRow(ctx, `
		INSERT INTO saved_hunts (org_id, name, query_json, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)
		RETURNING id`,
		hunt.OrgID, hunt.Name, hunt.Query, hunt.CreatedBy, hunt.CreatedAt, hunt.UpdatedAt,
	).Scan(&hunt.ID)
	if err != nil {
		return fmt.Errorf("create saved hunt: %w", err)
	}
	return nil
}

func (s *savedHuntPostgres) Update(ctx context.Context, hunt *SavedHunt) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if hunt.Query == nil {
		hunt.Query = json.RawMessage("{}")
	}
	hunt.UpdatedAt = time.Now().UTC()

	tag, err := s.pool.Exec(ctx, `
		UPDATE saved_hunts
		SET name = $1, query_json = $2, updated_at = $3
		WHERE id = $4 AND org_id = $5`,
		hunt.Name, hunt.Query, hunt.UpdatedAt, hunt.ID, hunt.OrgID)
	if err != nil {
		return fmt.Errorf("update saved hunt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSavedHuntNotFound
	}
	return nil
}

func (s *savedHuntPostgres) Delete(ctx context.Context, orgID, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tag, err := s.pool.Exec(ctx, `
		DELETE FROM saved_hunts
		WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return fmt.Errorf("delete saved hunt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSavedHuntNotFound
	}
	return nil
}

// scanSavedHunt reads a SavedHunt from a pgx.Row (used by queryRow pattern).
func scanSavedHunt(row pgx.Row) (SavedHunt, error) {
	var h SavedHunt
	if err := row.Scan(&h.ID, &h.OrgID, &h.Name, &h.Query, &h.CreatedBy, &h.CreatedAt, &h.UpdatedAt); err != nil {
		return SavedHunt{}, fmt.Errorf("scan saved hunt: %w", err)
	}
	return h, nil
}
