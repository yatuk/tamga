package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/policy"
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
	policyWriteMu.Lock()
	defer policyWriteMu.Unlock()
	// A rollback installs a policy on one person's say. Under dual control
	// the revision's text goes into a proposal instead.
	if cfg.refuseDirectWrite(w, r) {
		return
	}
	// Checked before the file is touched: a revision saved under looser rules
	// must not replace a policy that loads with one that does not.
	next, err := policy.LoadFromBytes([]byte(rev.YAML))
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "revision is not a valid policy: " + err.Error()})
		return
	}
	if cfg.refusePrivileged(w, r, next) {
		return
	}
	before := cfg.PolicyStore.GetPolicy()
	if err := writePolicyFile(cfg.PolicyPath, []byte(rev.YAML)); err != nil {
		writePolicyWriteError(w, err)
		return
	}
	if err := cfg.PolicyStore.Reload(cfg.PolicyPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	cfg.refreshPolicyScanners()
	recordPolicyChange(cfg.Audit, cfg.PolicyHistory, policyChange{
		kind: "policy.rollback", source: sourceRollback, actor: actorFromRequest(r),
		target: id, message: "rollback to " + id, yaml: rev.YAML,
		old: before, new: cfg.PolicyStore.GetPolicy(),
		detail: map[string]interface{}{"revision": id},
	})
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
	if strings.TrimSpace(body.YAML) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty policy"})
		return
	}
	// A proposal is judged when it is written, as a direct write would be,
	// and again when it is approved: what cannot be installed is not left
	// waiting for an approver to find out.
	next := loadPolicyForWrite(w, body.YAML)
	if next == nil {
		return
	}
	if cfg.refusePrivileged(w, r, next) {
		return
	}
	var active *policy.Policy
	if cfg.PolicyStore != nil {
		active = cfg.PolicyStore.GetPolicy()
	}
	changes := policy.Diff(active, next)
	// The author is the verified caller. A name in the body is not read.
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
		cfg.Audit.Append(incidents.AuditEntry{
			Kind: "policy.proposal.create", Target: p.ID, Actor: p.Author,
			Detail: map[string]interface{}{
				"changed":  policy.ChangedPaths(changes),
				"sections": policy.ChangedSections(changes),
				"rules":    policy.ChangedRules(changes),
			},
		})
	}
	writeJSON(w, http.StatusCreated, p)
}

func (cfg Config) handleProposalApprove(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyHistory == nil || cfg.PolicyStore == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "history not configured"})
		return
	}
	id := r.PathValue("id")
	policyWriteMu.Lock()
	defer policyWriteMu.Unlock()
	prop, ok := cfg.PolicyHistory.GetProposal(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "proposal not found"})
		return
	}
	// A proposal is settled once.
	if refuseSettledProposal(w, prop) {
		return
	}
	// Dual control: when governance.require_dual_control is set in the
	// active policy, the author cannot approve their own proposal. Both
	// sides of the comparison are verified actors. Without the flag,
	// self-approval is allowed.
	actor := actorFromRequest(r)
	if cfg.dualControlOn() && (actor == prop.Author || sharedActor(actor)) {
		msg := "dual control required: the author of a proposal cannot approve it"
		if sharedActor(actor) {
			msg += ". " + sharedActorNote(actor)
		}
		writeJSON(w, http.StatusForbidden, map[string]interface{}{"error": msg, "code": codeSelfApproval})
		return
	}
	// The proposal replaces the whole policy with its own text. If the
	// policy changed after it was written, approving it would undo that
	// change without anyone having looked at it.
	stale, err := policyChangedSince(cfg.PolicyHistory, prop.Timestamp)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if stale {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error": "the policy changed after this proposal was written, so approving it would undo that change. " +
				"Reject it and open a new proposal against the current policy.",
			"code": codeProposalStale,
		})
		return
	}
	// The stored text is what is applied, and it is judged again now: the
	// rules a policy is held to, and who may change what, are those of
	// today and of the approver. Before the proposal is marked approved and
	// before the file is touched, for the same reason as a rollback.
	next := loadPolicyForWrite(w, prop.YAML)
	if next == nil {
		return
	}
	if cfg.refusePrivileged(w, r, next) {
		return
	}
	before := cfg.PolicyStore.GetPolicy()
	if err := writePolicyFile(cfg.PolicyPath, []byte(prop.YAML)); err != nil {
		writePolicyWriteError(w, err)
		return
	}
	if err := cfg.PolicyStore.Reload(cfg.PolicyPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	cfg.refreshPolicyScanners()
	prop.Status = "approved"
	prop.ApprovedBy = actor
	prop.ApprovedAt = time.Now().UTC()
	updateErr := cfg.PolicyHistory.UpdateProposal(prop)
	revID, changed := recordPolicyChange(cfg.Audit, cfg.PolicyHistory, policyChange{
		kind: "policy.proposal.approve", source: sourceApprove, actor: actor,
		target: id, message: "approve proposal " + id, yaml: prop.YAML,
		old: before, new: cfg.PolicyStore.GetPolicy(),
		detail: map[string]interface{}{"proposal_author": prop.Author},
	})
	if updateErr != nil {
		// The policy is in force and recorded; only the proposal's own
		// state could not be saved.
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "the policy was applied, but the proposal could not be marked approved: " + updateErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "revision_id": revID, "changed": changed})
}

// refuseSettledProposal answers 409 and returns true for a proposal that
// was already approved or rejected.
func refuseSettledProposal(w http.ResponseWriter, prop history.Proposal) bool {
	if prop.Status == "" || prop.Status == "open" {
		return false
	}
	writeJSON(w, http.StatusConflict, map[string]interface{}{
		"error":  "this proposal is already " + prop.Status + " and cannot be approved or rejected again",
		"code":   codeProposalClosed,
		"status": prop.Status,
	})
	return true
}

func (cfg Config) handleProposalReject(w http.ResponseWriter, r *http.Request) {
	if cfg.PolicyHistory == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "history not configured"})
		return
	}
	id := r.PathValue("id")
	policyWriteMu.Lock()
	defer policyWriteMu.Unlock()
	prop, ok := cfg.PolicyHistory.GetProposal(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "proposal not found"})
		return
	}
	if refuseSettledProposal(w, prop) {
		return
	}
	// Whoever could not approve a proposal for lack of the admin role
	// cannot turn it down either; its author can always withdraw it. A
	// proposal that no longer loads can be rejected by anyone who gets here.
	if actorFromRequest(r) != prop.Author {
		if next, err := policy.LoadFromBytes([]byte(prop.YAML)); err == nil && cfg.refusePrivileged(w, r, next) {
			return
		}
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
