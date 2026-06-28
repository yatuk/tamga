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
	deleted := 0
	if cfg.Store != nil && cfg.DefaultOrgID != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if eraser, ok := cfg.Store.(subjectEraser); ok {
			var err error
			deleted, err = eraser.EraseSubject(ctx, cfg.DefaultOrgID, body.UserID, body.Email, body.TCKNHash)
			if err != nil {
				log.Error().Err(err).Str("org_id", cfg.DefaultOrgID).Str("subject_type", subjectType(body.UserID, body.Email, body.TCKNHash)).Msg("subject erase failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "subject erase failed"})
				return
			}
		}
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind: "privacy.subject_erase", Actor: actorFromRequest(r),
			Detail: map[string]interface{}{
				"user_id":   body.UserID,
				"email":     body.Email,
				"tckn_hash": body.TCKNHash,
				"deleted":   deleted,
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"deleted": deleted,
		"note":    "rows soft-deleted; retention sweep finalises removal",
	})
}

// handleSubjectAccess returns request_log rows for a data subject (GDPR Art. 15).
func handleSubjectAccessImpl(cfg Config, w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	email := r.URL.Query().Get("email")
	tcknHash := r.URL.Query().Get("tckn_hash")
	if userID == "" && email == "" && tcknHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id, email, or tckn_hash query param required"})
		return
	}
	if cfg.Store != nil && cfg.DefaultOrgID != "" {
		if accessor, ok := cfg.Store.(subjectAccessor); ok {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			rows, err := accessor.SubjectAccess(ctx, cfg.DefaultOrgID, userID, email, tcknHash, 500)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"subject": map[string]string{"user_id": userID, "email": email, "tckn_hash": tcknHash},
				"rows":    rows,
				"count":   len(rows),
			})
			return
		}
	}
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

// subjectType returns the type of identifier used for the erase request,
// for logging purposes only (no PII values are included).
func subjectType(userID, email, tcknHash string) string {
	switch {
	case userID != "":
		return "user_id"
	case email != "":
		return "email"
	case tcknHash != "":
		return "tckn_hash"
	default:
		return "unknown"
	}
}
