// Package docstore keeps the small settings stores (custom patterns, team
// roles, webhooks) in PostgreSQL so they survive a restart and agree across
// replicas.
//
// Those stores are read on hot paths and hold a few dozen records, so each
// keeps its working copy in memory. docstore is what stands behind it: a
// change is written here before it is applied in memory, the copy is loaded
// from here at startup, and reloaded periodically to pick up changes another
// replica made.
package docstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Schema is also shipped as migration 016; creating it at startup keeps an
// existing database from needing a manual step.
const Schema = `
CREATE TABLE IF NOT EXISTS stored_documents (
    kind       TEXT NOT NULL,
    id         TEXT NOT NULL,
    doc        TEXT NOT NULL,
    encrypted  BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, id)
);
`

const queryTimeout = 3 * time.Second

// ErrUnavailable wraps every failure to reach the database, so a caller can
// tell "could not store it" from "what you asked for is invalid".
var ErrUnavailable = errors.New("settings storage unavailable")

// Persister is what a memory store needs from its backing storage.
type Persister interface {
	// Put stores doc, JSON-encoded, under id.
	Put(id string, doc any) error
	Delete(id string) error
	// All calls each with every stored record.
	All(each func(id string, raw []byte) error) error
}

// Sealer encrypts a document at rest. vault.Cipher is one.
type Sealer interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(blob string) (string, error)
}

// Init creates the table.
func Init(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("docstore: no database pool")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, Schema); err != nil {
		return fmt.Errorf("docstore: create table: %w", err)
	}
	return nil
}

// Table is the records of one kind.
type Table struct {
	pool *pgxpool.Pool
	kind string
	seal Sealer
}

var _ Persister = (*Table)(nil)

// NewTable returns the table for kind. With a sealer every document is
// encrypted before it is written.
func NewTable(pool *pgxpool.Pool, kind string, seal Sealer) *Table {
	return &Table{pool: pool, kind: kind, seal: seal}
}

func (t *Table) Put(id string, doc any) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("docstore: encode %s/%s: %w", t.kind, id, err)
	}
	body := string(raw)
	if t.seal != nil {
		if body, err = t.seal.Encrypt(body); err != nil {
			return fmt.Errorf("docstore: encrypt %s/%s: %w", t.kind, id, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	_, err = t.pool.Exec(ctx, `
		INSERT INTO stored_documents (kind, id, doc, encrypted, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (kind, id) DO UPDATE SET doc = EXCLUDED.doc, encrypted = EXCLUDED.encrypted, updated_at = now()`,
		t.kind, id, body, t.seal != nil)
	if err != nil {
		return fmt.Errorf("%w: write %s/%s: %v", ErrUnavailable, t.kind, id, err)
	}
	return nil
}

func (t *Table) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	if _, err := t.pool.Exec(ctx, `DELETE FROM stored_documents WHERE kind = $1 AND id = $2`, t.kind, id); err != nil {
		return fmt.Errorf("%w: delete %s/%s: %v", ErrUnavailable, t.kind, id, err)
	}
	return nil
}

func (t *Table) All(each func(id string, raw []byte) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	rows, err := t.pool.Query(ctx, `SELECT id, doc, encrypted FROM stored_documents WHERE kind = $1`, t.kind)
	if err != nil {
		return fmt.Errorf("%w: read %s: %v", ErrUnavailable, t.kind, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, body string
		var encrypted bool
		if err := rows.Scan(&id, &body, &encrypted); err != nil {
			return fmt.Errorf("docstore: read %s: %w", t.kind, err)
		}
		if encrypted {
			if t.seal == nil {
				return fmt.Errorf("docstore: %s/%s is encrypted and no key is configured", t.kind, id)
			}
			if body, err = t.seal.Decrypt(body); err != nil {
				return fmt.Errorf("docstore: decrypt %s/%s (was the key changed?): %w", t.kind, id, err)
			}
		}
		if err := each(id, []byte(body)); err != nil {
			return err
		}
	}
	return rows.Err()
}
