package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/store"
)

// subjectBody is the selector of a subject request as a JSON body. One is
// used: the first that is given, in this order.
type subjectBody struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	TCKNHash string `json:"tckn_hash"`
	TCKN     string `json:"tckn"`
}

// handleSubjectEraseImpl deletes request_log rows that match the supplied
// subject identifier (user_id, email, a national id, or a stored digest).
// When Postgres is not configured we return a placeholder so callers can
// wire UI without a DB.
// Every call that names a subject is written to the audit log: see
// auditSubject.
func handleSubjectEraseImpl(cfg Config, w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var body subjectBody
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
	}
	if body.UserID == "" && body.Email == "" && body.TCKNHash == "" && body.TCKN == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errSubjectRequired})
		return
	}
	selector := subjectType(body.UserID, body.Email, body.TCKNHash, body.TCKN)
	if msg := subjectSelectorError(selector, body.TCKNHash, body.TCKN); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	deleted := 0
	result := subjectStoreUnavailable
	if cfg.Store != nil && cfg.DefaultOrgID != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var err error
		deleted, err = eraseSubject(ctx, cfg, selector, body)
		if err != nil && !errors.Is(err, errNoSubjectStore) {
			log.Error().Err(err).Str("org_id", cfg.DefaultOrgID).Str("subject_type", selector).Msg("subject erase failed")
			cfg.auditSubject(r, auditSubjectErase, selector, "deleted", 0, subjectError)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "subject erase failed"})
			return
		}
		if err == nil {
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

// eraseSubject deletes the rows of the selector. It returns
// errNoSubjectStore when the store cannot answer that kind of selector.
//
// A national id is turned into the forms the log may hold it in here, and
// goes no further: not to the store, a log line or the audit entry.
func eraseSubject(ctx context.Context, cfg Config, selector string, body subjectBody) (int, error) {
	if selector == selectorTCKN {
		if eraser, ok := cfg.Store.(subjectMatchEraser); ok {
			return eraser.EraseSubjectByMatch(ctx, cfg.DefaultOrgID, store.SubjectMatchForms(body.TCKN))
		}
		return 0, errNoSubjectStore
	}
	if eraser, ok := cfg.Store.(subjectEraser); ok {
		return eraser.EraseSubject(ctx, cfg.DefaultOrgID, body.UserID, body.Email, body.TCKNHash)
	}
	return 0, errNoSubjectStore
}

// accessSubject returns the rows of the selector, as eraseSubject deletes
// them.
func accessSubject(ctx context.Context, cfg Config, selector string, body subjectBody) ([]store.RequestLogRow, error) {
	if selector == selectorTCKN {
		if accessor, ok := cfg.Store.(subjectMatchAccessor); ok {
			return accessor.SubjectAccessByMatch(ctx, cfg.DefaultOrgID, store.SubjectMatchForms(body.TCKN), 500)
		}
		return nil, errNoSubjectStore
	}
	if accessor, ok := cfg.Store.(subjectAccessor); ok {
		return accessor.SubjectAccess(ctx, cfg.DefaultOrgID, body.UserID, body.Email, body.TCKNHash, 500)
	}
	return nil, errNoSubjectStore
}

// handleSubjectAccess returns request_log rows for a data subject (GDPR Art. 15).
//
// The selector names a person. POST takes it from a JSON body, the fields
// DELETE takes, and reads nothing from the query string; GET takes it from
// the query string, which access logs and proxies keep, and is there for
// the callers that already use it. A national id (tckn) is therefore taken
// from a body only.
func handleSubjectAccessImpl(cfg Config, w http.ResponseWriter, r *http.Request) {
	var body subjectBody
	if r.Method == http.MethodPost {
		defer func() { _ = r.Body.Close() }()
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
	} else {
		if r.URL.Query().Has("tckn") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errTCKNInQuery})
			return
		}
		body.UserID = r.URL.Query().Get("user_id")
		body.Email = r.URL.Query().Get("email")
		body.TCKNHash = r.URL.Query().Get("tckn_hash")
	}
	if body.UserID == "" && body.Email == "" && body.TCKNHash == "" && body.TCKN == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errSubjectRequired})
		return
	}
	selector := subjectType(body.UserID, body.Email, body.TCKNHash, body.TCKN)
	if msg := subjectSelectorError(selector, body.TCKNHash, body.TCKN); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	// The answer repeats the selector it was given, except a national id:
	// that is said by its kind only.
	subject := map[string]string{"user_id": body.UserID, "email": body.Email, "tckn_hash": body.TCKNHash}
	if selector == selectorTCKN {
		subject["selector"] = selectorTCKN
	}
	if cfg.Store != nil && cfg.DefaultOrgID != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := accessSubject(ctx, cfg, selector, body)
		if err != nil && !errors.Is(err, errNoSubjectStore) {
			// The cause is the database's and stays in the log.
			log.Error().Err(err).Str("org_id", cfg.DefaultOrgID).Str("subject_type", selector).Msg("subject access failed")
			cfg.auditSubject(r, auditSubjectAccess, selector, "rows", 0, subjectError)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "subject access failed"})
			return
		}
		if err == nil {
			// Written before the rows leave: reading what is stored about a
			// person is what the audit trail is for.
			cfg.auditSubject(r, auditSubjectAccess, selector, "rows", len(rows), subjectOK)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"subject": subject,
				"rows":    rows,
				"count":   len(rows),
			})
			return
		}
	}
	cfg.auditSubject(r, auditSubjectAccess, selector, "rows", 0, subjectStoreUnavailable)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subject": subject,
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

// subjectMatchAccessor and subjectMatchEraser are the same two for a
// selector that is found in the stored findings: the store is given the
// forms the log may hold the identifier in (store.SubjectMatchForms), never
// the identifier.
type subjectMatchAccessor interface {
	SubjectAccessByMatch(ctx context.Context, orgID string, forms []string, limit int) ([]store.RequestLogRow, error)
}

type subjectMatchEraser interface {
	EraseSubjectByMatch(ctx context.Context, orgID string, forms []string) (int, error)
}

// errNoSubjectStore says the store cannot answer the selector that was sent.
var errNoSubjectStore = errors.New("store cannot answer this selector")

// The audit entries of the two subject routes, and what they say happened.
const (
	auditSubjectAccess = "privacy.subject_access"
	auditSubjectErase  = "privacy.subject_erase"

	subjectOK               = "ok"
	subjectError            = "error"
	subjectStoreUnavailable = "store_unavailable"

	selectorTCKNHash  = "tckn_hash"
	selectorTCKN      = "tckn"
	errTCKNHashFormat = "tckn_hash must be a stored digest in hex: 64 characters, 0-9 and a-f"
	// The next two say what is wrong with a national id without repeating it.
	errTCKNFormat      = "tckn must be a national id: 11 digits with a valid checksum"
	errTCKNInQuery     = "tckn is read from a POST or DELETE body only: a national id must not be put in a URL"
	errSubjectRequired = "user_id, email, tckn, or tckn_hash required"
)

// auditSubject records that someone read or erased what is stored about a
// person: who (the verified actor), with which kind of selector, how many
// rows, and how it ended.
//
// The selector's value is not written, neither in the clear nor hashed. The
// audit log is read by more roles than may look a person up, and an erase
// does not reach into it; it must not become a second store of user ids,
// e-mail addresses and national ids, least of all of the people who asked
// to be erased. A hash of an e-mail address is undone by hashing addresses,
// and a keyed one would still say which calls named the same person.
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

// subjectSelectorError returns what is wrong with the selector a subject
// request is answered by, or "" when it can be used.
func subjectSelectorError(selector, tcknHash, tckn string) string {
	switch {
	case selector == selectorTCKNHash && !validTCKNHash(tcknHash):
		return errTCKNHashFormat
	case selector == selectorTCKN && !validTCKN(tckn):
		return errTCKNFormat
	}
	return ""
}

// validTCKNHash reports whether s has the form of the selector the API
// documents: the 64 lower-case hex characters of a stored digest (what
// follows "hmac-sha256:", or "sha256:" in rows of earlier versions). The
// store matches this selector as a substring of the stored findings, so
// anything shorter, or a LIKE wildcard, would select other people's rows
// and, on DELETE, erase them.
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

// validTCKN reports whether s is a Turkish national id as the scanner
// records one: 11 digits, the first not 0, with both check digits right. A
// mistyped id is refused here instead of silently finding nothing.
func validTCKN(s string) bool {
	if len(s) != 11 || s[0] == '0' {
		return false
	}
	var d [11]int
	for i := 0; i < 11; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		d[i] = int(s[i] - '0')
	}
	odd := d[0] + d[2] + d[4] + d[6] + d[8]
	even := d[1] + d[3] + d[5] + d[7]
	if ((odd*7-even)%10+10)%10 != d[9] {
		return false
	}
	sum := 0
	for i := 0; i < 10; i++ {
		sum += d[i]
	}
	return sum%10 == d[10]
}

// subjectType returns the kind of selector a subject request is answered by:
// the first that is given of user_id, e-mail, stored digest and national id.
// It names the kind, never the value.
func subjectType(userID, email, tcknHash, tckn string) string {
	switch {
	case userID != "":
		return "user_id"
	case email != "":
		return "email"
	case tcknHash != "":
		return selectorTCKNHash
	case tckn != "":
		return selectorTCKN
	default:
		return "unknown"
	}
}
