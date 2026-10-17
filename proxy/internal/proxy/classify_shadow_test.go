package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/classifier"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
)

// shadowScorer is semanticScorer with a shadow model beside it that sees
// what the primary does not: "candidate catches" scores high with it only.
type shadowScorer struct {
	semanticScorer
	model   string
	skipped string // when set, the shadow gives no scores and says why
}

func (s *shadowScorer) Score(ctx context.Context, id string, texts []string) ([]classifier.Result, error) {
	out, err := s.semanticScorer.Score(ctx, id, texts)
	if err != nil {
		return nil, err
	}
	for i, t := range texts {
		out[i].Model = "primary-model"
		out[i].ShadowModel = s.model
		if s.skipped != "" {
			out[i].ShadowSkipped = s.skipped
			continue
		}
		out[i].HasShadow = true
		out[i].ShadowScore = 0.01
		if strings.Contains(t, "candidate catches") {
			out[i].ShadowScore = 0.995
		}
	}
	return out, nil
}

type shadowRig struct {
	url    string
	mu     sync.Mutex
	events []events.Event
}

func newShadowRig(t *testing.T, policyYAML string, scorer classifier.Scorer) *shadowRig {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	pol := mustPolicy(t, policyYAML)
	rig := &shadowRig{}
	bus := events.NewBus()
	bus.Subscribe(func(e events.Event) {
		rig.mu.Lock()
		rig.events = append(rig.events, e)
		rig.mu.Unlock()
	})
	bus.Start()
	t.Cleanup(bus.Stop)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
		Classifier:   classifier.NewGuard(scorer),
		Bus:          bus,
	}))
	t.Cleanup(srv.Close)
	rig.url = srv.URL
	return rig
}

// event waits for the n-th request event (1-based).
func (r *shadowRig) event(t *testing.T, n int) events.Event {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		var reqs []events.Event
		for _, e := range r.events {
			if e.EventType == "request_scanned" || e.EventType == "request_blocked" {
				reqs = append(reqs, e)
			}
		}
		r.mu.Unlock()
		if len(reqs) >= n {
			return reqs[n-1]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("request event %d was not published", n)
	return events.Event{}
}

func shadowCount(model, outcome string) int64 {
	for _, c := range classifier.ShadowStats() {
		if c.Model == model && c.Outcome == outcome {
			return c.Texts
		}
	}
	return 0
}

func TestShadow_RecordedNeverActedOn(t *testing.T) {
	const model = "candidate-recorded"
	pol := classifierPolicy("block", "    shadow_threshold: 0.99\n")
	rig := newShadowRig(t, pol, &shadowScorer{model: model})

	// The shadow would flag this, the primary does not: it goes through.
	resp, body := postBody(t, rig.url, userMessage("the candidate catches this sentence and nobody else does"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("a shadow score changed the verdict: %d %s", resp.StatusCode, body)
	}
	if strings.Contains(body, "shadow") {
		t.Fatalf("the response mentions the shadow: %s", body)
	}
	e := rig.event(t, 1)
	rec := e.Classifier
	if rec == nil || rec.Model != "primary-model" || rec.ShadowModel != model || rec.Threshold != 0.9 || rec.ShadowThreshold != 0.99 {
		t.Fatalf("record = %+v", rec)
	}
	if len(rec.Texts) != 1 {
		t.Fatalf("texts = %+v", rec.Texts)
	}
	got := rec.Texts[0]
	if got.Flagged || got.Shadow == nil || *got.Shadow != 0.995 || !got.ShadowFlagged || got.Role != "user" || got.Seg < 0 || got.Threshold != 0.9 {
		t.Fatalf("text = %+v", got)
	}
	if n := shadowCount(model, classifier.ShadowOnly); n != 1 {
		t.Fatalf("shadow_only = %d, want 1", n)
	}

	// The primary flags this, the shadow would not: blocked as before.
	resp, body = postBody(t, rig.url, userMessage(semanticAttack))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", resp.StatusCode, body)
	}
	e = rig.event(t, 2)
	if e.Action != "BLOCK" || e.Classifier == nil || !e.Classifier.Texts[0].Flagged || e.Classifier.Texts[0].ShadowFlagged {
		t.Fatalf("blocked event: action %q, record %+v", e.Action, e.Classifier)
	}
	if n := shadowCount(model, classifier.ShadowPrimaryOnly); n != 1 {
		t.Fatalf("primary_only = %d, want 1", n)
	}
	// The verdict and its findings are what they are without a shadow.
	plain := classifierServer(t, pol, &semanticScorer{})
	presp, pbody := postBody(t, plain, userMessage(semanticAttack))
	if presp.StatusCode != resp.StatusCode || stripIDs(pbody) != stripIDs(body) {
		t.Fatalf("with a shadow: %d %s\nwithout: %d %s", resp.StatusCode, body, presp.StatusCode, pbody)
	}
}

// stripIDs drops the request id, which differs between two requests.
func stripIDs(body string) string {
	if i := strings.Index(body, `"request_id"`); i >= 0 {
		if j := strings.Index(body[i:], ","); j >= 0 {
			return body[:i] + body[i+j:]
		}
	}
	return body
}

func TestShadow_CachedTextIsRecordedNotCountedAgain(t *testing.T) {
	const model = "candidate-cached"
	rig := newShadowRig(t, classifierPolicy("block", ""), &shadowScorer{model: model})
	msg := userMessage("What time does the meeting start tomorrow?")
	for i := 0; i < 2; i++ {
		if resp, body := postBody(t, rig.url, msg); resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.StatusCode, body)
		}
	}
	second := rig.event(t, 2).Classifier
	if second == nil || !second.Texts[0].Cached || second.Texts[0].Shadow == nil {
		t.Fatalf("second record = %+v", second)
	}
	// Default shadow threshold: the primary's.
	if second.ShadowThreshold != 0.9 {
		t.Fatalf("shadow threshold = %v, want the primary's 0.9", second.ShadowThreshold)
	}
	if n := shadowCount(model, classifier.ShadowNeither); n != 1 {
		t.Fatalf("neither = %d, want 1: a text from the cache was counted again", n)
	}
}

func TestShadow_MissingScoresAreRecordedAsSuch(t *testing.T) {
	const model = "candidate-late"
	rig := newShadowRig(t, classifierPolicy("block", ""), &shadowScorer{model: model, skipped: "late"})
	if resp, body := postBody(t, rig.url, userMessage("the candidate catches this one too, if it were on time")); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	rec := rig.event(t, 1).Classifier
	if rec == nil || rec.Texts[0].Shadow != nil || rec.Texts[0].ShadowSkipped != "late" || rec.Texts[0].ShadowFlagged {
		t.Fatalf("record = %+v", rec)
	}
	if n := shadowCount(model, classifier.ShadowMissing); n != 1 {
		t.Fatalf("missing = %d, want 1", n)
	}
}

func TestShadow_NoShadowModelNoRecord(t *testing.T) {
	rig := newShadowRig(t, classifierPolicy("block", ""), &semanticScorer{})
	if resp, body := postBody(t, rig.url, userMessage("What time does the meeting start tomorrow?")); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if rec := rig.event(t, 1).Classifier; rec != nil {
		t.Fatalf("a request scored without a shadow carries a record: %+v", rec)
	}
}

// The LiteLLM route stores the comparison with its event, as the proxy
// path does.
func TestShadow_LiteLLMRouteRecordsTheScores(t *testing.T) {
	const model = "candidate-litellm"
	pol := strings.Replace(litellmPolicy, "providers:\n", "scan:\n  classifier:\n    enabled: true\n    threshold: 0.9\nproviders:\n", 1)
	g := newScanRig(t, scanRigOptions{policy: pol, scorer: &shadowScorer{model: model}})
	key := bridgeKey(t, g)
	resp, raw := g.litellmRaw(t, key, litellmToolBody("request", []string{"the candidate catches this sentence and nobody else does"}, nil, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, raw)
	}
	e := g.bridgeEvents(t, 1)[0]
	if e.Action != "PASS" || e.Classifier == nil || e.Classifier.ShadowModel != model || len(e.Classifier.Texts) != 1 || !e.Classifier.Texts[0].ShadowFlagged || e.Classifier.Texts[0].Flagged {
		t.Fatalf("event: action %q, record %+v", e.Action, e.Classifier)
	}
}
