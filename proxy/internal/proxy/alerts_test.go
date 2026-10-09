package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/webhooks"
)

// One rule per action, so each request below gets the decision it is named
// for. %s takes extra lines for the two rules that carry notify URLs.
const alertPolicy = `
version: "1.0"
rules:
  secret_detection:
    action: BLOCK
    sensitivity: low
%s
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email]
%s
  pii:
    action: WARN
    sensitivity: low
    types: [credit_card]
  injection_detection:
    action: STRIP
    sensitivity: low
    applies_to: [tool]
  injection:
    action: BLOCK
    sensitivity: low
    applies_to: [system, user, assistant, tool_definition, request]
scan:
  on_malformed: raw_scan
%s
providers:
  allowed: [openai]
`

const (
	alertSecret = "AKIAIOSFODNN7EXAMPLE"
	alertEmail  = "jane.doe@example.com"
	alertCard   = "4532015112830366"
)

func chatBody(content string) string {
	b, _ := json.Marshal(map[string]any{
		"model":    "gpt-4o",
		"messages": []map[string]string{{"role": "user", "content": content}},
	})
	return string(b)
}

var (
	alertBlockBody  = chatBody("Kindly forward the zebrafish quartz memo using " + alertSecret)
	alertRedactBody = chatBody("Kindly write to " + alertEmail + " about the zebrafish quartz memo")
	alertWarnBody   = chatBody("Kindly charge the zebrafish quartz memo to " + alertCard)
	alertPassBody   = chatBody("Merhaba")
	alertStripBody  = `{"model":"gpt-4o","messages":[
  {"role":"user","content":"Summarise the page I linked."},
  {"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"fetch","arguments":"{}"}}]},
  {"role":"tool","tool_call_id":"c1","content":"Welcome to our site. ` + planted + ` Opening hours: 9 to 5."}
]}`
)

// alertHit is one request an alert receiver saw.
type alertHit struct {
	contentType string
	header      http.Header
	body        string
}

type alertReceiver struct {
	*httptest.Server
	mu   sync.Mutex
	hits []alertHit
}

// newAlertReceiver answers with status. With hang set it does not answer at
// all until the test is over.
func newAlertReceiver(t *testing.T, status int, hang bool) *alertReceiver {
	t.Helper()
	rcv := &alertReceiver{}
	release := make(chan struct{})
	rcv.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rcv.mu.Lock()
		rcv.hits = append(rcv.hits, alertHit{contentType: r.Header.Get("Content-Type"), header: r.Header.Clone(), body: string(b)})
		rcv.mu.Unlock()
		if hang {
			<-release
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(func() {
		close(release)
		rcv.Close()
	})
	return rcv
}

func (r *alertReceiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.hits)
}

func (r *alertReceiver) all() []alertHit {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]alertHit(nil), r.hits...)
}

// actions lists the action of each alert a generic receiver saw.
func (r *alertReceiver) actions(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, h := range r.all() {
		var doc struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal([]byte(h.body), &doc); err != nil {
			t.Fatalf("receiver got a body that is not JSON: %s", h.body)
		}
		out = append(out, doc.Action)
	}
	return out
}

type alertRig struct {
	url   string
	store *webhooks.MemoryStore
	disp  *webhooks.Dispatcher
}

// newAlertRig is the proxy as main wires it for alerts: the dispatcher
// subscribed to the bus for saved destinations, and handed to the handler
// for rule notify URLs. upstreamAnswer is what the provider replies.
func newAlertRig(t *testing.T, policyYAML string, dcfg webhooks.DispatcherConfig, upstreamAnswer string) *alertRig {
	t.Helper()
	if upstreamAnswer == "" {
		upstreamAnswer = `{"choices":[{"message":{"content":"ok"}}]}`
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(upstreamAnswer))
	}))
	u, _ := url.Parse(upstream.URL)
	pol := mustPolicy(t, policyYAML)
	registry := testRegistry()
	registry.Register(scanner.NewJailbreakScanner())

	// The receivers are httptest servers, so the rig allows loopback on
	// purpose. main gives the guard the operator's allowlist instead.
	guard := webhooks.NewGuard(webhooks.GuardConfig{AllowNets: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
	store := webhooks.NewMemoryStore()
	store.UseGuard(guard)
	if dcfg.DefaultOrg == "" {
		dcfg.DefaultOrg = "default"
	}
	if dcfg.Guard == nil {
		dcfg.Guard = guard
	}
	disp := webhooks.NewDispatcher(store, dcfg)
	bus := events.NewBus()
	bus.Subscribe(disp.HandleEvent)
	bus.Start()

	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     registry,
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
		Bus:          bus,
		Alerts:       disp,
	}))
	t.Cleanup(func() {
		srv.Close()
		upstream.Close()
		bus.Stop()
		disp.Stop(200 * time.Millisecond)
	})
	return &alertRig{url: srv.URL, store: store, disp: disp}
}

func plainAlertPolicy() string { return fmt.Sprintf(alertPolicy, "", "", "") }

// send posts a chat request as organisation acme, user alice, and returns
// the status, the request id the proxy gave it, and how long it took.
func (g *alertRig) send(t *testing.T, body string) (int, string, time.Duration) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, g.url+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tamga-Org-Id", "acme")
	req.Header.Set("X-Tamga-User-Id", "alice")
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	took := time.Since(start)
	id := resp.Header.Get("X-Tamga-Request-Id")
	if id == "" {
		t.Fatalf("no X-Tamga-Request-Id on a %d response", resp.StatusCode)
	}
	return resp.StatusCode, id, took
}

func (g *alertRig) add(t *testing.T, w webhooks.Webhook) webhooks.Webhook {
	t.Helper()
	out, err := g.store.Create(w)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (g *alertRig) stats(kind webhooks.Kind) webhooks.KindStats {
	for _, s := range g.disp.Stats() {
		if s.Kind == string(kind) {
			return s
		}
	}
	return webhooks.KindStats{}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// settle gives a delivery that should not happen the time to happen.
func settle() { time.Sleep(250 * time.Millisecond) }

// requestTextIn reports the first piece of a request's text found in s: any
// five consecutive characters of a planted value, a word of the prompt, or
// part of the planted injection.
func requestTextIn(s string) string {
	low := strings.ToLower(s)
	for _, v := range []string{alertSecret, alertEmail, alertCard} {
		for i := 0; i+5 <= len(v); i++ {
			if strings.Contains(s, v[i:i+5]) {
				return v[i : i+5]
			}
		}
	}
	for _, word := range []string{"kindly", "zebrafish", "quartz", "memo", "previous instructions", "reveal the", "opening hours", "merhaba"} {
		if strings.Contains(low, word) {
			return word
		}
	}
	return ""
}

func TestAlerts_BlockReachesEachKindInItsOwnFormat(t *testing.T) {
	g := newAlertRig(t, plainAlertPolicy(), webhooks.DispatcherConfig{}, "")
	generic := newAlertReceiver(t, http.StatusOK, false)
	slack := newAlertReceiver(t, http.StatusOK, false)
	hec := newAlertReceiver(t, http.StatusOK, false)
	pager := newAlertReceiver(t, http.StatusAccepted, false)
	off := newAlertReceiver(t, http.StatusOK, false)

	wGeneric := g.add(t, webhooks.Webhook{Kind: webhooks.KindGeneric, URL: generic.URL, Enabled: true, Headers: map[string]string{"X-Tenant": "soc"}})
	g.add(t, webhooks.Webhook{Kind: webhooks.KindSlack, URL: slack.URL, Enabled: true})
	g.add(t, webhooks.Webhook{Kind: webhooks.KindSplunkHEC, URL: hec.URL, Enabled: true})
	g.add(t, webhooks.Webhook{Kind: webhooks.KindPagerDuty, URL: pager.URL, Enabled: true, AuthToken: "RKEY_1234"})
	wOff := g.add(t, webhooks.Webhook{Kind: webhooks.KindGeneric, URL: off.URL, Enabled: false})

	before := time.Now().UTC().Add(-time.Second)
	status, id, _ := g.send(t, alertBlockBody)
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
	eventually(t, "all four destinations", func() bool {
		return generic.count() == 1 && slack.count() == 1 && hec.count() == 1 && pager.count() == 1
	})

	// generic: the JSON document.
	h := generic.all()[0]
	var doc struct {
		RequestID     string `json:"request_id"`
		OrgID         string `json:"org_id"`
		Provider      string `json:"provider"`
		Model         string `json:"model"`
		Action        string `json:"action"`
		Direction     string `json:"direction"`
		UserID        string `json:"user_id"`
		TS            string `json:"ts"`
		FindingsCount int    `json:"findings_count"`
		Findings      []struct {
			Type, Category, Severity string
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(h.body), &doc); err != nil {
		t.Fatalf("generic body: %v: %s", err, h.body)
	}
	if h.contentType != "application/json" || h.header.Get("X-Tenant") != "soc" {
		t.Errorf("generic headers: %v", h.header)
	}
	if doc.RequestID != id || doc.OrgID != "acme" || doc.Provider != "openai" || doc.Model != "gpt-4o" ||
		doc.Action != "BLOCK" || doc.Direction != "input" || doc.UserID != "alice" {
		t.Errorf("generic document: %s", h.body)
	}
	if _, err := time.Parse(time.RFC3339, doc.TS); err != nil {
		t.Errorf("generic ts %q: %v", doc.TS, err)
	}
	if doc.FindingsCount == 0 || len(doc.Findings) != doc.FindingsCount || doc.Findings[0].Type != "secret" ||
		doc.Findings[0].Category == "" || doc.Findings[0].Severity == "" {
		t.Errorf("generic findings: %s", h.body)
	}

	// slack: one text field a person can read.
	h = slack.all()[0]
	var slackDoc map[string]string
	if err := json.Unmarshal([]byte(h.body), &slackDoc); err != nil || len(slackDoc) != 1 {
		t.Fatalf("slack body: %v: %s", err, h.body)
	}
	if h.contentType != "application/json" || !strings.HasPrefix(slackDoc["text"], "[Tamga] BLOCK on input: secret/") ||
		!strings.Contains(slackDoc["text"], "Request: "+id) || !strings.Contains(slackDoc["text"], "Organisation: acme") {
		t.Errorf("slack: %s %s", h.contentType, h.body)
	}

	// splunk_hec: a CEF line as plain text.
	h = hec.all()[0]
	if h.contentType != "text/plain; charset=utf-8" || !strings.HasPrefix(h.body, "CEF:0|Tamga|Proxy|") ||
		!strings.Contains(h.body, "deviceExternalId="+id) || !strings.Contains(h.body, "act=BLOCK") ||
		!strings.Contains(h.body, "cs5=acme") || !strings.Contains(h.body, "suser=alice") {
		t.Errorf("splunk_hec: %s %s", h.contentType, h.body)
	}

	// pagerduty: Events API v2, the routing key in the body.
	h = pager.all()[0]
	var pd struct {
		RoutingKey  string `json:"routing_key"`
		EventAction string `json:"event_action"`
		DedupKey    string `json:"dedup_key"`
		Payload     struct {
			Summary  string            `json:"summary"`
			Severity string            `json:"severity"`
			Details  map[string]string `json:"custom_details"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(h.body), &pd); err != nil {
		t.Fatalf("pagerduty body: %v: %s", err, h.body)
	}
	if h.contentType != "application/json" || pd.RoutingKey != "RKEY_1234" || pd.EventAction != "trigger" || pd.DedupKey == "" ||
		pd.Payload.Details["request"] != id || !strings.HasPrefix(pd.Payload.Summary, "[Tamga] BLOCK") || pd.Payload.Severity == "info" {
		t.Errorf("pagerduty: %s", h.body)
	}

	// The console's "last fired" follows live deliveries.
	eventually(t, "last fired", func() bool {
		w, _ := g.store.Get(wGeneric.ID)
		return w.LastFired.After(before)
	})
	settle()
	if off.count() != 0 {
		t.Errorf("a disabled destination received %d alerts", off.count())
	}
	if w, _ := g.store.Get(wOff.ID); !w.LastFired.IsZero() {
		t.Error("a disabled destination shows as fired")
	}
	if generic.count() != 1 {
		t.Errorf("one decision, %d alerts", generic.count())
	}
}

// BLOCK, STRIP and WARN go to every enabled destination. PASS goes nowhere.
// REDACT goes only to a destination whose own settings bound how often it
// can fire, here a cooldown.
func TestAlerts_WhichDecisionsAreDelivered(t *testing.T) {
	g := newAlertRig(t, plainAlertPolicy(), webhooks.DispatcherConfig{}, "")
	plain := newAlertReceiver(t, http.StatusOK, false)
	bounded := newAlertReceiver(t, http.StatusOK, false)
	g.add(t, webhooks.Webhook{Kind: webhooks.KindGeneric, URL: plain.URL, Enabled: true})
	g.add(t, webhooks.Webhook{Kind: webhooks.KindGeneric, URL: bounded.URL, Enabled: true, CooldownSecs: 3600})

	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"pass", alertPassBody, http.StatusOK},
		{"redact", alertRedactBody, http.StatusOK},
		{"strip", alertStripBody, http.StatusOK},
		{"warn", alertWarnBody, http.StatusOK},
		{"block", alertBlockBody, http.StatusForbidden},
	} {
		if status, _, _ := g.send(t, tc.body); status != tc.status {
			t.Fatalf("%s: status = %d, want %d", tc.name, status, tc.status)
		}
	}
	eventually(t, "the alerts", func() bool { return plain.count() >= 3 && bounded.count() >= 4 })
	settle()

	count := func(actions []string) map[string]int {
		m := map[string]int{}
		for _, a := range actions {
			m[a]++
		}
		return m
	}
	if got := count(plain.actions(t)); len(got) != 3 || got["STRIP"] != 1 || got["WARN"] != 1 || got["BLOCK"] != 1 {
		t.Errorf("destination without correlation got %v, want one each of STRIP, WARN, BLOCK", got)
	}
	if got := count(bounded.actions(t)); len(got) != 4 || got["REDACT"] != 1 || got["STRIP"] != 1 || got["WARN"] != 1 || got["BLOCK"] != 1 {
		t.Errorf("destination with a cooldown got %v, want one each of REDACT, STRIP, WARN, BLOCK", got)
	}
}

func TestAlerts_RuleNotifyOnBlockAndRedact(t *testing.T) {
	onBlock := newAlertReceiver(t, http.StatusOK, false)
	onRedact := newAlertReceiver(t, http.StatusOK, false)
	notify := func(u string) string {
		return "    notify:\n      - channel: slack\n        webhook: \"" + u + "\"\n"
	}
	g := newAlertRig(t, fmt.Sprintf(alertPolicy, notify(onBlock.URL), notify(onRedact.URL), ""), webhooks.DispatcherConfig{}, "")

	status, blockID, _ := g.send(t, alertBlockBody)
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
	eventually(t, "the BLOCK rule's notify", func() bool { return onBlock.count() == 1 })

	status, redactID, _ := g.send(t, alertRedactBody)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	eventually(t, "the REDACT rule's notify", func() bool { return onRedact.count() == 1 })

	// A request that matched neither rule tells neither URL.
	g.send(t, alertPassBody)
	g.send(t, alertWarnBody)
	settle()
	if onBlock.count() != 1 || onRedact.count() != 1 {
		t.Fatalf("notify counts = %d and %d, want 1 and 1", onBlock.count(), onRedact.count())
	}

	for _, tc := range []struct {
		rcv        *alertReceiver
		id, action string
	}{{onBlock, blockID, "BLOCK"}, {onRedact, redactID, "REDACT"}} {
		h := tc.rcv.all()[0]
		var doc struct {
			Text  string `json:"text"`
			Tamga struct {
				RequestID string `json:"request_id"`
				Action    string `json:"action"`
				OrgID     string `json:"org_id"`
				Model     string `json:"model"`
				UserID    string `json:"user_id"`
			} `json:"tamga"`
		}
		if err := json.Unmarshal([]byte(h.body), &doc); err != nil {
			t.Fatalf("%s notify body: %v: %s", tc.action, err, h.body)
		}
		if h.contentType != "application/json" || doc.Tamga.RequestID != tc.id || doc.Tamga.Action != tc.action ||
			doc.Tamga.OrgID != "acme" || doc.Tamga.Model != "gpt-4o" || doc.Tamga.UserID != "alice" ||
			!strings.HasPrefix(doc.Text, "[Tamga] "+tc.action+" on input") {
			t.Errorf("%s notify: %s", tc.action, h.body)
		}
		if got := requestTextIn(h.body); got != "" {
			t.Errorf("%s notify carries request text %q: %s", tc.action, got, h.body)
		}
	}
	if s := g.stats(webhooks.KindRuleNotify); s.Sent != 2 || s.Failed != 0 {
		t.Errorf("rule notify stats: %+v", s)
	}
}

func TestAlerts_CooldownSuppressesTheSecond(t *testing.T) {
	g := newAlertRig(t, plainAlertPolicy(), webhooks.DispatcherConfig{}, "")
	cooled := newAlertReceiver(t, http.StatusOK, false)
	every := newAlertReceiver(t, http.StatusOK, false)
	g.add(t, webhooks.Webhook{Kind: webhooks.KindSlack, URL: cooled.URL, Enabled: true, CooldownSecs: 3600})
	g.add(t, webhooks.Webhook{Kind: webhooks.KindGeneric, URL: every.URL, Enabled: true})

	_, first, _ := g.send(t, alertBlockBody)
	g.send(t, alertBlockBody)
	// The destination without a cooldown hears of both, which also shows the
	// second decision was published and not lost.
	eventually(t, "both alerts at the destination without a cooldown", func() bool { return every.count() == 2 })
	settle()
	if cooled.count() != 1 {
		t.Fatalf("the destination with a cooldown got %d alerts, want 1", cooled.count())
	}
	if !strings.Contains(cooled.all()[0].body, first) {
		t.Errorf("the alert that got through is not the first: %s", cooled.all()[0].body)
	}
	if s := g.stats(webhooks.KindSlack); s.Sent != 1 || s.Suppressed != 1 {
		t.Errorf("stats: %+v, want 1 sent and 1 suppressed", s)
	}
}

// Receivers that hang or fail cost the caller nothing: the request is
// answered while the delivery is still being attempted.
func TestAlerts_SlowAndFailingReceiversDoNotTouchTheRequest(t *testing.T) {
	dcfg := webhooks.DispatcherConfig{AttemptTimeout: 1500 * time.Millisecond, Backoff: 10 * time.Millisecond}
	hangs := newAlertReceiver(t, http.StatusOK, true)
	hangsOnNotify := newAlertReceiver(t, http.StatusOK, true)
	fails := newAlertReceiver(t, http.StatusInternalServerError, false)
	refuses := newAlertReceiver(t, http.StatusBadRequest, false)
	notify := "    notify:\n      - webhook: \"" + hangsOnNotify.URL + "\"\n"
	g := newAlertRig(t, fmt.Sprintf(alertPolicy, notify, "", ""), dcfg, "")
	g.add(t, webhooks.Webhook{Kind: webhooks.KindGeneric, URL: hangs.URL, Enabled: true})
	g.add(t, webhooks.Webhook{Kind: webhooks.KindSlack, URL: fails.URL, Enabled: true})
	g.add(t, webhooks.Webhook{Kind: webhooks.KindTeams, URL: refuses.URL, Enabled: true})

	status, _, took := g.send(t, alertBlockBody)
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
	// One attempt at the hanging receiver alone lasts 1.5 seconds.
	if took > time.Second {
		t.Fatalf("the blocked request took %v with a hanging receiver", took)
	}
	status, _, took = g.send(t, alertWarnBody)
	if status != http.StatusOK || took > time.Second {
		t.Fatalf("the forwarded request: status %d in %v", status, took)
	}

	eventually(t, "the receivers to be reached", func() bool {
		return hangs.count() >= 1 && hangsOnNotify.count() >= 1 &&
			g.stats(webhooks.KindSlack).Failed == 2 && g.stats(webhooks.KindTeams).Failed == 2
	})
	settle()
	// Two decisions (BLOCK, WARN): a 500 is tried three times each, a 400 once.
	if n := fails.count(); n != 6 {
		t.Errorf("the 500 receiver was tried %d times for 2 alerts, want 6", n)
	}
	if n := refuses.count(); n != 2 {
		t.Errorf("the 400 receiver was tried %d times for 2 alerts, want 2", n)
	}
	if s := g.stats(webhooks.KindSlack); s.Sent != 0 {
		t.Errorf("stats for the 500 receiver: %+v", s)
	}
}

func TestAlerts_FullQueueDropsAndCounts(t *testing.T) {
	// One worker, room for one more, and a receiver that never answers.
	dcfg := webhooks.DispatcherConfig{QueueSize: 1, Workers: 1, AttemptTimeout: 30 * time.Second, MaxAttempts: 1}
	g := newAlertRig(t, plainAlertPolicy(), dcfg, "")
	hangs := newAlertReceiver(t, http.StatusOK, true)
	g.add(t, webhooks.Webhook{Kind: webhooks.KindGeneric, URL: hangs.URL, Enabled: true})

	g.send(t, alertBlockBody)
	eventually(t, "the worker to be stuck", func() bool { return hangs.count() == 1 })

	const more = 8
	var slowest time.Duration
	for i := 0; i < more; i++ {
		status, _, took := g.send(t, alertBlockBody)
		if status != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", status)
		}
		if took > slowest {
			slowest = took
		}
	}
	if slowest > time.Second {
		t.Fatalf("a request took %v with the alert queue full", slowest)
	}
	// One of the eight fits in the queue; the other seven are dropped.
	eventually(t, "the drops to be counted", func() bool { return g.stats(webhooks.KindGeneric).Dropped == more-1 })
	if s := g.stats(webhooks.KindGeneric); s.Sent != 0 || hangs.count() != 1 {
		t.Errorf("stats: %+v, receiver saw %d", s, hangs.count())
	}
}

// A decision on the response is the same kind of event and is delivered the
// same way, marked as output.
func TestAlerts_OutputBlockIsDelivered(t *testing.T) {
	pol := fmt.Sprintf(alertPolicy, "", "", "output_rules:\n  enabled: true\n  block_on: [credit_card]\n")
	g := newAlertRig(t, pol, webhooks.DispatcherConfig{}, `{"choices":[{"message":{"content":"Your card is `+alertCard+`"}}]}`)
	rcv := newAlertReceiver(t, http.StatusOK, false)
	g.add(t, webhooks.Webhook{Kind: webhooks.KindGeneric, URL: rcv.URL, Enabled: true})

	status, id, _ := g.send(t, alertPassBody)
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want the response blocked with 403", status)
	}
	eventually(t, "the output alert", func() bool { return rcv.count() == 1 })
	body := rcv.all()[0].body
	var doc struct {
		RequestID string `json:"request_id"`
		Action    string `json:"action"`
		Direction string `json:"direction"`
		OrgID     string `json:"org_id"`
		Model     string `json:"model"`
		UserID    string `json:"user_id"`
		Findings  []struct{ Category string }
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.RequestID != id || doc.Action != "BLOCK" || doc.Direction != "output" || doc.OrgID != "acme" ||
		doc.Model != "gpt-4o" || doc.UserID != "alice" || len(doc.Findings) == 0 || doc.Findings[0].Category != "credit_card" {
		t.Errorf("output alert: %s", body)
	}
	if got := requestTextIn(body); got != "" {
		t.Errorf("output alert carries response text %q: %s", got, body)
	}
}

// Point 4 of K-4. A request with a secret in it, an address, a card number
// and an injection is sent past every kind of destination and a rule's
// notify URL. Nothing any of them receives, body or header, has any part of
// the secret or of the prompt in it.
func TestAlerts_NoRequestTextReachesAnyDestination(t *testing.T) {
	kinds := []webhooks.Kind{
		webhooks.KindSlack, webhooks.KindTeams, webhooks.KindSplunk, webhooks.KindSplunkHEC,
		webhooks.KindSentinel, webhooks.KindQRadar, webhooks.KindDatadog, webhooks.KindJira,
		webhooks.KindPagerDuty, webhooks.KindOpsgenie, webhooks.KindServiceNow, webhooks.KindGeneric,
	}
	var total atomic.Int32
	var mu sync.Mutex
	seen := map[string][]alertHit{}
	rcv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen[r.URL.Path] = append(seen[r.URL.Path], alertHit{contentType: r.Header.Get("Content-Type"), header: r.Header.Clone(), body: string(b)})
		mu.Unlock()
		total.Add(1)
	}))
	defer rcv.Close()

	notify := func(name string) string {
		return "    notify:\n      - webhook: \"" + rcv.URL + "/notify-" + name + "\"\n"
	}
	g := newAlertRig(t, fmt.Sprintf(alertPolicy, notify("block"), notify("redact"), ""), webhooks.DispatcherConfig{}, "")
	for _, k := range kinds {
		// A cooldown, so REDACT is delivered too and every action is covered.
		g.add(t, webhooks.Webhook{Kind: k, URL: rcv.URL + "/" + string(k), Enabled: true, AuthToken: "tok", CooldownSecs: 3600})
	}

	// Everything at once, on the user's turn, so each finding is in play:
	// masked secret, masked address and card, and the raw injection match.
	everything := chatBody("Kindly forward the zebrafish quartz memo using " + alertSecret + " to " + alertEmail +
		" and charge " + alertCard + ". " + planted)
	if status, _, _ := g.send(t, everything); status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
	g.send(t, alertRedactBody)
	g.send(t, alertWarnBody)
	g.send(t, alertStripBody)

	// 4 decisions x 12 kinds, plus the two rules' notify URLs: the block
	// rule's for the first request, the redact rule's for the first two.
	want := int32(4*len(kinds) + 3)
	eventually(t, "every delivery", func() bool { return total.Load() == want })
	settle()
	if n := total.Load(); n != want {
		t.Fatalf("%d deliveries, want %d", n, want)
	}

	mu.Lock()
	defer mu.Unlock()
	for path, hits := range seen {
		for _, h := range hits {
			if got := requestTextIn(h.body); got != "" {
				t.Errorf("%s: request text %q in body %s", path, got, h.body)
			}
			for name, vals := range h.header {
				if got := requestTextIn(strings.Join(vals, " ")); got != "" {
					t.Errorf("%s: request text %q in header %s", path, got, name)
				}
			}
			if strings.Contains(h.body, `"match"`) {
				t.Errorf("%s: a match field in %s", path, h.body)
			}
		}
	}
	for _, k := range kinds {
		if len(seen["/"+string(k)]) != 4 {
			t.Errorf("%s received %d alerts, want 4", k, len(seen["/"+string(k)]))
		}
	}
}

// The scan API decides and tells nobody. It publishes nothing on the bus,
// which TestScan_LeavesNoRecord holds it to, so a dispatcher on the bus has
// nothing to deliver; and a PASS on the proxy path is not an alert either.
func TestAlerts_PassIsSilent(t *testing.T) {
	g := newAlertRig(t, plainAlertPolicy(), webhooks.DispatcherConfig{}, "")
	rcv := newAlertReceiver(t, http.StatusOK, false)
	g.add(t, webhooks.Webhook{Kind: webhooks.KindGeneric, URL: rcv.URL, Enabled: true, CooldownSecs: 1})
	for i := 0; i < 3; i++ {
		if status, _, _ := g.send(t, alertPassBody); status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
	}
	settle()
	if rcv.count() != 0 || len(g.disp.Stats()) != 0 {
		t.Fatalf("a PASS produced %d alerts, stats %+v", rcv.count(), g.disp.Stats())
	}
}
