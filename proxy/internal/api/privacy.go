package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/store"
)

// handleSubjectEraseImpl deletes request_log rows that match the supplied
// subject identifier (user_id, email, or hashed TCKN). When Postgres is not
// configured we return a placeholder so callers can wire UI without a DB.
// Every call that names a subject is written to the audit log: see
// auditSubject.
func handleSubjectEraseImpl(cfg Config, w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var body struct {
		UserID   string `json:"user_id"`
		Email    string `json:"email"`
		TCKNHash string `json:"tckn_hash"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
	}
	if body.UserID == "" && body.Email == "" && body.TCKNHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id, email, or tckn_hash required"})
		return
	}
	selector := subjectType(body.UserID, body.Email, body.TCKNHash)
	if selector == selectorTCKNHash && !validTCKNHash(body.TCKNHash) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errTCKNHashFormat})
		return
	}
	deleted := 0
	result := subjectStoreUnavailable
	if cfg.Store != nil && cfg.DefaultOrgID != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if eraser, ok := cfg.Store.(subjectEraser); ok {
			var err error
			deleted, err = eraser.EraseSubject(ctx, cfg.DefaultOrgID, body.UserID, body.Email, body.TCKNHash)
			if err != nil {
				log.Error().Err(err).Str("org_id", cfg.DefaultOrgID).Str("subject_type", selector).Msg("subject erase failed")
				cfg.auditSubject(r, auditSubjectErase, selector, "deleted", 0, subjectError)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "subject erase failed"})
				return
			}
			result = subjectOK
		}
	}
	cfg.auditSubject(r, auditSubjectErase, selector, "deleted", deleted, result)
	out := map[string]interface{}{
		"ok":      true,
		"deleted": deleted,
	}
	if result == subjectStoreUnavailable {
		out["note"] = "store not available; subject erase requires Postgres"
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSubjectAccess returns request_log rows for a data subject (GDPR Art. 15).
//
// The selector names a person. POST takes it from a JSON body, the fields
// DELETE takes, and reads nothing from the query string; GET takes it from
// the query string, which access logs and proxies keep, and is there for
// the callers that already use it.
func handleSubjectAccessImpl(cfg Config, w http.ResponseWriter, r *http.Request) {
	var userID, email, tcknHash string
	if r.Method == http.MethodPost {
		defer func() { _ = r.Body.Close() }()
		var body struct {
			UserID   string `json:"user_id"`
			Email    string `json:"email"`
			TCKNHash string `json:"tckn_hash"`
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
				return
			}
		}
		userID, email, tcknHash = body.UserID, body.Email, body.TCKNHash
	} else {
		userID = r.URL.Query().Get("user_id")
		email = r.URL.Query().Get("email")
		tcknHash = r.URL.Query().Get("tckn_hash")
	}
	if userID == "" && email == "" && tcknHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id, email, or tckn_hash required"})
		return
	}
	selector := subjectType(userID, email, tcknHash)
	if selector == selectorTCKNHash && !validTCKNHash(tcknHash) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errTCKNHashFormat})
		return
	}
	if cfg.Store != nil && cfg.DefaultOrgID != "" {
		if accessor, ok := cfg.Store.(subjectAccessor); ok {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			rows, err := accessor.SubjectAccess(ctx, cfg.DefaultOrgID, userID, email, tcknHash, 500)
			if err != nil {
				// The cause is the database's and stays in the log.
				log.Error().Err(err).Str("org_id", cfg.DefaultOrgID).Str("subject_type", selector).Msg("subject access failed")
				cfg.auditSubject(r, auditSubjectAccess, selector, "rows", 0, subjectError)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "subject access failed"})
				return
			}
			// Written before the rows leave: reading what is stored about a
			// person is what the audit trail is for.
			cfg.auditSubject(r, auditSubjectAccess, selector, "rows", len(rows), subjectOK)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"subject": map[string]string{"user_id": userID, "email": email, "tckn_hash": tcknHash},
				"rows":    rows,
				"count":   len(rows),
			})
			return
		}
	}
	cfg.auditSubject(r, auditSubjectAccess, selector, "rows", 0, subjectStoreUnavailable)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subject": map[string]string{"user_id": userID, "email": email, "tckn_hash": tcknHash},
		"rows":    []interface{}{},
		"count":   0,
		"note":    "store not available; subject access requires Postgres",
	})
}

// subjectEraser is implemented by stores that know how to redact a data
// subject's rows. The Postgres store provides it; the noop store does not,
// so we gracefully no-op.
type subjectEraser interface {
	EraseSubject(ctx context.Context, orgID, userID, email, tcknHash string) (int, error)
}

// subjectAccessor is implemented by stores that can return a subject's rows.
type subjectAccessor interface {
	SubjectAccess(ctx context.Context, orgID, userID, email, tcknHash string, limit int) ([]store.RequestLogRow, error)
}

// The audit entries of the two subject routes, and what they say happened.
const (
	auditSubjectAccess = "privacy.subject_access"
	auditSubjectErase  = "privacy.subject_erase"

	subjectOK               = "ok"
	subjectError            = "error"
	subjectStoreUnavailable = "store_unavailable"

	selectorTCKNHash  = "tckn_hash"
	errTCKNHashFormat = "tckn_hash must be a SHA-256 in hex: 64 characters, 0-9 and a-f"
)

// auditSubject records that someone read or erased what is stored about a
// person: who (the verified actor), with which kind of selector, how many
// rows, and how it ended.
//
// The selector's value is not written, neither in the clear nor hashed. The
// audit log is read by more roles than may look a person up, and an erase
// does not reach into it; it must not become a second store of user ids,
// e-mail addresses and national-id hashes, least of all of the people who
// asked to be erased. A hash of an e-mail address is undone by hashing
// addresses, and a keyed one would need a key this deployment does not have.
func (cfg Config) auditSubject(r *http.Request, kind, selector, countKey string, count int, result string) {
	if cfg.Audit == nil {
		return
	}
	cfg.Audit.Append(incidents.AuditEntry{
		Kind:  kind,
		Actor: actorFromRequest(r),
		Detail: map[string]interface{}{
			"selector": selector,
			countKey:   count,
			"result":   result,
		},
	})
}

// validTCKNHash reports whether s has the form of the selector the API
// documents: a SHA-256 in lower-case hex. The store matches this selector as
// a substring of the stored findings, so anything shorter, or a LIKE
// wildcard, would select other people's rows and, on DELETE, erase them.
func validTCKNHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// subjectType returns the kind of selector a subject request is answered by:
// the store uses the first of user_id, e-mail and TCKN hash that is given.
// It names the kind, never the value.
func subjectType(userID, email, tcknHash string) string {
	switch {
	case userID != "":
		return "user_id"
	case email != "":
		return "email"
	case tcknHash != "":
		return selectorTCKNHash
	default:
		return "unknown"
	}
}
