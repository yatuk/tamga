// Package apikeys implements Tamga's own keys: the scoped keys that open the
// management API and the virtual keys that identify a caller on the proxy
// path.
//
// A key is 192 random bits, so a plain SHA-256 is enough to store it: there
// is nothing to brute-force, and the check runs on every request. The raw
// value is returned once, at creation; only the hash and a short prefix for
// recognising the key are kept.
//
// A presented key is found by its hash, never by its id. The id names the
// record in audit entries, log lines and the console, so it is drawn
// separately and says nothing about the key.
package apikeys

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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

// idPrefix starts the id of every key created since ids stopped being taken
// from the key. An id without it belongs to an older key and is the first 12
// hex characters of that key's random part.
const idPrefix = "key_"

// prefixLen is how much of a key is kept for recognising it: "tk_" and five
// hex characters, 20 of the key's 192 bits. That is enough to match a key in
// hand to its row and leaves 172 bits unknown. It is shown in the key list
// only; audit entries and log lines carry the id.
const prefixLen = 8

// Key is the sanitized view of a stored API key record.
type Key struct {
	// ID is random and independent of the key. See idPrefix for older keys.
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
	ExpiresAt time.Time `json:"expires_at,omitzero"`
	LastUsed  time.Time `json:"last_used,omitzero"`
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

// sameHash compares two key hashes in constant time. The stores find a record
// by its hash through a map or an index, which is not constant time; this is
// the comparison that decides.
func sameHash(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// newID draws a record id. It shares no bytes with the key.
func newID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return idPrefix + hex.EncodeToString(buf), nil
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
	id, err := newID()
	if err != nil {
		return CreatedKey{}, "", err
	}
	meta := Key{
		ID:        id,
		Label:     strings.TrimSpace(p.Label),
		Scope:     scope,
		Prefix:    raw[:prefixLen],
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
	data   map[string]*memKey // by id
	byHash map[string]string  // hash -> id
}

type memKey struct {
	Key
	hash string
}

// NewMemoryStore creates an in-memory API key store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]*memKey), byHash: make(map[string]string)}
}

func (s *MemoryStore) List() []Key {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Key, 0, len(s.data))
	for _, k := range s.data {
		out = append(out, k.Key)
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, taken := s.data[ck.ID]; taken {
		return CreatedKey{}, errors.New("apikeys: id collision, try again")
	}
	s.data[ck.ID] = &memKey{Key: ck.Key, hash: hash}
	s.byHash[hash] = ck.ID
	return ck, nil
}

func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.data[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.data, id)
	delete(s.byHash, k.hash)
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
	if !ok || !sameHash(k.hash, h) || k.Expired(time.Now()) {
		return Key{}, false
	}
	k.LastUsed = time.Now().UTC()
	return k.Key, true
}
