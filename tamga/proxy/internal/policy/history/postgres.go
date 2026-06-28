package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dbPool is the subset of *pgxpool.Pool methods used by PostgresStore.
// It enables mock-based testing without pulling in a real database.
type dbPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// PostgresStore persists revisions + proposals in Postgres so the chain
// survives restarts and is shared across replicas. It satisfies the
// `Store` interface so callers can swap it in without touching API code.
type PostgresStore struct {
	pool dbPool
}

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, fmt.Errorf("postgres pool required")
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) AppendRevision(rev Revision) (Revision, error) {
	if rev.ID == "" {
		rev.ID = uuid.Must(uuid.NewV7()).String()
	}
	if rev.Timestamp.IsZero() {
		rev.Timestamp = time.Now().UTC()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Look up the most-recent revision so we can set parent_id automatically.
	var parentID sql.NullString
	if err := s.pool.QueryRow(ctx,
		`SELECT id::text FROM policy_revisions ORDER BY ts DESC LIMIT 1`,
	).Scan(&parentID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Revision{}, err
	}
	if parentID.Valid {
		rev.ParentID = parentID.String
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO policy_revisions (id, ts, author, message, yaml, parent_id)
VALUES ($1, $2, $3, NULLIF($4,''), $5, NULLIF($6,'')::uuid)`,
		rev.ID, rev.Timestamp, rev.Author, rev.Message, rev.YAML, rev.ParentID)
	if err != nil {
		return Revision{}, fmt.Errorf("insert revision: %w", err)
	}
	return rev, nil
}

func (s *PostgresStore) ListRevisions() ([]Revision, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
SELECT id::text, ts, author, COALESCE(message,''), yaml, COALESCE(parent_id::text,'')
FROM policy_revisions
ORDER BY ts DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Revision
	for rows.Next() {
		var r Revision
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.Author, &r.Message, &r.YAML, &r.ParentID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetRevision(id string) (Revision, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var r Revision
	err := s.pool.QueryRow(ctx, `
SELECT id::text, ts, author, COALESCE(message,''), yaml, COALESCE(parent_id::text,'')
FROM policy_revisions WHERE id = $1::uuid`, id,
	).Scan(&r.ID, &r.Timestamp, &r.Author, &r.Message, &r.YAML, &r.ParentID)
	if err != nil {
		return Revision{}, false
	}
	return r, true
}

func (s *PostgresStore) CreateProposal(p Proposal) (Proposal, error) {
	if p.ID == "" {
		p.ID = uuid.Must(uuid.NewV7()).String()
	}
	if p.Timestamp.IsZero() {
		p.Timestamp = time.Now().UTC()
	}
	if p.Status == "" {
		p.Status = "open"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
INSERT INTO policy_proposals (id, ts, author, message, yaml, status)
VALUES ($1, $2, $3, NULLIF($4,''), $5, $6)`,
		p.ID, p.Timestamp, p.Author, p.Message, p.YAML, p.Status)
	if err != nil {
		return Proposal{}, err
	}
	return p, nil
}

func (s *PostgresStore) ListProposals() ([]Proposal, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `
SELECT id::text, ts, author, COALESCE(message,''), yaml, status,
       COALESCE(approved_by,''), COALESCE(approved_at, 'epoch'::timestamptz),
       COALESCE(rejected_by,''), COALESCE(rejected_at, 'epoch'::timestamptz)
FROM policy_proposals
ORDER BY ts DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Proposal
	for rows.Next() {
		var p Proposal
		if err := rows.Scan(&p.ID, &p.Timestamp, &p.Author, &p.Message, &p.YAML, &p.Status,
			&p.ApprovedBy, &p.ApprovedAt, &p.RejectedBy, &p.RejectedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetProposal(id string) (Proposal, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var p Proposal
	err := s.pool.QueryRow(ctx, `
SELECT id::text, ts, author, COALESCE(message,''), yaml, status,
       COALESCE(approved_by,''), COALESCE(approved_at, 'epoch'::timestamptz),
       COALESCE(rejected_by,''), COALESCE(rejected_at, 'epoch'::timestamptz)
FROM policy_proposals WHERE id = $1::uuid`, id,
	).Scan(&p.ID, &p.Timestamp, &p.Author, &p.Message, &p.YAML, &p.Status,
		&p.ApprovedBy, &p.ApprovedAt, &p.RejectedBy, &p.RejectedAt)
	if err != nil {
		return Proposal{}, false
	}
	return p, true
}

func (s *PostgresStore) UpdateProposal(p Proposal) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd, err := s.pool.Exec(ctx, `
UPDATE policy_proposals
SET status       = $2,
    approved_by  = NULLIF($3,''),
    approved_at  = CASE WHEN $3 <> '' THEN $4 ELSE NULL END,
    rejected_by  = NULLIF($5,''),
    rejected_at  = CASE WHEN $5 <> '' THEN $6 ELSE NULL END
WHERE id = $1::uuid`,
		p.ID, p.Status, p.ApprovedBy, p.ApprovedAt, p.RejectedBy, p.RejectedAt)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("proposal not found: %s", p.ID)
	}
	return nil
}
