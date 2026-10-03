// Package users stores team member role assignments used for RBAC on the
// dashboard. The source-of-truth for membership (email, name, avatar) is
// Clerk; this store is merely the mapping of Clerk user_id -> local role.
package users

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/docstore"
)

// Role is the RBAC role assigned to a team member. It maps to scope in the
// admin auth middleware:
//
//	admin   -> full access (read + write)
//	analyst -> read + write (but no admin-only endpoints like /team)
//	viewer  -> read-only
const (
	RoleAdmin   = "admin"
	RoleAnalyst = "analyst"
	RoleViewer  = "viewer"
)

func IsValidRole(r string) bool {
	switch strings.ToLower(r) {
	case RoleAdmin, RoleAnalyst, RoleViewer:
		return true
	}
	return false
}

// Member is the dashboard-facing view of a team member. Name / email are
// fetched from Clerk at runtime and injected by the API handler; the store
// only persists (user_id, role, updated_at).
type Member struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email,omitempty"`
	Name      string    `json:"name,omitempty"`
	ImageURL  string    `json:"image_url,omitempty"`
	Role      string    `json:"role"`
	UpdatedAt time.Time `json:"updated_at"`
}

var ErrNotFound = errors.New("member not found")

type Store interface {
	Role(userID string) (string, bool)
	Set(userID, role string) (Member, error)
	List() []Member
	Delete(userID string)
}

type MemoryStore struct {
	// wmu serialises changes; mu guards the working copy that every
	// dashboard request reads.
	wmu     sync.Mutex
	mu      sync.RWMutex
	data    map[string]Member
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

// Reload replaces the working copy with what the backing storage holds.
func (s *MemoryStore) Reload() error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if s.persist == nil {
		return nil
	}
	fresh := make(map[string]Member)
	err := s.persist.All(func(id string, raw []byte) error {
		var m Member
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
		fresh[id] = m
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

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]Member)}
}

func (s *MemoryStore) Role(userID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[userID]
	if !ok {
		return "", false
	}
	return v.Role, true
}

func (s *MemoryStore) Set(userID, role string) (Member, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Member{}, errors.New("user_id required")
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if !IsValidRole(role) {
		return Member{}, errors.New("invalid role")
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	m := Member{UserID: userID, Role: role, UpdatedAt: time.Now().UTC()}
	s.mu.RLock()
	cur, ok := s.data[userID]
	s.mu.RUnlock()
	if ok {
		cur.Role = role
		cur.UpdatedAt = m.UpdatedAt
		m = cur
	}
	if s.persist != nil {
		// Only the assignment is stored; name and email come from Clerk.
		if err := s.persist.Put(userID, Member{UserID: m.UserID, Role: m.Role, UpdatedAt: m.UpdatedAt}); err != nil {
			return Member{}, err
		}
	}
	s.mu.Lock()
	s.data[userID] = m
	s.mu.Unlock()
	return m, nil
}

func (s *MemoryStore) Delete(userID string) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if s.persist != nil {
		if err := s.persist.Delete(userID); err != nil {
			// The role stays until the write goes through: removing it
			// here only would bring it back on the next reload.
			log.Warn().Err(err).Str("user_id", userID).Msg("team role: delete failed")
			return
		}
	}
	s.mu.Lock()
	delete(s.data, userID)
	s.mu.Unlock()
}

func (s *MemoryStore) List() []Member {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Member, 0, len(s.data))
	for _, v := range s.data {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}
