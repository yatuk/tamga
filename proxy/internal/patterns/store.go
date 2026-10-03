// Package patterns implements the store for user-defined custom detection
// patterns (regex or literal). The working copy is in memory, because the
// custom scanner reads it on every request; with a Persister attached it is
// loaded from and written through to the database. The dashboard exposes these as a
// distinct surface from built-in scanners so analysts can iterate on
// organization-specific terms without editing policy YAML.
package patterns

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yatuk/tamga/internal/docstore"
)

const (
	KindRegex   = "regex"
	KindLiteral = "literal"
)

// Severity values are the same set the policy engine understands.
const (
	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

// Pattern is a user-defined detection rule.
type Pattern struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Pattern   string    `json:"pattern"`
	Severity  string    `json:"severity"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ErrNotFound is returned when the pattern id is unknown.
var ErrNotFound = errors.New("pattern not found")

// Store describes the HTTP-facing operations.
type Store interface {
	List() []Pattern
	Get(id string) (Pattern, bool)
	Create(p Pattern) (Pattern, error)
	Update(id string, p Pattern) (Pattern, error)
	Delete(id string) error
}

// MemoryStore is a thread-safe in-memory implementation.
type MemoryStore struct {
	// wmu serialises changes, so a slow write to the database never holds
	// the lock the scanner reads under.
	wmu     sync.Mutex
	mu      sync.RWMutex
	data    map[string]Pattern
	persist docstore.Persister
}

// Persist attaches backing storage and loads what it holds. From then on a
// change is written there before it is applied here.
func (s *MemoryStore) Persist(p docstore.Persister) error {
	s.wmu.Lock()
	s.persist = p
	s.wmu.Unlock()
	return s.Reload()
}

// Reload replaces the working copy with what the backing storage holds,
// which is how a change made on another replica arrives.
func (s *MemoryStore) Reload() error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if s.persist == nil {
		return nil
	}
	fresh := make(map[string]Pattern)
	err := s.persist.All(func(id string, raw []byte) error {
		var p Pattern
		if err := json.Unmarshal(raw, &p); err != nil {
			return err
		}
		fresh[id] = p
		return nil
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.data = fresh
	s.mu.Unlock()
	return nil
}

// NewMemoryStore creates an in-memory pattern store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]Pattern)}
}

func validateKind(k string) (string, error) {
	k = strings.ToLower(strings.TrimSpace(k))
	switch k {
	case KindRegex, KindLiteral:
		return k, nil
	}
	return "", errors.New("kind must be regex or literal")
}

func validateSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
		return strings.ToLower(s)
	}
	return SeverityMedium
}

// Validate checks pattern integrity and compiles the regex if applicable.
// Returns the normalized pattern ready for storage.
func Validate(p Pattern) (Pattern, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return p, errors.New("name required")
	}
	p.Pattern = strings.TrimSpace(p.Pattern)
	if p.Pattern == "" {
		return p, errors.New("pattern required")
	}
	kind, err := validateKind(p.Kind)
	if err != nil {
		return p, err
	}
	p.Kind = kind
	p.Severity = validateSeverity(p.Severity)
	if kind == KindRegex {
		if _, err := regexp.Compile(p.Pattern); err != nil {
			return p, errors.New("invalid regex: " + err.Error())
		}
	}
	return p, nil
}

func newID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func (s *MemoryStore) List() []Pattern {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Pattern, 0, len(s.data))
	for _, v := range s.data {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (s *MemoryStore) Get(id string) (Pattern, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[id]
	return v, ok
}

func (s *MemoryStore) Create(p Pattern) (Pattern, error) {
	normalized, err := Validate(p)
	if err != nil {
		return Pattern{}, err
	}
	now := time.Now().UTC()
	normalized.ID = newID()
	normalized.CreatedAt = now
	normalized.UpdatedAt = now
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if s.persist != nil {
		if err := s.persist.Put(normalized.ID, normalized); err != nil {
			return Pattern{}, err
		}
	}
	s.mu.Lock()
	s.data[normalized.ID] = normalized
	s.mu.Unlock()
	return normalized, nil
}

func (s *MemoryStore) Update(id string, p Pattern) (Pattern, error) {
	normalized, err := Validate(p)
	if err != nil {
		return Pattern{}, err
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	cur, ok := s.Get(id)
	if !ok {
		return Pattern{}, ErrNotFound
	}
	normalized.ID = cur.ID
	normalized.CreatedAt = cur.CreatedAt
	normalized.UpdatedAt = time.Now().UTC()
	if s.persist != nil {
		if err := s.persist.Put(id, normalized); err != nil {
			return Pattern{}, err
		}
	}
	s.mu.Lock()
	s.data[id] = normalized
	s.mu.Unlock()
	return normalized, nil
}

func (s *MemoryStore) Delete(id string) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if _, ok := s.Get(id); !ok {
		return ErrNotFound
	}
	if s.persist != nil {
		if err := s.persist.Delete(id); err != nil {
			return err
		}
	}
	s.mu.Lock()
	delete(s.data, id)
	s.mu.Unlock()
	return nil
}
