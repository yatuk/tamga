package api

import (
	"context"
	"encoding/json"
	"errors"
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
					// Being in the Clerk tenant assigns nothing: the role is
					// empty, or the operator's default for accounts without
					// one.
					Role: cfg.defaultRole(),
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
		// The role of an account nobody assigned one: "" (it has no access)
		// or "viewer".
		"default_role": cfg.defaultRole(),
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
	// assign roles again. The store checks that and writes in one step, so
	// two demotions at once cannot both find another admin still there.
	// A role that changes ends the user's sessions (see users.MemoryStore).
	m, err := cfg.Users.Assign(id, body.Role)
	if err != nil {
		if errors.Is(err, users.ErrLastAdmin) {
			writeLastAdmin(w)
			return
		}
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

func writeLastAdmin(w http.ResponseWriter) {
	writeJSON(w, http.StatusConflict, map[string]string{
		"error": "this is the last admin; make someone else an admin first",
		"code":  codeLastAdmin,
	})
}

// teamTarget returns the user id a team route names, having answered the
// request when there is no user store or the id cannot be one.
func (cfg Config) teamTarget(w http.ResponseWriter, r *http.Request) (string, bool) {
	if cfg.Users == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "users store unavailable"})
		return "", false
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if !validUserID(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return "", false
	}
	return id, true
}

// handleTeamRoleDelete serves DELETE /team/{id}/role: the account keeps no
// role. Its sessions end, and it cannot sign in again unless the operator
// set a default role for accounts without one.
func (cfg Config) handleTeamRoleDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := cfg.teamTarget(w, r)
	if !ok {
		return
	}
	// As with a change of role: nobody removes their own.
	if actorFromRequest(r) == actorUserPrefix+id {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "you cannot remove your own role",
			"code":  codeOwnRole,
		})
		return
	}
	if err := cfg.Users.Remove(id); err != nil {
		switch {
		case errors.Is(err, users.ErrLastAdmin):
			writeLastAdmin(w)
		case errors.Is(err, users.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "this account has no role"})
		default:
			writeStoreError(w, err)
		}
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "team.role.remove",
			Target: id,
			Actor:  actorFromRequest(r),
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "user_id": id})
}

// handleTeamSessionsRevoke serves POST /team/{id}/sessions/revoke: every
// session token the user holds now stops working, and the user signs in
// again. The role is not touched. An admin may do this to their own
// sessions; the request that asks for it is the last one its token makes.
//
// The replica that serves this refuses the tokens from the next request on;
// another replica does once it has reloaded the team roles, up to 30 s
// later (main.settingsReloadInterval).
func (cfg Config) handleTeamSessionsRevoke(w http.ResponseWriter, r *http.Request) {
	id, ok := cfg.teamTarget(w, r)
	if !ok {
		return
	}
	from, err := cfg.Users.RevokeSessions(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "team.sessions.revoke",
			Target: id,
			Actor:  actorFromRequest(r),
			Detail: map[string]interface{}{"sessions_valid_from": from},
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                  true,
		"user_id":             id,
		"sessions_valid_from": from,
	})
}
