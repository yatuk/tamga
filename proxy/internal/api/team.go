package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/users"
)

func (cfg Config) handleTeamList(w http.ResponseWriter, r *http.Request) {
	if cfg.Users == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"items": []users.Member{}, "clerk": false})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stored := cfg.Users.List()
	roleByID := make(map[string]users.Member, len(stored))
	for _, m := range stored {
		roleByID[m.UserID] = m
	}

	items := make([]users.Member, 0, len(stored))
	clerkOK := false
	if cfg.Clerk != nil {
		clerkUsers, err := cfg.Clerk.ListUsers(ctx, 200)
		if err == nil {
			clerkOK = true
			seen := map[string]bool{}
			for _, cu := range clerkUsers {
				m := users.Member{
					UserID:   cu.ID,
					Email:    cu.Email(),
					Name:     cu.Name(),
					ImageURL: cu.ImageURL,
					Role:     users.RoleViewer,
				}
				if existing, ok := roleByID[cu.ID]; ok {
					m.Role = existing.Role
					m.UpdatedAt = existing.UpdatedAt
				}
				items = append(items, m)
				seen[cu.ID] = true
			}
			for _, m := range stored {
				if !seen[m.UserID] {
					items = append(items, m)
				}
			}
		}
	}
	if !clerkOK {
		items = stored
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"total": len(items),
		"clerk": clerkOK,
	})
}

const (
	codeOwnRole   = "own_role"
	codeLastAdmin = "last_admin"
)

// validUserID reports whether id can be the id of a user: what an identity
// provider issues (a GitHub number, a Clerk "user_..." id, a SCIM id or an
// email-like subject), and nothing that is a path or a control character.
func validUserID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.ContainsRune("_-.@|:+", c):
		default:
			return false
		}
	}
	return true
}

func adminCount(members []users.Member) int {
	n := 0
	for _, m := range members {
		if m.Role == users.RoleAdmin {
			n++
		}
	}
	return n
}

func (cfg Config) handleTeamRolePut(w http.ResponseWriter, r *http.Request) {
	if cfg.Users == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "users store unavailable"})
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if !validUserID(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}
	// Nobody decides their own role: an admin who could would also be the
	// one to undo a demotion.
	if actorFromRequest(r) == actorUserPrefix+id {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "you cannot change your own role",
			"code":  codeOwnRole,
		})
		return
	}
	defer func() { _ = r.Body.Close() }()
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	// The last admin stays one: without it only the static admin key could
	// assign roles again.
	if cur, ok := cfg.Users.Role(id); ok && cur == users.RoleAdmin &&
		strings.ToLower(strings.TrimSpace(body.Role)) != users.RoleAdmin && adminCount(cfg.Users.List()) <= 1 {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "this is the last admin; make someone else an admin first",
			"code":  codeLastAdmin,
		})
		return
	}
	m, err := cfg.Users.Set(id, body.Role)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "team.role",
			Target: m.UserID,
			Actor:  actorFromRequest(r),
			Detail: map[string]interface{}{"role": m.Role},
		})
	}
	writeJSON(w, http.StatusOK, m)
}
