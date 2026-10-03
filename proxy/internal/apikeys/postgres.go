package apikeys

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

const (
	// verifyTTL is how long a verified key is served from memory. It is also
	// the longest a revoked or expired-by-edit key keeps working on a replica
	// that did not perform the revocation.
	verifyTTL = 30 * time.Second
	// missTTL keeps an unknown key from costing a query per request.
	missTTL = 5 * time.Second
	// staleFor is how long a previously verified key is still accepted while
	// the database cannot be reached. Without it a database blip would turn
	// every request away.
	staleFor = 5 * time.Minute
	// maxCached bounds the memory a flood of made-up keys can take.
	maxCached = 10000

	queryTimeout = 3 * time.Second
)

// Schema is the table the store uses. It is also shipped as migration 015;
// creating it here keeps an existing database, whose init scripts already
// ran, from needing a manual step.
const Schema = `
CREATE TABLE IF NOT EXISTS virtual_keys (
    id           TEXT PRIMARY KEY,
    key_hash     TEXT NOT NULL UNIQUE,
    prefix       TEXT NOT NULL,
    label        TEXT NOT NULL DEFAULT '',
    scope        TEXT NOT NULL,
    org_id       TEXT NOT NULL DEFAULT '',
    role         TEXT NOT NULL DEFAULT '',
    user_id      TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_virtual_keys_active ON virtual_keys(created_at DESC) WHERE revoked_at IS NULL;
`

type cached struct {
	key  Key
	ok   bool
	seen time.Time
}

// PostgresStore keeps keys in the database, so they survive a restart and
// are the same on every replica.
type PostgresStore struct {
	pool *pgxpool.Pool

	mu    sync.Mutex
	cache map[string]cached // by key hash
	now   func() time.Time
}

var _ Store = (*PostgresStore)(nil)

// NewPostgresStore creates the table if needed and returns the store.
func NewPostgresStore(ctx context.Context, pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("apikeys: no database pool")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, Schema); err != nil {
		return nil, fmt.Errorf("apikeys: create table: %w", err)
	}
	return &PostgresStore{pool: pool, cache: make(map[string]cached), now: time.Now}, nil
}

const keyColumns = `id, label, scope, prefix, org_id, role, user_id, created_at, expires_at, last_used_at`

func scanKey(row pgx.Row) (Key, error) {
	var k Key
	var expires, lastUsed *time.Time
	if err := row.Scan(&k.ID, &k.Label, &k.Scope, &k.Prefix, &k.OrgID, &k.Role, &k.UserID, &k.CreatedAt, &expires, &lastUsed); err != nil {
		return Key{}, err
	}
	if expires != nil {
		k.ExpiresAt = expires.UTC()
	}
	if lastUsed != nil {
		k.LastUsed = lastUsed.UTC()
	}
	k.CreatedAt = k.CreatedAt.UTC()
	return k, nil
}

func (s *PostgresStore) List() []Key {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx, `SELECT `+keyColumns+` FROM virtual_keys WHERE revoked_at IS NULL ORDER BY created_at DESC`)
	if err != nil {
		log.Warn().Err(err).Msg("api keys: list failed")
		return []Key{}
	}
	defer rows.Close()
	out := []Key{}
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			log.Warn().Err(err).Msg("api keys: list failed")
			return out
		}
		out = append(out, k)
	}
	return out
}

func (s *PostgresStore) Create(label, scope string) (CreatedKey, error) {
	return s.CreateWith(Params{Label: label, Scope: scope})
}

func (s *PostgresStore) CreateWith(p Params) (CreatedKey, error) {
	ck, hash, err := newKey(p)
	if err != nil {
		return CreatedKey{}, err
	}
	var expires *time.Time
	if !ck.ExpiresAt.IsZero() {
		expires = &ck.ExpiresAt
	}
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO virtual_keys (id, key_hash, prefix, label, scope, org_id, role, user_id, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		ck.ID, hash, ck.Prefix, ck.Label, ck.Scope, ck.OrgID, ck.Role, ck.UserID, ck.CreatedAt, expires)
	if err != nil {
		return CreatedKey{}, fmt.Errorf("apikeys: create: %w", err)
	}
	return ck, nil
}

func (s *PostgresStore) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	tag, err := s.pool.Exec(ctx, `UPDATE virtual_keys SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("apikeys: revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	// Takes effect at once on this replica; others follow within verifyTTL.
	s.mu.Lock()
	for h, c := range s.cache {
		if c.key.ID == id {
			delete(s.cache, h)
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *PostgresStore) Verify(raw string) (Key, bool) {
	if raw == "" {
		return Key{}, false
	}
	hash := hashKey(raw)
	now := s.now()

	s.mu.Lock()
	c, hit := s.cache[hash]
	s.mu.Unlock()
	if hit {
		ttl := missTTL
		if c.ok {
			ttl = verifyTTL
		}
		if now.Sub(c.seen) < ttl {
			return c.key, c.ok && !c.key.Expired(now)
		}
	}

	// The lookup also records the use, so last_used_at costs no extra query
	// and is at most verifyTTL behind.
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	k, err := scanKey(s.pool.QueryRow(ctx, `
		UPDATE virtual_keys SET last_used_at = now()
		WHERE key_hash = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())
		RETURNING `+keyColumns, hash))
	switch {
	case err == nil:
		s.remember(hash, cached{key: k, ok: true, seen: now})
		return k, true
	case errors.Is(err, pgx.ErrNoRows):
		s.remember(hash, cached{seen: now})
		return Key{}, false
	}
	log.Warn().Err(err).Msg("api keys: lookup failed")
	if hit && c.ok && now.Sub(c.seen) < staleFor && !c.key.Expired(now) {
		return c.key, true
	}
	return Key{}, false
}

func (s *PostgresStore) remember(hash string, c cached) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cache) >= maxCached {
		// Misses go first: they are what a flood of invented keys leaves.
		for h, old := range s.cache {
			if !old.ok {
				delete(s.cache, h)
			}
		}
		if len(s.cache) >= maxCached {
			s.cache = make(map[string]cached)
		}
	}
	s.cache[hash] = c
}
