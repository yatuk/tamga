package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/classifier"
	"github.com/yatuk/tamga/internal/events"
)

// The shadow comparison is counted by shadow model and outcome, and only
// once a shadow has answered.
func TestMetrics_ShadowCounters(t *testing.T) {
	classifier.RecordShadow("metrics-candidate", classifier.ShadowOnly)
	classifier.RecordShadow("metrics-candidate", classifier.ShadowOnly)
	classifier.RecordShadow("metrics-candidate", classifier.ShadowPrimaryOnly)

	rec := httptest.NewRecorder()
	Config{}.handleMetrics(rec, httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil))
	out := rec.Body.String()
	for _, want := range []string{
		"# TYPE tamga_classifier_shadow_texts_total counter",
		`tamga_classifier_shadow_texts_total{shadow_model="metrics-candidate",outcome="shadow_only"} 2`,
		`tamga_classifier_shadow_texts_total{shadow_model="metrics-candidate",outcome="primary_only"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics lack %q", want)
		}
	}
}

// The stored detail of a request scored with a shadow model carries the
// scores; one without carries none.
func TestEventDetail_ClassifierScores(t *testing.T) {
	now := time.Now().UTC()
	h := newFakeHistory()
	h.add(histOrg, "s1", "openai", "gpt-4o", "pass", now.Add(-time.Minute), 0)
	h.add(histOrg, "s2", "openai", "gpt-4o", "pass", now.Add(-2*time.Minute), 0)
	shadow := 0.997
	raw, _ := json.Marshal(events.ClassifierRecord{
		Model: "primary", Threshold: 0.98, ShadowModel: "candidate", ShadowThreshold: 0.995,
		Texts: []events.ClassifierText{{Seg: 1, Role: "user", Score: 0.2, Threshold: 0.98, Shadow: &shadow, ShadowFlagged: true}},
	})
	h.rows[0].rec.ClassifierScores = raw

	status, _, body := histGet(t, histConfig(h, events.NewRecentBuffer(10)), "/events/s1")
	if status != http.StatusOK || !strings.Contains(string(body), `"classifier_scores":{`) || !strings.Contains(string(body), `"shadow":0.997`) {
		t.Fatalf("stored detail: status %d: %s", status, body)
	}
	status, _, body = histGet(t, histConfig(h, events.NewRecentBuffer(10)), "/events/s2")
	if status != http.StatusOK || strings.Contains(string(body), "classifier_scores") {
		t.Fatalf("detail without a shadow: status %d: %s", status, body)
	}
}
