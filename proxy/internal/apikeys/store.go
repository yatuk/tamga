// Package apikeys implements Tamga's own keys: the scoped keys that open the
// management API and the virtual keys that identify a caller on the proxy
// path.
//
// A key is 192 random bits, so a plain SHA-256 is enough to store it: there
// is nothing to brute-force, and the check runs on every request. The raw
// value is returned once, at creation; only the hash and a short prefix for
// recognising the key are kept.
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

// Scope values. read / write / admin mirror the dashboard Settings > Access
// tabs and open the management API. proxy identifies a caller on the proxy
// path and opens nothing else, so an application's key leaking does not
// expose the events it produced.
const (
	ScopeRead  = "read"
	ScopeWrite = "write"
	ScopeAdmin = "admin"
	ScopeProxy = "proxy"
)

// IsValidScope reports whether s is a recognised API key scope.
func IsValidScope(s string) bool {
	switch strings.ToLower(s) {
	case ScopeRead, ScopeWrite, ScopeAdmin, ScopeProxy:
		return true
	}
	return false
}

// Key is the sanitized view of a stored API key record.
type Key struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Scope  string `json:"scope"`
	Prefix string `json:"prefix"`
	// OrgID, Role and UserID are the identity a request made with this key
	// has on the proxy path. Empty means the deployment default.
	OrgID     string    `json:"org_id,omitempty"`
	Role      string    `json:"role,omitempty"`
	UserID    string    `json:"user_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	LastUsed  time.Time `json:"last_used,omitempty"`
}

// Expired reports whether the key has an expiry and it has passed.
func (k Key) Expired(now time.Time) bool {
	return !k.ExpiresAt.IsZero() && !now.Before(k.ExpiresAt)
}

// CreatedKey wraps the metadata and the one-time plaintext value.
type CreatedKey struct {
	Key
	RawKey string `json:"raw_key"`
}

// Params describes a key to create.
type Params struct {
	Label  string
	Scope  string
	OrgID  string
	Role   string
	UserID string
	// ExpiresAt is optional; the zero value means the key does not expire.
	ExpiresAt time.Time
}

// Store is the minimal interface used by the HTTP layer.
type Store interface {
	List() []Key
	Create(label, scope string) (CreatedKey, error)
	CreateWith(p Params) (CreatedKey, error)
	// Delete revokes the key. A revoked key no longer verifies and is no
	// longer listed.
	Delete(id string) error
	Verify(raw string) (Key, bool)
}

// ErrNotFound is returned when Delete / Verify cannot find a record.
var ErrNotFound = errors.New("api key not found")

func hashKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// newKey draws a key and fills in the record for it.
func newKey(p Params) (CreatedKey, string, error) {
	scope := strings.ToLower(strings.TrimSpace(p.Scope))
	if !IsValidScope(scope) {
		return CreatedKey{}, "", fmt.Errorf("invalid scope %q", p.Scope)
	}
	if !p.ExpiresAt.IsZero() && !p.ExpiresAt.After(time.Now()) {
		return CreatedKey{}, "", errors.New("expires_at is in the past")
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return CreatedKey{}, "", err
	}
	raw := "tk_" + hex.EncodeToString(buf)
	meta := Key{
		ID:        hex.EncodeToString(buf[:6]),
		Label:     strings.TrimSpace(p.Label),
		Scope:     scope,
		Prefix:    raw[:8],
		OrgID:     strings.TrimSpace(p.OrgID),
		Role:      strings.TrimSpace(p.Role),
		UserID:    strings.TrimSpace(p.UserID),
		CreatedAt: time.Now().UTC(),
		ExpiresAt: p.ExpiresAt.UTC(),
	}
	if p.ExpiresAt.IsZero() {
		meta.ExpiresAt = time.Time{}
	}
	return CreatedKey{Key: meta, RawKey: raw}, hashKey(raw), nil
}

// MemoryStore is a thread-safe, in-memory implementation. Keys do not
// survive a restart; it is what runs when no database is configured.
type MemoryStore struct {
	mu     sync.RWMutex
	data   map[string]*Key   // by id
	byHash map[string]string // hash -> id
}

// NewMemoryStore creates an in-memory API key store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]*Key), byHash: make(map[string]string)}
}

func (s *MemoryStore) List() []Key {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Key, 0, len(s.data))
	for _, k := range s.data {
		out = append(out, *k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *MemoryStore) Create(label, scope string) (CreatedKey, error) {
	return s.CreateWith(Params{Label: label, Scope: scope})
}

func (s *MemoryStore) CreateWith(p Params) (CreatedKey, error) {
	ck, hash, err := newKey(p)
	if err != nil {
		return CreatedKey{}, err
	}
	meta := ck.Key
	s.mu.Lock()
	s.data[meta.ID] = &meta
	s.byHash[hash] = meta.ID
	s.mu.Unlock()
	return ck, nil
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return ErrNotFound
	}
	delete(s.data, id)
	for h, kid := range s.byHash {
		if kid == id {
			delete(s.byHash, h)
		}
	}
	return nil
}

func (s *MemoryStore) Verify(raw string) (Key, bool) {
	if raw == "" {
		return Key{}, false
	}
	h := hashKey(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.data[s.byHash[h]]
	if !ok || k.Expired(time.Now()) {
		return Key{}, false
	}
	k.LastUsed = time.Now().UTC()
	return *k, true
}
