package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/policy/history"
)

// newTestPolicyStore creates a PolicyStore with the given policy for testing.
func newTestPolicyStore(p *policy.Policy) *policy.PolicyStore {
	return policy.NewPolicyStore(p)
}

// newFileStore creates a FileStore in a temp directory for testing proposals.
func newFileStore(t *testing.T) *history.FileStore {
	t.Helper()
	fs, err := history.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	return fs
}

// testPolicyYAML is a minimal valid policy for testing.
const testPolicyYAML = "version: \"1.0\"\nname: test-policy\n"

func setPathID(r *http.Request, id string) {
	r.SetPathValue("id", id)
}

func TestProposalApprove_DualControlEnabled(t *testing.T) {
	// Bob (different actor) should be able to approve.
	t.Run("different_actor_can_approve", func(t *testing.T) {
		fs := newFileStore(t)
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")
		_ = os.WriteFile(policyPath, []byte(testPolicyYAML), 0o644)

		prop, _ := fs.CreateProposal(history.Proposal{
			Author:  "alice",
			Message: "test proposal",
			YAML:    testPolicyYAML,
		})
		cfg := Config{
			PolicyStore: newTestPolicyStore(&policy.Policy{
				Name:       "test-policy",
				Governance: &policy.Governance{RequireDualControl: true},
			}),
			PolicyHistory: fs,
			PolicyPath:    policyPath,
		}
		r := httptest.NewRequest(http.MethodPost, "/policies/proposals/"+prop.ID+"/approve", nil)
		setPathID(r, prop.ID)
		r.Header.Set("X-Tamga-Actor", "bob")
		w := httptest.NewRecorder()
		cfg.handleProposalApprove(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("different actor approve: want 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	// Alice (same actor) must not be able to approve with dual-control enabled.
	t.Run("same_actor_blocked", func(t *testing.T) {
		fs := newFileStore(t)
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")
		_ = os.WriteFile(policyPath, []byte(testPolicyYAML), 0o644)

		prop, _ := fs.CreateProposal(history.Proposal{
			Author:  "alice",
			Message: "another proposal",
			YAML:    testPolicyYAML,
		})
		cfg := Config{
			PolicyStore: newTestPolicyStore(&policy.Policy{
				Name:       "test-policy",
				Governance: &policy.Governance{RequireDualControl: true},
			}),
			PolicyHistory: fs,
			PolicyPath:    policyPath,
		}
		r := httptest.NewRequest(http.MethodPost, "/policies/proposals/"+prop.ID+"/approve", nil)
		setPathID(r, prop.ID)
		r.Header.Set("X-Tamga-Actor", "alice")
		w := httptest.NewRecorder()
		cfg.handleProposalApprove(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("same actor approve: want 403, got %d", w.Code)
		}
		var body map[string]string
		_ = json.NewDecoder(w.Body).Decode(&body)
		if body["error"] != "dual-control required: author cannot self-approve" {
			t.Fatalf("unexpected error: %s", body["error"])
		}
	})
}

func TestProposalApprove_DualControlDisabled(t *testing.T) {
	t.Run("governance_nil", func(t *testing.T) {
		fs := newFileStore(t)
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")
		_ = os.WriteFile(policyPath, []byte(testPolicyYAML), 0o644)
		prop, _ := fs.CreateProposal(history.Proposal{
			Author:  "alice",
			Message: "test proposal",
			YAML:    testPolicyYAML,
		})
		cfg := Config{
			PolicyStore: newTestPolicyStore(&policy.Policy{
				Name:       "test-policy",
				Governance: nil,
			}),
			PolicyHistory: fs,
			PolicyPath:    policyPath,
		}
		r := httptest.NewRequest(http.MethodPost, "/policies/proposals/"+prop.ID+"/approve", nil)
		setPathID(r, prop.ID)
		r.Header.Set("X-Tamga-Actor", "alice")
		w := httptest.NewRecorder()
		cfg.handleProposalApprove(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("self-approve without governance: want 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("require_dual_control_false", func(t *testing.T) {
		fs := newFileStore(t)
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")
		_ = os.WriteFile(policyPath, []byte(testPolicyYAML), 0o644)
		prop, _ := fs.CreateProposal(history.Proposal{
			Author:  "alice",
			Message: "another proposal",
			YAML:    testPolicyYAML,
		})
		cfg := Config{
			PolicyStore: newTestPolicyStore(&policy.Policy{
				Name:       "test-policy",
				Governance: &policy.Governance{RequireDualControl: false},
			}),
			PolicyHistory: fs,
			PolicyPath:    policyPath,
		}
		r := httptest.NewRequest(http.MethodPost, "/policies/proposals/"+prop.ID+"/approve", nil)
		setPathID(r, prop.ID)
		r.Header.Set("X-Tamga-Actor", "alice")
		w := httptest.NewRecorder()
		cfg.handleProposalApprove(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("self-approve when disabled: want 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("policy_store_nil", func(t *testing.T) {
		fs := newFileStore(t)
		tmpDir := t.TempDir()
		policyPath := filepath.Join(tmpDir, "policy.yaml")
		_ = os.WriteFile(policyPath, []byte(testPolicyYAML), 0o644)
		prop, _ := fs.CreateProposal(history.Proposal{
			Author:  "alice",
			Message: "third proposal",
			YAML:    testPolicyYAML,
		})
		cfg := Config{
			PolicyStore:   nil,
			PolicyHistory: fs,
			PolicyPath:    policyPath,
		}
		r := httptest.NewRequest(http.MethodPost, "/policies/proposals/"+prop.ID+"/approve", nil)
		setPathID(r, prop.ID)
		r.Header.Set("X-Tamga-Actor", "alice")
		w := httptest.NewRecorder()
		cfg.handleProposalApprove(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("approve without policy store: want 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestProposalReject(t *testing.T) {
	fs := newFileStore(t)
	prop, _ := fs.CreateProposal(history.Proposal{
		Author:  "alice",
		Message: "test proposal",
		YAML:    testPolicyYAML,
	})

	cfg := Config{
		PolicyStore:   newTestPolicyStore(&policy.Policy{Name: "test-policy"}),
		PolicyHistory: fs,
	}

	r := httptest.NewRequest(http.MethodPost, "/policies/proposals/"+prop.ID+"/reject", nil)
	setPathID(r, prop.ID)
	r.Header.Set("X-Tamga-Actor", "bob")
	w := httptest.NewRecorder()
	cfg.handleProposalReject(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("reject: want 200, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&result)
	if result["status"] != "rejected" {
		t.Fatalf("want status rejected, got %v", result["status"])
	}
}

func TestProposalList_Empty(t *testing.T) {
	fs := newFileStore(t)
	cfg := Config{PolicyHistory: fs}

	r := httptest.NewRequest(http.MethodGet, "/policies/proposals", nil)
	w := httptest.NewRecorder()
	cfg.handleProposalList(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", w.Code)
	}
}

func TestProposalCreate(t *testing.T) {
	fs := newFileStore(t)
	cfg := Config{PolicyHistory: fs}

	body := `{"message":"test","yaml":"version: \"1.0\"\nname: test\n"}`
	r := httptest.NewRequest(http.MethodPost, "/policies/proposals", strings.NewReader(body))
	r.Header.Set("X-Tamga-Actor", "alice")
	w := httptest.NewRecorder()
	cfg.handleProposalCreate(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", w.Code, w.Body.String())
	}
	var result map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&result)
	if result["author"] != "alice" {
		t.Fatalf("want author alice, got %v", result["author"])
	}
	if result["status"] != "open" {
		t.Fatalf("want status open, got %v", result["status"])
	}
}
