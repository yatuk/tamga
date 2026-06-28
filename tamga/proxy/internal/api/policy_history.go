package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/policy/history"
)

func (cfg Config) handlePolicyHistory(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyHistory == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	revs, err := cfg.PolicyHistory.ListRevisions()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, revs)
}

func (cfg Config) handlePolicyRevisionGet(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyHistory == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "history not configured"})
		return
	}
	id := r.PathValue("id")
	rev, ok := cfg.PolicyHistory.GetRevision(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "revision not found"})
		return
	}
	writeJSON(w, http.StatusOK, rev)
}

func (cfg Config) handlePolicyRollback(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyHistory == nil || cfg.PolicyStore == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "history not configured"})
		return
	}
	id := r.PathValue("id")
	rev, ok := cfg.PolicyHistory.GetRevision(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "revision not found"})
		return
	}
	if err := os.WriteFile(cfg.PolicyPath, []byte(rev.YAML), 0o600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := cfg.PolicyStore.Reload(cfg.PolicyPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_, _ = cfg.PolicyHistory.AppendRevision(history.Revision{
		Author:  actorFromRequest(r),
		Message: "rollback to " + id,
		YAML:    rev.YAML,
	})
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind: "policy.rollback", Target: id, Actor: actorFromRequest(r),
			Detail: map[string]interface{}{"revision": id},
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "revision": id})
}

// --- Proposals ---

func (cfg Config) handleProposalList(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyHistory == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	props, err := cfg.PolicyHistory.ListProposals()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, props)
}

func (cfg Config) handleProposalCreate(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyHistory == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "history not configured"})
		return
	}
	defer func() { _ = r.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var body struct {
		Message string `json:"message"`
		YAML    string `json:"yaml"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	p, err := cfg.PolicyHistory.CreateProposal(history.Proposal{
		Author:  actorFromRequest(r),
		Message: body.Message,
		YAML:    body.YAML,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{Kind: "policy.proposal.create", Target: p.ID, Actor: p.Author})
	}
	writeJSON(w, http.StatusCreated, p)
}

func (cfg Config) handleProposalApprove(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyHistory == nil || cfg.PolicyStore == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "history not configured"})
		return
	}
	id := r.PathValue("id")
	prop, ok := cfg.PolicyHistory.GetProposal(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "proposal not found"})
		return
	}
	// Dual-control: when governance.require_dual_control is set, the author
	// cannot approve their own proposal. Without this flag (or when policy
	// doesn't specify governance), self-approval is allowed.
	actor := actorFromRequest(r)
	if cfg.PolicyStore != nil {
		if pol := cfg.PolicyStore.GetPolicy(); pol != nil &&
			pol.Governance != nil && pol.Governance.RequireDualControl {
			if actor != "" && actor == prop.Author {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "dual-control required: author cannot self-approve"})
				return
			}
		}
	}
	prop.Status = "approved"
	prop.ApprovedBy = actor
	prop.ApprovedAt = time.Now().UTC()
	if err := cfg.PolicyHistory.UpdateProposal(prop); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// Apply the YAML.
	if err := os.WriteFile(cfg.PolicyPath, []byte(prop.YAML), 0o600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := cfg.PolicyStore.Reload(cfg.PolicyPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	rev, _ := cfg.PolicyHistory.AppendRevision(history.Revision{
		Author: actor, Message: "approve proposal " + id, YAML: prop.YAML,
	})
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind: "policy.proposal.approve", Target: id, Actor: actor,
			Detail: map[string]interface{}{"revision_id": rev.ID, "author": prop.Author},
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "revision_id": rev.ID})
}

func (cfg Config) handleProposalReject(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyHistory == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "history not configured"})
		return
	}
	id := r.PathValue("id")
	prop, ok := cfg.PolicyHistory.GetProposal(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "proposal not found"})
		return
	}
	prop.Status = "rejected"
	prop.RejectedBy = actorFromRequest(r)
	prop.RejectedAt = time.Now().UTC()
	if err := cfg.PolicyHistory.UpdateProposal(prop); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{Kind: "policy.proposal.reject", Target: id, Actor: prop.RejectedBy})
	}
	writeJSON(w, http.StatusOK, prop)
}

// actorFromRequest returns the best-effort identity string used in audit
// entries. The admin key and scoped API keys provide distinct identities,
// while Clerk JWT support will drop in via Sprint 2B.
func actorFromRequest(r *http.Request) string {
	if v := r.Header.Get("X-Tamga-Actor"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Tamga-User-Id"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Tamga-Admin-Key"); v != "" {
		return "admin"
	}
	return "anonymous"
}
