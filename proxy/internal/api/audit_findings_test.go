package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/incidents"
)

// unreadableIncidents makes the lifecycle changes and then cannot read the
// incident back, as a database does that takes the write and times out on
// the read.
type unreadableIncidents struct {
	*incidents.MemoryStore
}

func (unreadableIncidents) Get(string) (incidents.State, error) {
	return incidents.State{}, errors.New("read timed out")
}

// A triage, a resolve and a reopen are on the audit log once they are made,
// whether or not the incident can be read back for the answer.
func TestIncidentLifecycleIsAuditedWhenTheReadBackFails(t *testing.T) {
	f := newRealFixture(t)
	mem := incidents.NewMemoryStore()
	for name, store := range map[string]incidents.Store{"read fails": unreadableIncidents{mem}, "no incident store": nil} {
		audit := incidents.NewAuditRing(16)
		h := testMux(Config{
			AdminKey: authzAdminKey, JWTSecret: authzSecret, Users: f.users,
			Incidents: store, IncidentLifecycle: mem, Audit: audit,
		})
		for _, step := range []struct {
			action, body, kind string
		}{
			{"triage", `{"assignee":"ada"}`, "incident.triage"},
			{"resolve", `{"resolution":"false_positive"}`, "incident.resolve"},
			{"reopen", ``, "incident.reopen"},
		} {
			req, _ := http.NewRequest("POST", "/api/v1/incidents/req-1/"+step.action, strings.NewReader(step.body))
			req.Header.Set("Authorization", "Bearer "+f.tokens["analyst"])
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"request_id":"req-1"`) {
				t.Fatalf("%s, %s: %d %s", name, step.action, rec.Code, rec.Body.String())
			}
			var found *incidents.AuditEntry
			for _, e := range audit.List(0) {
				if e.Kind == step.kind {
					e := e
					found = &e
				}
			}
			if found == nil {
				t.Errorf("%s: no %s entry although the change was made", name, step.kind)
				continue
			}
			if found.Actor != "user:u-analyst" || found.Target != "req-1" {
				t.Errorf("%s: %s entry = %+v", name, step.kind, *found)
			}
		}
	}
}

// The subject of an access request is a person. POST takes the selector in
// the body and reads none from the query string; GET still works.
func TestSubjectAccessByPost(t *testing.T) {
	f := newRealFixture(t)
	const email = "ada@example.invalid"

	rec := f.as("admin", "POST", "/events/subject", `{"email":"`+email+`"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"email":"`+email+`"`) {
		t.Fatalf("POST with the selector in the body: %d %s", rec.Code, rec.Body.String())
	}
	// The query string is not where POST looks.
	if rec := f.as("admin", "POST", "/events/subject?email="+email, ``); rec.Code != http.StatusBadRequest {
		t.Errorf("POST with the selector in the query string only: %d %s, want 400", rec.Code, rec.Body.String())
	}
	if rec := f.as("admin", "POST", "/events/subject?email=other@example.invalid", `{"user_id":"u1"}`); rec.Code != http.StatusOK ||
		strings.Contains(rec.Body.String(), "other@example.invalid") || !strings.Contains(rec.Body.String(), `"user_id":"u1"`) {
		t.Errorf("POST with both: %d %s, want the body's selector only", rec.Code, rec.Body.String())
	}
	if rec := f.as("admin", "POST", "/events/subject", `{"email":`); rec.Code != http.StatusBadRequest {
		t.Errorf("POST with a broken body: %d %s", rec.Code, rec.Body.String())
	}
	// Same permission as GET: admin only.
	for _, role := range []string{"viewer", "analyst"} {
		if rec := f.as(role, "POST", "/events/subject", `{"email":"`+email+`"}`); rec.Code != http.StatusForbidden {
			t.Errorf("%s POST /events/subject: %d, want 403", role, rec.Code)
		}
	}
	// GET is as it was.
	if rec := f.as("admin", "GET", "/events/subject?email="+email, ``); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), email) {
		t.Errorf("GET with the selector in the query string: %d %s", rec.Code, rec.Body.String())
	}
}

// No route updates a webhook: the handler that had none is gone, and the
// method is not served.
func TestWebhookUpdateHasNoRoute(t *testing.T) {
	f := newRealFixture(t)
	rec := f.as("admin", "PUT", "/webhooks/some-id", `{"label":"x"}`)
	if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
		t.Errorf("PUT /webhooks/{id}: %d %s", rec.Code, rec.Body.String())
	}
}
