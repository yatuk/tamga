// Package users stores team member role assignments used for RBAC on the
// dashboard. The source-of-truth for membership (email, name, avatar) is
// Clerk; this store is merely the mapping of Clerk user_id -> local role.
package users

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
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
	mu   sync.RWMutex
	data map[string]Member
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
	s.mu.Lock()
	defer s.mu.Unlock()
	m := Member{UserID: userID, Role: role, UpdatedAt: time.Now().UTC()}
	if cur, ok := s.data[userID]; ok {
		cur.Role = role
		cur.UpdatedAt = m.UpdatedAt
		s.data[userID] = cur
		return cur, nil
	}
	s.data[userID] = m
	return m, nil
}

func (s *MemoryStore) Delete(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, userID)
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
