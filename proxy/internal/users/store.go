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
	// SessionsValidFrom is the unix second up to which this user's session
	// tokens are revoked. It moves when the role changes or is removed, on
	// an admin's request and when the user signs out.
	SessionsValidFrom int64 `json:"sessions_valid_from,omitempty"`
}

var (
	ErrNotFound = errors.New("member not found")
	// ErrLastAdmin refuses the change that would leave no admin.
	ErrLastAdmin = errors.New("this is the last admin")
)

type Store interface {
	Role(userID string) (string, bool)
	Set(userID, role string) (Member, error)
	List() []Member
	Delete(userID string)

	// Assign is Set for a request: it refuses, with ErrLastAdmin, to take
	// the admin role from the only admin. The check and the write are one
	// step.
	Assign(userID, role string) (Member, error)
	// Remove takes a member's role away, under the same rule.
	Remove(userID string) error
	// SessionsValidFrom returns the unix second up to which a user's
	// session tokens are revoked: one issued at or before it is refused.
	// Zero when nothing was revoked.
	SessionsValidFrom(userID string) int64
	// RevokeSessions revokes every session token a user holds now and
	// returns the new SessionsValidFrom. The user need not have a role.
	RevokeSessions(userID string) (int64, error)
}

// MemoryStore keeps the roles in memory and, with Persist, in the database
// behind it.
//
// A member whose role was removed is kept as a record without a role, so
// that the moment its sessions were revoked outlives the role: assigning a
// role again must not bring a token from before back to life. Role and List
// do not show such a record.
//
// Across replicas a change, a revocation included, is seen by the others at
// their next reload (main.settingsReloadInterval, 30 s). Until then another
// replica answers with the role and the revocation time it last read.
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
	return s.reloadLocked()
}

// reloadLocked is Reload for a caller that holds wmu.
func (s *MemoryStore) reloadLocked() error {
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
	if !ok || v.Role == "" {
		return "", false
	}
	return v.Role, true
}

func (s *MemoryStore) SessionsValidFrom(userID string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[userID].SessionsValidFrom
}

// revokedNow returns the revocation time that covers every token issued up
// to now, the ones dated just after an earlier revocation included (see
// api.Config.issueSession).
func revokedNow(m Member, now time.Time) int64 {
	t := now.Unix()
	if m.SessionsValidFrom >= t {
		t = m.SessionsValidFrom + 1
	}
	return t
}

// put writes m to the backing storage and then to the working copy. The
// caller holds wmu.
func (s *MemoryStore) put(m Member) error {
	if s.persist != nil {
		// Only the assignment is stored; name and email come from Clerk.
		stored := Member{UserID: m.UserID, Role: m.Role, UpdatedAt: m.UpdatedAt, SessionsValidFrom: m.SessionsValidFrom}
		if err := s.persist.Put(m.UserID, stored); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.data[m.UserID] = m
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) get(userID string) (Member, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.data[userID]
	return m, ok
}

// beginChange starts a change: with backing storage it reads the storage
// first, so that the change is made to what the other replicas last wrote
// and not to a copy up to a reload interval old. Otherwise a role assigned
// here could write back a revocation time from before another replica's
// revocation, and the last-admin check could count an admin another replica
// has demoted. The caller holds wmu.
//
// Two replicas that change the same record, or each demote a different
// admin, at the same instant can still both go through: the storage has no
// compare-and-set.
func (s *MemoryStore) beginChange() error {
	return s.reloadLocked()
}

// lastAdmin reports whether userID holds the only admin role. The caller
// holds wmu and has called beginChange.
func (s *MemoryStore) lastAdmin(userID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data[userID].Role != RoleAdmin {
		return false
	}
	for id, m := range s.data {
		if id != userID && m.Role == RoleAdmin {
			return false
		}
	}
	return true
}

// Set assigns a role. A role that differs from the one before revokes the
// user's sessions: what a token was issued for is no longer what the user
// may do.
func (s *MemoryStore) Set(userID, role string) (Member, error) {
	return s.assign(userID, role, false)
}

func (s *MemoryStore) Assign(userID, role string) (Member, error) {
	return s.assign(userID, role, true)
}

func (s *MemoryStore) assign(userID, role string, keepAnAdmin bool) (Member, error) {
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
	if err := s.beginChange(); err != nil {
		return Member{}, err
	}
	if keepAnAdmin && role != RoleAdmin && s.lastAdmin(userID) {
		return Member{}, ErrLastAdmin
	}
	now := time.Now().UTC()
	m, _ := s.get(userID)
	if m.Role != role {
		m.SessionsValidFrom = revokedNow(m, now)
	}
	m.UserID, m.Role, m.UpdatedAt = userID, role, now
	if err := s.put(m); err != nil {
		return Member{}, err
	}
	return m, nil
}

// Delete removes a member's role. A failed write is logged and the role
// stays; Remove reports it.
func (s *MemoryStore) Delete(userID string) {
	if err := s.remove(userID, false); err != nil && !errors.Is(err, ErrNotFound) {
		// The role stays until the write goes through: removing it here
		// only would bring it back on the next reload.
		log.Warn().Err(err).Str("user_id", userID).Msg("team role: delete failed")
	}
}

func (s *MemoryStore) Remove(userID string) error {
	return s.remove(userID, true)
}

func (s *MemoryStore) remove(userID string, keepAnAdmin bool) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if err := s.beginChange(); err != nil {
		return err
	}
	if keepAnAdmin && s.lastAdmin(userID) {
		return ErrLastAdmin
	}
	m, ok := s.get(userID)
	if !ok || m.Role == "" {
		return ErrNotFound
	}
	now := time.Now().UTC()
	m.Role = ""
	m.SessionsValidFrom = revokedNow(m, now)
	m.UpdatedAt = now
	return s.put(m)
}

func (s *MemoryStore) RevokeSessions(userID string) (int64, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, errors.New("user_id required")
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if err := s.beginChange(); err != nil {
		return 0, err
	}
	m, ok := s.get(userID)
	if !ok {
		m = Member{UserID: userID, UpdatedAt: time.Now().UTC()}
	}
	m.SessionsValidFrom = revokedNow(m, time.Now())
	if err := s.put(m); err != nil {
		return 0, err
	}
	return m.SessionsValidFrom, nil
}

func (s *MemoryStore) List() []Member {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Member, 0, len(s.data))
	for _, v := range s.data {
		// A record kept for its revocation time only is not a member.
		if v.Role == "" {
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}
