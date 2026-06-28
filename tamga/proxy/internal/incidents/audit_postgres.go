package incidents

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dbConn is a minimal subset of *pgxpool.Pool needed by AuditPersister.
type dbConn interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// AuditPersister is a Postgres-backed companion to AuditRing. The ring
// stays authoritative for fast in-memory Verify() calls, but every
// Append() is also written to the audit_log table so the chain
// survives restarts and is visible from any replica.
type AuditPersister struct {
	pool dbConn
	mu   sync.Mutex
}

// NewAuditPersister opens no new connections; it reuses the pool passed in.
// Pass nil to create a persister that short-circuits all writes
// (useful for testing).
func NewAuditPersister(pool *pgxpool.Pool) *AuditPersister {
	ap := &AuditPersister{}
	if pool != nil {
		ap.pool = pool
	}
	return ap
}

// Append writes a single audit entry. Errors are returned but callers
// typically log-and-continue so that audit-log outages never block a
// security event from being recorded in the ring buffer.
func (p *AuditPersister) Append(ctx context.Context, e AuditEntry) error {
	if p == nil || p.pool == nil {
		return nil
	}
	detail, err := json.Marshal(e.Detail)
	if err != nil {
		detail = []byte("{}")
	}
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = p.pool.Exec(ctx2, `
INSERT INTO audit_log (ts, actor, kind, target, detail, prev_hash, hash)
VALUES ($1, NULLIF($2,''), $3, NULLIF($4,''), $5::jsonb, NULLIF($6,''), $7)`,
		e.Timestamp, e.Actor, e.Kind, e.Target, string(detail), e.PrevHash, e.Hash)
	return err
}

// Load hydrates the ring buffer from the DB. Called once at startup so
// dashboards show historical entries even after a proxy restart.
func (p *AuditPersister) Load(ctx context.Context, limit int) ([]AuditEntry, error) {
	if p == nil || p.pool == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 512
	}
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, err := p.pool.Query(ctx2, `
SELECT ts, COALESCE(actor,''), kind, COALESCE(target,''), detail,
       COALESCE(prev_hash,''), hash
FROM audit_log
ORDER BY id DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var detailRaw []byte
		if err := rows.Scan(&e.Timestamp, &e.Actor, &e.Kind, &e.Target, &detailRaw, &e.PrevHash, &e.Hash); err != nil {
			return nil, err
		}
		if len(detailRaw) > 0 {
			var m map[string]interface{}
			if err := json.Unmarshal(detailRaw, &m); err == nil {
				e.Detail = m
			}
		}
		out = append(out, e)
	}
	// Reverse so oldest-first, matching AuditRing.Append semantics.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

// PersistAll writes a slice in order; used for migrations.
func (p *AuditPersister) PersistAll(ctx context.Context, entries []AuditEntry) error {
	if p == nil || p.pool == nil || len(entries) == 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range entries {
		if err := p.Append(ctx, e); err != nil {
			return fmt.Errorf("persist audit entry: %w", err)
		}
	}
	return nil
}
