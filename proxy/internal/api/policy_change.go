package api

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/policy/history"
)

// policyWriteMu serialises everything that rewrites the policy or settles a
// proposal, so that a check (is dual control on, is the proposal still open,
// did the policy change since) and the write it guards are one step within
// this process. Replicas that share a Postgres history are not serialised
// against each other.
var policyWriteMu sync.Mutex

// Where a policy change came from, as written to the audit log.
const (
	sourcePut          = "api_put"
	sourceReload       = "reload"
	sourceRollback     = "rollback"
	sourceApprove      = "proposal_approve"
	sourceEntityAdd    = "custom_entity_add"
	sourceEntityDelete = "custom_entity_delete"
	sourceFileWatcher  = "file_watcher"
)

// Error codes of the policy write paths. The console keys on these.
const (
	codeDualControl    = "dual_control_required"
	codeSelfApproval   = "self_approval"
	codeAdminRequired  = "admin_role_required"
	codeProposalClosed = "proposal_closed"
	codeProposalStale  = "proposal_stale"
)

const dualControlMessage = "governance.require_dual_control is on: the policy cannot be written directly. " +
	"Open a proposal (POST /api/v1/policies/proposals) and have a second person approve it. " +
	"Turning dual control off is itself a proposal a second person approves."

// sharedActorNote explains, for an actor that is not one person, why dual
// control cannot be met with it.
func sharedActorNote(actor string) string {
	who := "This request carries no identity of its own"
	switch actor {
	case ActorAdminKey:
		who = "Every caller that uses the static admin key is the same actor (\"admin-key\")"
	case ActorDevMode:
		who = "In dev mode every caller is the same actor (\"dev-mode\")"
	}
	return who + ", so dual control cannot be satisfied this way. Give each person their own API key or sign-in; " +
		"to turn dual control off without one, edit the policy file on disk."
}

// dualControlOn reports whether the active policy asks for a second person.
func (cfg Config) dualControlOn() bool {
	if cfg.PolicyStore == nil {
		return false
	}
	pol := cfg.PolicyStore.GetPolicy()
	return pol != nil && pol.Governance != nil && pol.Governance.RequireDualControl
}

// refuseDirectWrite answers 409 and returns true when the active policy
// requires dual control. Every path that writes the policy without a
// proposal asks it first.
func (cfg Config) refuseDirectWrite(w http.ResponseWriter, r *http.Request) bool {
	if !cfg.dualControlOn() {
		return false
	}
	msg := dualControlMessage
	if actor := actorFromRequest(r); sharedActor(actor) {
		msg += " " + sharedActorNote(actor)
	}
	writeJSON(w, http.StatusConflict, map[string]interface{}{"error": msg, "code": codeDualControl})
	return true
}

// refusePrivileged answers 403 and returns true when the caller is not an
// admin and next differs from the active policy in a section only an admin
// may change. The comparison is of the decoded policies; see
// policy.PrivilegedChanges.
func (cfg Config) refusePrivileged(w http.ResponseWriter, r *http.Request, next *policy.Policy) bool {
	if strings.EqualFold(RoleFrom(r.Context()), "admin") {
		return false
	}
	var active *policy.Policy
	if cfg.PolicyStore != nil {
		active = cfg.PolicyStore.GetPolicy()
	}
	sections := policy.PrivilegedChanges(active, next)
	if len(sections) == 0 {
		return false
	}
	writeJSON(w, http.StatusForbidden, map[string]interface{}{
		"error": "this change needs the admin role: it touches " + strings.Join(sections, ", ") +
			". Without it you may change rules (except notify), output_rules, custom_entities and competitors.",
		"code":                codeAdminRequired,
		"privileged_sections": sections,
	})
	return true
}

// writePolicyLoadError answers a policy that did not load. One that decodes
// but holds refused values is 422 with an issue per field, the shape
// ValidateSemantics failures have; one that does not decode is 400.
func writePolicyLoadError(w http.ResponseWriter, err error) {
	var bad *policy.IssuesError
	if errors.As(err, &bad) {
		writePolicyValidationFailed(w, bad.Issues)
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "policy parse: " + err.Error()})
}

// loadPolicyForWrite decodes and validates policy text someone wants to
// install. It answers the request and returns nil when the text is refused.
func loadPolicyForWrite(w http.ResponseWriter, yamlText string) *policy.Policy {
	pol, err := policy.LoadFromBytes([]byte(yamlText))
	if err != nil {
		writePolicyLoadError(w, err)
		return nil
	}
	if sem := policy.ValidateSemantics(pol); policy.HasValidationErrors(sem) {
		writePolicyValidationFailed(w, sem)
		return nil
	}
	return pol
}

func (cfg Config) refreshPolicyScanners() {
	if cfg.CustomScanner != nil {
		cfg.CustomScanner.Refresh()
	}
	if cfg.CompetitorScanner != nil {
		cfg.CompetitorScanner.Refresh()
	}
}

// policyChange is one change of the active policy, for the record.
type policyChange struct {
	kind    string // audit kind, e.g. "policy.put"
	source  string // one of the source* constants
	actor   string // verified actor; empty for a change of the file on disk
	target  string // audit target; the policy name when empty
	message string // history message
	yaml    string // the text now in force; empty skips the history entry
	old     *policy.Policy
	new     *policy.Policy
	detail  map[string]interface{}
}

// recordPolicyChange writes the audit entry for a policy change and, when
// the text is given, appends it to the policy history. The audit entry names
// what changed by path (sections, rules, keys) and never a value. It returns
// the id of the history revision ("" when none was written) and the paths.
func recordPolicyChange(audit *incidents.AuditRing, hist history.Store, c policyChange) (string, []string) {
	changes := policy.Diff(c.old, c.new)
	paths := policy.ChangedPaths(changes)

	revID := ""
	if hist != nil && c.yaml != "" {
		author := c.actor
		if author == "" {
			author = "policy-file"
		}
		if rev, err := hist.AppendRevision(history.Revision{Author: author, Message: c.message, YAML: c.yaml}); err == nil {
			revID = rev.ID
		}
	}
	if audit != nil {
		detail := map[string]interface{}{
			"source":   c.source,
			"changed":  paths,
			"sections": policy.ChangedSections(changes),
			"rules":    policy.ChangedRules(changes),
		}
		if c.new != nil {
			detail["version"] = c.new.Version
		}
		if revID != "" {
			detail["revision_id"] = revID
		}
		for k, v := range c.detail {
			detail[k] = v
		}
		target := c.target
		if target == "" && c.new != nil {
			target = c.new.Name
		}
		audit.Append(incidents.AuditEntry{Kind: c.kind, Target: target, Actor: c.actor, Detail: detail})
	}
	return revID, paths
}

// RecordPolicyFileChange records a reload the file watcher did: a change of
// the policy file on disk, which has no actor. A reload that changed nothing
// is not recorded; that is what the watcher sees after a write through the
// API, which has its own entry. path is read for the history entry.
func RecordPolicyFileChange(audit *incidents.AuditRing, hist history.Store, path string, old, new *policy.Policy) {
	if len(policy.Diff(old, new)) == 0 {
		return
	}
	text, _ := os.ReadFile(path)
	recordPolicyChange(audit, hist, policyChange{
		kind:    "policy.file_change",
		source:  sourceFileWatcher,
		message: "policy file changed on disk",
		yaml:    string(text),
		old:     old,
		new:     new,
		detail:  map[string]interface{}{"note": "the policy file was changed on disk; no API caller is involved"},
	})
}

// policyChangedSince reports whether the policy history holds a revision
// newer than t. Every write through the API and every reload that changed
// the policy appends one, so a proposal written before the newest revision
// was written against a policy that is no longer the active one.
func policyChangedSince(hist history.Store, t time.Time) (bool, error) {
	revs, err := hist.ListRevisions()
	if err != nil {
		return false, err
	}
	for _, rev := range revs {
		if rev.Timestamp.After(t) {
			return true, nil
		}
	}
	return false, nil
}
