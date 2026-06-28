// Package apikeys implements an in-memory scoped API key store.
//
// Keys have a scope (read / write / admin) and a SHA-256 hash. Raw keys are
// only returned at creation time — clients must persist the value themselves.
package apikeys

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Scope values mirror the dashboard Settings > Access tabs.
const (
	ScopeRead  = "read"
	ScopeWrite = "write"
	ScopeAdmin = "admin"
)

// IsValidScope reports whether s is a recognised API key scope.
func IsValidScope(s string) bool {
	switch strings.ToLower(s) {
	case ScopeRead, ScopeWrite, ScopeAdmin:
		return true
	}
	return false
}

// Key is the sanitized view of a stored API key record.
type Key struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Scope     string    `json:"scope"`
	Prefix    string    `json:"prefix"`
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used,omitempty"`
}

// CreatedKey wraps the metadata and the one-time plaintext value.
type CreatedKey struct {
	Key
	RawKey string `json:"raw_key"`
}

// Store is the minimal interface used by the HTTP layer.
type Store interface {
	List() []Key
	Create(label, scope string) (CreatedKey, error)
	Delete(id string) error
	Verify(raw string) (Key, bool)
}

type record struct {
	meta Key
	hash string
}

// MemoryStore is a thread-safe, in-memory implementation.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]*record
}

// NewMemoryStore creates an in-memory API key store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]*record)}
}

// ErrNotFound is returned when Delete / Verify cannot find a record.
var ErrNotFound = errors.New("api key not found")

func hashKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (s *MemoryStore) List() []Key {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Key, 0, len(s.data))
	for _, r := range s.data {
		out = append(out, r.meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *MemoryStore) Create(label, scope string) (CreatedKey, error) {
	if !IsValidScope(scope) {
		return CreatedKey{}, fmt.Errorf("invalid scope %q", scope)
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return CreatedKey{}, err
	}
	raw := "tk_" + hex.EncodeToString(buf)
	id := hex.EncodeToString(buf[:6])
	prefix := raw[:8]
	meta := Key{
		ID:        id,
		Label:     strings.TrimSpace(label),
		Scope:     strings.ToLower(scope),
		Prefix:    prefix,
		CreatedAt: time.Now().UTC(),
	}
	rec := &record{meta: meta, hash: hashKey(raw)}
	s.mu.Lock()
	s.data[id] = rec
	s.mu.Unlock()
	return CreatedKey{Key: meta, RawKey: raw}, nil
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return ErrNotFound
	}
	delete(s.data, id)
	return nil
}

func (s *MemoryStore) Verify(raw string) (Key, bool) {
	if raw == "" {
		return Key{}, false
	}
	h := hashKey(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.data {
		if r.hash == h {
			r.meta.LastUsed = time.Now().UTC()
			return r.meta, true
		}
	}
	return Key{}, false
}
