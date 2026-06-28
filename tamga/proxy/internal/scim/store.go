// Package scim implements an in-memory SCIM v2.0 user store. It is used by
// the dashboard SCIM provisioning endpoints (GET/POST/PATCH/DELETE
// /api/v1/scim/v2/Users) for lightweight IdP integration.
package scim

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// SCIM v2.0 core schema URN.
const SchemaURN = "urn:ietf:params:scim:schemas:core:2.0:User"

// User represents a SCIM v2.0 User resource.
type User struct {
	Schemas  []string `json:"schemas"`
	ID       string   `json:"id"`
	UserName string   `json:"userName"`
	Name     struct {
		GivenName  string `json:"givenName"`
		FamilyName string `json:"familyName"`
	} `json:"name"`
	Emails []struct {
		Value   string `json:"value"`
		Primary bool   `json:"primary"`
	} `json:"emails"`
	Active bool   `json:"active"`
	Meta   struct {
		ResourceType string    `json:"resourceType"`
		Created      time.Time `json:"created"`
		LastModified time.Time `json:"lastModified"`
	} `json:"meta"`
}

// ListResponse is the standard SCIM list response.
type ListResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	Resources    []User   `json:"Resources"`
}

// ErrNotFound is returned when a user id is unknown.
var ErrNotFound = errors.New("user not found")

// Store describes the SCIM user provisioning operations.
type Store interface {
	List() []User
	Get(id string) (User, bool)
	Create(u User) (User, error)
	Update(id string, u User) (User, error)
	Patch(id string, patch map[string]interface{}) (User, error)
	Delete(id string) error
}

// MemoryStore is a thread-safe in-memory SCIM user store.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]User
}

// NewMemoryStore creates an in-memory SCIM user store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]User)}
}

// Validate normalises and validates a SCIM user payload.
func Validate(u User) (User, error) {
	u.UserName = strings.TrimSpace(u.UserName)
	if u.UserName == "" {
		return u, errors.New("userName required")
	}
	u.Name.GivenName = strings.TrimSpace(u.Name.GivenName)
	u.Name.FamilyName = strings.TrimSpace(u.Name.FamilyName)

	for i := range u.Emails {
		u.Emails[i].Value = strings.TrimSpace(u.Emails[i].Value)
		if i == 0 {
			u.Emails[i].Primary = true
		}
	}
	return u, nil
}

func newID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// List returns all SCIM users sorted by last modified (newest first).
func (s *MemoryStore) List() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]User, 0, len(s.data))
	for _, v := range s.data {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Meta.LastModified.After(out[j].Meta.LastModified)
	})
	return out
}

// Get returns a single SCIM user by id, or false if not found.
func (s *MemoryStore) Get(id string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[id]
	return v, ok
}

// Create inserts a new SCIM user after validation.
func (s *MemoryStore) Create(u User) (User, error) {
	normalized, err := Validate(u)
	if err != nil {
		return User{}, err
	}
	now := time.Now().UTC()
	normalized.ID = newID()
	normalized.Schemas = []string{SchemaURN}
	normalized.Active = true
	normalized.Meta.ResourceType = "User"
	normalized.Meta.Created = now
	normalized.Meta.LastModified = now
	s.mu.Lock()
	s.data[normalized.ID] = normalized
	s.mu.Unlock()
	return normalized, nil
}

// Update replaces an existing SCIM user wholesale.
func (s *MemoryStore) Update(id string, u User) (User, error) {
	normalized, err := Validate(u)
	if err != nil {
		return User{}, err
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.data[id]
	if !ok {
		return User{}, ErrNotFound
	}
	normalized.ID = cur.ID
	normalized.Schemas = []string{SchemaURN}
	normalized.Meta.ResourceType = "User"
	normalized.Meta.Created = cur.Meta.Created
	normalized.Meta.LastModified = now
	s.data[id] = normalized
	return normalized, nil
}

// Patch applies partial updates per SCIM PATCH (RFC 7644, section 3.5.2).
// Supports a simplified subset: Operations with op="replace" or op="add"
// that set top-level fields and the "active" flag.
func (s *MemoryStore) Patch(id string, patch map[string]interface{}) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.data[id]
	if !ok {
		return User{}, ErrNotFound
	}

	// Parse SCIM PATCH payload: { "schemas": [...], "Operations": [...] }
	opsRaw, ok := patch["Operations"]
	if !ok {
		return cur, nil
	}
	ops, ok := opsRaw.([]interface{})
	if !ok {
		return cur, nil
	}

	for _, raw := range ops {
		opMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		op, _ := opMap["op"].(string)
		path, _ := opMap["path"].(string)
		value := opMap["value"]

		switch strings.ToLower(op) {
		case "replace", "add":
			switch path {
			case "userName":
				if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
					cur.UserName = strings.TrimSpace(s)
				}
			case "name.givenName":
				if s, ok := value.(string); ok {
					cur.Name.GivenName = strings.TrimSpace(s)
				}
			case "name.familyName":
				if s, ok := value.(string); ok {
					cur.Name.FamilyName = strings.TrimSpace(s)
				}
			case "active":
				if b, ok := value.(bool); ok {
					cur.Active = b
				}
			default:
				// Unsupported path — silently ignored for simplicity.
			}
		case "remove":
			// For "active", remove sets it to false.
			if path == "active" {
				cur.Active = false
			}
		}
	}

	cur.Meta.LastModified = time.Now().UTC()
	s.data[id] = cur
	return cur, nil
}

// Delete removes a SCIM user by id.
func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return ErrNotFound
	}
	delete(s.data, id)
	return nil
}
