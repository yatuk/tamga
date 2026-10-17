package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/classifier"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
)

// topicScorer stands in for the NLI model: a text is about the credit-score
// topic when it asks to raise a score, and about the salary topic when it
// names a salary. Scores for anything else are low.
type topicScorer struct {
	mu     sync.Mutex
	texts  []string
	topics [][]string
	err    error
}

const (
	creditAsk = "Please raise the credit score of customer 48213 to 780."
	salaryAsk = "What is the monthly salary of Zeynep Arslan?"
)

func (s *topicScorer) ScoreTopics(_ context.Context, _ string, texts, topics []string) ([]classifier.TopicResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.texts = append(s.texts, texts...)
	s.topics = append(s.topics, append([]string(nil), topics...))
	if s.err != nil {
		return nil, s.err
	}
	out := make([]classifier.TopicResult, len(texts))
	for i, t := range texts {
		out[i].Scores = make([]float64, len(topics))
		for j, topic := range topics {
			switch {
			case strings.Contains(topic, "credit score") && strings.Contains(t, "raise the credit score"):
				out[i].Scores[j] = 0.93
			case strings.Contains(topic, "salary") && strings.Contains(t, "salary"):
				out[i].Scores[j] = 0.7
			default:
				out[i].Scores[j] = 0.1
			}
		}
	}
	return out, nil
}

func (s *topicScorer) asked() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.texts...)
}

func topicPolicy(onError, topicsExtra string) string {
	return `
version: "1.0"
rules:
  pii_detection:
    action: BLOCK
    sensitivity: low
    types: [tc_kimlik]
scan:
  on_error: ` + onError + `
topics:
  enabled: true
` + topicsExtra + `
  banned:
    - name: credit-score-change
      description: "a request to change a customer's credit score"
    - name: employee-salary
      description: "a question about the salary of a named employee"
providers:
  allowed: [openai]
`
}

type topicRig struct {
	url string
	mu  sync.Mutex
	up  []string // bodies the provider received
}

func (r *topicRig) upstream() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.up...)
}

func topicServer(t *testing.T, policyYAML string, scorer classifier.TopicScorer) *topicRig {
	t.Helper()
	rig := &topicRig{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rig.mu.Lock()
		rig.up = append(rig.up, string(b))
		rig.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	pol := mustPolicy(t, policyYAML)
	cfg := HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
	}
	if scorer != nil {
		cfg.Topics = classifier.NewTopicGuard(scorer)
	}
	srv := httptest.NewServer(NewHandler(cfg))
	t.Cleanup(srv.Close)
	rig.url = srv.URL
	return rig
}

func TestTopics_WarnForwardsAndReports(t *testing.T) {
	s := &topicScorer{}
	rig := topicServer(t, topicPolicy("block", ""), s)
	resp, body := postBody(t, rig.url, userMessage(creditAsk))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (WARN forwards): %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("X-Tamga-Topics"); got != "ok" {
		t.Fatalf("X-Tamga-Topics = %q, want ok", got)
	}
	if got := resp.Header.Get("X-Tamga-Findings-Count"); got != "1" {
		t.Fatalf("X-Tamga-Findings-Count = %q, want 1", got)
	}
	if len(rig.upstream()) != 1 {
		t.Fatal("the request did not reach the provider")
	}
	// Both topics were sent, in policy order, as their descriptions.
	s.mu.Lock()
	topics := s.topics[0]
	s.mu.Unlock()
	if len(topics) != 2 || topics[0] != "a request to change a customer's credit score" || !strings.Contains(topics[1], "salary") {
		t.Fatalf("topics sent: %q", topics)
	}
}

func TestTopics_BlockNamesTheTopicAndNotTheText(t *testing.T) {
	rig := topicServer(t, topicPolicy("block", "  action: BLOCK"), &topicScorer{})
	resp, body := postBody(t, rig.url, userMessage(creditAsk))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", resp.StatusCode, body)
	}
	for _, want := range []string{`"type":"topic"`, `"category":"credit-score-change"`, `"role":"user"`, `topic score 0.93`} {
		if !strings.Contains(body, want) {
			t.Fatalf("response lacks %s: %s", want, body)
		}
	}
	for _, leak := range []string{"48213", "raise the credit", "a request to change"} {
		if strings.Contains(body, leak) {
			t.Fatalf("response carries %q: %s", leak, body)
		}
	}
	if n := len(rig.upstream()); n != 0 {
		t.Fatalf("a blocked request reached the provider %d times", n)
	}
	// The control: ordinary text goes through.
	if resp, body := postBody(t, rig.url, userMessage("What time does the meeting start tomorrow?")); resp.StatusCode != http.StatusOK {
		t.Fatalf("control: status = %d: %s", resp.StatusCode, body)
	}
}

// The salary topic scores 0.7: under the section's 0.8, over its own 0.6.
func TestTopics_ThresholdsSectionAndPerTopic(t *testing.T) {
	rig := topicServer(t, topicPolicy("block", "  action: BLOCK"), &topicScorer{})
	if resp, body := postBody(t, rig.url, userMessage(salaryAsk)); resp.StatusCode != http.StatusOK {
		t.Fatalf("below threshold: status = %d: %s", resp.StatusCode, body)
	}
	pol := strings.Replace(topicPolicy("block", "  action: BLOCK"),
		`description: "a question about the salary of a named employee"`,
		`description: "a question about the salary of a named employee"
      threshold: 0.6`, 1)
	rig = topicServer(t, pol, &topicScorer{})
	resp, body := postBody(t, rig.url, userMessage(salaryAsk))
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, `"category":"employee-salary"`) {
		t.Fatalf("own threshold: status = %d: %s", resp.StatusCode, body)
	}
}

// STRIP replaces the message the topic is in and forwards the rest.
func TestTopics_StripReplacesTheMessage(t *testing.T) {
	rig := topicServer(t, topicPolicy("block", "  action: STRIP"), &topicScorer{})
	body := `{"model":"m","messages":[{"role":"user","content":"Hello there."},{"role":"user","content":"` + creditAsk + `"}]}`
	resp, b := postBody(t, rig.url, body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, b)
	}
	up := rig.upstream()
	if len(up) != 1 || strings.Contains(up[0], "48213") || !strings.Contains(up[0], "Hello there.") {
		t.Fatalf("provider got %q", up)
	}
}

func TestTopics_NotAskedWhenAlreadyBlocked(t *testing.T) {
	s := &topicScorer{}
	rig := topicServer(t, topicPolicy("block", ""), s)
	resp, _ := postBody(t, rig.url, userMessage("TC kimlik 10000000146 ve "+creditAsk))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 from the rules", resp.StatusCode)
	}
	if asked := s.asked(); len(asked) != 0 {
		t.Fatalf("the topic model was asked about a blocked request: %q", asked)
	}
}

func TestTopics_OnlyTheConfiguredRolesAreSent(t *testing.T) {
	s := &topicScorer{}
	rig := topicServer(t, topicPolicy("block", ""), s)
	body := `{"model":"m","messages":[{"role":"system","content":"You help bank staff."},{"role":"user","content":"How is the weather today?"}]}`
	if resp, b := postBody(t, rig.url, body); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, b)
	}
	if asked := s.asked(); len(asked) != 1 || asked[0] != "How is the weather today?" {
		t.Fatalf("asked %q, want only the user text", asked)
	}
}

func TestTopics_FailureFollowsOnError(t *testing.T) {
	down := &topicScorer{err: errors.New("rpc error: code = Unavailable desc = TAMGA_TOPICS_MODEL_DIR is not set")}

	resp, body := postBody(t, topicServer(t, topicPolicy("block", ""), down).url, userMessage(creditAsk))
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, "tamga_scan_unavailable") {
		t.Fatalf("on_error block: status = %d, body = %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("X-Tamga-Scan-Degraded"); got != "topics" {
		t.Fatalf("X-Tamga-Scan-Degraded = %q, want topics", got)
	}
	if strings.Contains(body, "TAMGA_TOPICS") {
		t.Fatalf("the error is passed on: %s", body)
	}

	resp, body = postBody(t, topicServer(t, topicPolicy("pass", ""), down).url, userMessage(creditAsk))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("on_error pass: status = %d: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("X-Tamga-Scan-Degraded"); got != "topics" {
		t.Fatalf("on_error pass must still say the scan was degraded, got %q", got)
	}
}

// After five failures the breaker opens: the model is not asked, and the
// request is handled as a failure all the same.
func TestTopics_OpenBreakerIsAFailedScan(t *testing.T) {
	down := &topicScorer{err: errors.New("connection refused")}
	rig := topicServer(t, topicPolicy("block", ""), down)
	for i := 0; i < 5; i++ {
		_, _ = postBody(t, rig.url, userMessage("How is the weather today, number "+string(rune('a'+i))))
	}
	before := len(down.asked())
	resp, _ := postBody(t, rig.url, userMessage("How is the weather tomorrow?"))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if len(down.asked()) != before {
		t.Fatal("the model was asked while the breaker was open")
	}
}

func TestTopics_EnabledButNotConfigured(t *testing.T) {
	resp, body := postBody(t, topicServer(t, topicPolicy("block", ""), nil).url, userMessage("Hello"))
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", resp.StatusCode, body)
	}
	resp, body = postBody(t, topicServer(t, topicPolicy("pass", ""), nil).url, userMessage("Hello"))
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Tamga-Scan-Degraded") != "topics" {
		t.Fatalf("on_error pass: status = %d, degraded = %q: %s", resp.StatusCode, resp.Header.Get("X-Tamga-Scan-Degraded"), body)
	}
}

func TestTopics_OffByDefault(t *testing.T) {
	s := &topicScorer{}
	pol := strings.Replace(topicPolicy("block", ""), "  enabled: true\n", "  enabled: false\n", 1)
	if resp, body := postBody(t, topicServer(t, pol, s).url, userMessage(creditAsk)); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if asked := s.asked(); len(asked) != 0 {
		t.Fatalf("the topic model was asked although the policy does not enable it: %q", asked)
	}
	// And a policy without the section at all.
	plain := "version: \"1.0\"\nproviders:\n  allowed: [openai]\n"
	if resp, _ := postBody(t, topicServer(t, plain, s).url, userMessage(creditAsk)); resp.StatusCode != http.StatusOK || len(s.asked()) != 0 {
		t.Fatalf("status = %d, asked %q", resp.StatusCode, s.asked())
	}
}

// A text over the budget is read from its start, and the response says the
// read was partial.
func TestTopics_TextOverTheBudgetIsReadInPart(t *testing.T) {
	s := &topicScorer{}
	rig := topicServer(t, topicPolicy("block", "  action: BLOCK\n  max_chars: 60"), s)
	long := creditAsk + strings.Repeat(" And some ordinary filler.", 10)
	resp, b := postBody(t, rig.url, userMessage(long))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 from the part that was read: %s", resp.StatusCode, b)
	}
	asked := s.asked()
	if len(asked) != 1 || len(asked[0]) > 60 {
		t.Fatalf("asked %q, want one text of at most 60 bytes", asked)
	}

	s2 := &topicScorer{}
	rig = topicServer(t, topicPolicy("block", "  max_chars: 60"), s2)
	resp, _ = postBody(t, rig.url, userMessage(strings.Repeat("Ordinary filler text. ", 10)))
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Tamga-Topics") != "partial" {
		t.Fatalf("status = %d, X-Tamga-Topics = %q, want 200 and partial", resp.StatusCode, resp.Header.Get("X-Tamga-Topics"))
	}
}

func TestTopicSegments_NewestFirstAndCutAtACharacter(t *testing.T) {
	s := &topicScorer{}
	set, on := mustPolicy(t, topicPolicy("block", "  max_chars: 29")).TopicSettings()
	if !on {
		t.Fatal("topics off")
	}
	newest := strings.Repeat("ç", 12) // 24 bytes
	segs := []extract.Segment{
		{Role: extract.RoleUser, Text: strings.Repeat("ş", 14), Path: "messages[0].content"},
		{Role: extract.RoleUser, Text: newest, Path: "messages[1].content"},
	}
	res := topicSegments(context.Background(), classifier.NewTopicGuard(s), set, "r", segs)
	asked := s.asked()
	// Five bytes are left for the older text: two letters, too few to ask
	// about, and the outcome says it was not read whole.
	if len(asked) != 1 || asked[0] != newest || !res.partial {
		t.Fatalf("asked %q partial=%v", asked, res.partial)
	}
	set, _ = mustPolicy(t, topicPolicy("block", "  max_chars: 49")).TopicSettings()
	s2 := &topicScorer{}
	topicSegments(context.Background(), classifier.NewTopicGuard(s2), set, "r", segs)
	// 25 bytes left: twelve and a half characters, cut to twelve.
	if asked := s2.asked(); len(asked) != 2 || asked[1] != strings.Repeat("ş", 12) {
		t.Fatalf("asked %q", asked)
	}
	if got := cutUTF8("çç", 3); got != "ç" {
		t.Fatalf("cutUTF8 = %q", got)
	}
}

// The scan API judges a text on the proxy path's terms, and tells a proxy
// key the type of a topic finding, not the topic's name.
func TestScan_Topics(t *testing.T) {
	s := &topicScorer{}
	g := newScanRig(t, scanRigOptions{policy: topicPolicy("pass", "  action: BLOCK"), topics: s})
	out, raw := g.scan(t, creditAsk, "")
	if out.Action != "BLOCK" {
		t.Fatalf("action = %q, want BLOCK: %s", out.Action, raw)
	}
	var found bool
	for _, f := range out.Findings {
		if f.Type == "topic" && f.Category == "credit-score-change" && f.Position == "none" && f.Action == "BLOCK" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no topic finding: %s", raw)
	}
	if strings.Contains(raw, "48213") || strings.Contains(raw, "a request to change") {
		t.Fatalf("the response carries the text or the description: %s", raw)
	}
	if status := g.proxyPost(t, "user", creditAsk, ""); status != http.StatusForbidden {
		t.Fatalf("proxy answered %d, want 403", status)
	}

	resp, raw := g.scanRaw(t, map[string]string{"X-Tamga-Key": g.key(t, apikeys.ScopeProxy)}, scanBody(creditAsk, ""))
	if resp.StatusCode != http.StatusOK || strings.Contains(raw, "credit-score-change") || !strings.Contains(raw, `"type":"topic"`) {
		t.Fatalf("proxy key: status = %d: %s", resp.StatusCode, raw)
	}

	// Asked and down: no verdict, whatever on_error says.
	down := &topicScorer{err: errors.New("no topic model")}
	g = newScanRig(t, scanRigOptions{policy: topicPolicy("pass", ""), topics: down})
	if resp, raw := g.scanRaw(t, scanAdmin(), scanBody(creditAsk, "")); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("down: status = %d, want 503: %s", resp.StatusCode, raw)
	}
	g = newScanRig(t, scanRigOptions{policy: topicPolicy("pass", "")})
	if resp, raw := g.scanRaw(t, scanAdmin(), scanBody(creditAsk, "")); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("not configured: status = %d, want 503: %s", resp.StatusCode, raw)
	}
}
