package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/store"
	"github.com/yatuk/tamga/internal/webhooks"
)

// A call over the LiteLLM route is real traffic, and is recorded as a
// request on the proxy path is: its own event on the bus, which the request
// log, the live feed and the alerts take it from.

const bridgeOrg = "00000000-0000-0000-0000-0000000000b1"

// bridgeEvents waits for n events and returns them in the order published.
func (g *scanRig) bridgeEvents(t *testing.T, n int) []events.Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for g.eventCount() < n && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	// Anything that should not have been published gets the time to be.
	time.Sleep(100 * time.Millisecond)
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.events) != n {
		t.Fatalf("%d events published, want %d", len(g.events), n)
	}
	return append([]events.Event(nil), g.events...)
}

func bridgeKey(t *testing.T, g *scanRig) map[string]string {
	t.Helper()
	created, err := g.keys.CreateWith(apikeys.Params{Label: "litellm", Scope: apikeys.ScopeProxy, OrgID: bridgeOrg, Role: "analyst"})
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{"x-api-key": created.RawKey}
}

func TestLiteLLM_RequestCallsAreRecorded(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmToolPolicy})
	key := bridgeKey(t, g)
	post := func(body []byte) string {
		resp, raw := g.litellmRaw(t, key, body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.StatusCode, raw)
		}
		return resp.Header.Get("X-Tamga-Request-Id")
	}

	blockID := post(litellmToolBody("request", []string{"", "marker-qzxj " + litellmTCKNText}, nil, nil))
	redactID := post(litellmToolBody("request", []string{"marker-qzxj write to " + scanEmail}, nil, nil))
	passID := post(litellmToolBody("request", []string{"marker-qzxj " + litellmClean}, nil, nil))
	toolID := post(litellmToolBody("request", []string{litellmClean},
		[]map[string]interface{}{litellmTool("lookup", scanInjection, "x")},
		[]map[string]interface{}{litellmToolCall("lookup", map[string]interface{}{"q": litellmTCKN})}))

	evs := g.bridgeEvents(t, 4)
	want := []struct {
		id, eventType, action string
		// findings is how many at the least: the injection in the tool is
		// reported by more than one scanner.
		findings int
	}{
		{blockID, "request_blocked", "BLOCK", 1},
		{redactID, "request_scanned", "REDACT", 1},
		{passID, "request_scanned", "PASS", 0},
		{toolID, "request_blocked", "BLOCK", 2},
	}
	for i, w := range want {
		e := evs[i]
		if e.RequestID != w.id || e.EventType != w.eventType || e.Action != w.action || len(e.Findings) < w.findings || (w.findings < 2 && len(e.Findings) != w.findings) {
			t.Fatalf("event %d = %s %s %s with %d findings, want %+v", i, e.RequestID, e.EventType, e.Action, len(e.Findings), w)
		}
		if e.Provider != "litellm" || e.Model != "fake-model" || e.Endpoint != "/api/v1/beta/litellm_basic_guardrail_api (request)" {
			t.Errorf("event %d does not say it came over the bridge: provider %q model %q endpoint %q", i, e.Provider, e.Model, e.Endpoint)
		}
		if e.OrgID != bridgeOrg || !e.OrgVerified || e.PolicyName != "litellm-test" || e.PolicyVersion != "3.2" {
			t.Errorf("event %d: org %q verified %v policy %q %q", i, e.OrgID, e.OrgVerified, e.PolicyName, e.PolicyVersion)
		}
		if e.ScanLatencyMs < 0 || e.ScanLatencyMs > e.TotalLatencyMs || e.Timestamp.IsZero() {
			t.Errorf("event %d: latency %v of %v at %v", i, e.ScanLatencyMs, e.TotalLatencyMs, e.Timestamp)
		}
		// Tamga saw no provider, no tokens and no cost, and says so by
		// saying nothing.
		if e.InputTokens != 0 || e.OutputTokens != 0 || e.CostKnown || e.CacheStatus != "" || e.UserID != "" {
			t.Errorf("event %d claims tokens, a cost, a cache or a user", i)
		}
		if len(e.Body) != 0 || e.Direction != "" || e.OutputAction != "" || e.OutputScanned || e.OutputFindings != nil {
			t.Errorf("event %d: a request's event carries a body or an output", i)
		}
	}

	// Where each finding was, in LiteLLM's terms, and in which role.
	if f := evs[0].Findings[0]; f.Path != "texts[1]" || f.Role != "user" || f.Category != "tc_kimlik" || f.ActionTaken != "BLOCK" {
		t.Errorf("the blocked text's finding: %+v", f)
	}
	where := map[string]string{}
	for _, f := range evs[3].Findings {
		where[f.Role] = f.Path
	}
	if where["tool_definition"] != "tools[0].function.description" || where["assistant"] != "tool_calls[0].function.arguments#q" {
		t.Errorf("the tool findings' places: %v", where)
	}
	if evs[0].InputRisk.Percentage == 0 || evs[2].InputRisk.Percentage != 0 {
		t.Errorf("input risk: %d for the block, %d for the pass", evs[0].InputRisk.Percentage, evs[2].InputRisk.Percentage)
	}

	// Nothing of the texts is on an event: the matches are the scanners'
	// masked ones.
	all, err := json.Marshal(evs)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"qzxj", litellmTCKN, scanEmail, "jane", "numaras", "capital of France", "previous instructions", key["x-api-key"]} {
		if strings.Contains(string(all), text) {
			t.Fatalf("the events hold %q: %s", text, all)
		}
	}

	// The live feed's buffer has them, newest first.
	if got := g.recent.Latest(10); len(got) != 4 || got[0].RequestID != toolID {
		t.Fatalf("recent buffer holds %d events", len(got))
	}

	// The row the request log stores, by either way to the table.
	getPolicy := func() *policy.Policy { return nil }
	row, ok := store.RequestLogFromEvent(evs[0], "", getPolicy)
	if !ok {
		t.Fatal("no row for a blocked bridge call")
	}
	if row.RequestID != blockID || row.OrgID != bridgeOrg || row.Provider != "litellm" || row.Model != "fake-model" ||
		row.ActionTaken != "block" || row.FindingsCount != 1 || row.Endpoint != "/api/v1/beta/litellm_basic_guardrail_api (request)" ||
		row.PolicyName != "litellm-test" || row.InputTokens != 0 || row.CostUSD != nil || row.OutputFindings != nil || row.OutputAction != "" {
		t.Fatalf("row: %+v", row)
	}
	if strings.Contains(string(row.Findings), litellmTCKN) || !strings.Contains(string(row.Findings), `"category":"tc_kimlik"`) {
		t.Fatalf("row findings: %s", row.Findings)
	}
	wire, _ := json.Marshal(events.ToV2(evs[0]))
	var v2 events.EventV2
	if err := json.Unmarshal(wire, &v2); err != nil {
		t.Fatal(err)
	}
	overNATS, ok := store.RequestLogFromV2(v2, "", getPolicy)
	if !ok || overNATS.Provider != row.Provider || overNATS.Endpoint != row.Endpoint || overNATS.ActionTaken != row.ActionTaken ||
		string(overNATS.Findings) != string(row.Findings) {
		t.Fatalf("the row built from the NATS envelope differs: %+v", overNATS)
	}
}

// An answer call is an event when something was found or done. LiteLLM calls
// again every few chunks of a streamed answer, and the clean ones are not
// rows.
func TestLiteLLM_AnswerCallsAreRecordedWhenTheyFindSomething(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
	key := bridgeKey(t, g)
	post := func(body []byte) string {
		resp, raw := g.litellmRaw(t, key, body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.StatusCode, raw)
		}
		return resp.Header.Get("X-Tamga-Request-Id")
	}

	post(litellmToolBody("response", []string{"marker-qzxj Paris."}, nil, nil))
	post(litellmToolBody("response", []string{"marker-qzxj Paris, of"}, nil, nil))
	blockID := post(litellmToolBody("response", []string{"", "marker-qzxj " + litellmTCKNText}, nil, nil))
	redactID := post(litellmToolBody("response", []string{"marker-qzxj write to " + scanEmail}, nil, nil))
	callID := post(litellmToolBody("response", nil, nil, []map[string]interface{}{litellmToolCall("crm", map[string]interface{}{"id": litellmTCKN})}))

	evs := g.bridgeEvents(t, 3)
	for i, w := range []struct{ id, eventType, action, path string }{
		{blockID, "request_blocked", "BLOCK", "texts[1]"},
		{redactID, "request_scanned", "REDACT", "texts[0]"},
		{callID, "request_blocked", "BLOCK", "tool_calls[0].function.arguments#id"},
	} {
		e := evs[i]
		if e.RequestID != w.id || e.EventType != w.eventType || e.Action != w.action {
			t.Fatalf("event %d = %s %s %s, want %+v", i, e.RequestID, e.EventType, e.Action, w)
		}
		if e.Direction != "output" || e.OutputAction != w.action || !e.OutputScanned || len(e.OutputFindings) != 1 || len(e.Findings) != 1 {
			t.Errorf("event %d does not say it judged an answer: %+v", i, e)
		}
		if f := e.Findings[0]; f.Path != w.path || f.Role != "assistant" {
			t.Errorf("event %d: finding at %q in role %q", i, f.Path, f.Role)
		}
		if e.Provider != "litellm" || e.Endpoint != "/api/v1/beta/litellm_basic_guardrail_api (response)" || e.OrgID != bridgeOrg {
			t.Errorf("event %d: provider %q endpoint %q org %q", i, e.Provider, e.Endpoint, e.OrgID)
		}
		if e.InputRisk.Percentage != 0 || e.OutputRisk.Percentage == 0 {
			t.Errorf("event %d: the risk is the answer's, got input %d output %d", i, e.InputRisk.Percentage, e.OutputRisk.Percentage)
		}
	}
	all, _ := json.Marshal(evs)
	for _, text := range []string{"qzxj", litellmTCKN, scanEmail, "numaras"} {
		if strings.Contains(string(all), text) {
			t.Fatalf("the events hold %q: %s", text, all)
		}
	}

	row, ok := store.RequestLogFromEvent(evs[0], "", nil)
	if !ok || row.ActionTaken != "block" || row.OutputAction != "block" || row.FindingsCount != 1 ||
		!strings.Contains(string(row.OutputFindings), `"tc_kimlik"`) || len(row.OutputRisk) == 0 {
		t.Fatalf("row of a blocked answer: %+v", row)
	}
}

// A call that was refused, or that got no verdict, is a log line and a
// count, as before. So is a scan: POST /api/v1/scan stores nothing
// (TestScan_LeavesNoRecord).
func TestLiteLLM_RefusedCallsAreNotRecorded(t *testing.T) {
	failing := &spyScanner{name: "failing"}
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy, scanners: []scanner.Scanner{failing}})

	g.litellmRaw(t, nil, litellmBody("request", []string{litellmTCKNText}, nil))                             // 401
	g.litellmRaw(t, litellmAdmin(), []byte(`{"texts":["hello"]}`))                                           // 400
	g.litellmRaw(t, litellmAdmin(), litellmToolBody("request", make([]string, litellmMaxTexts+1), nil, nil)) // 413
	failing.fail.Store(true)
	if resp, _ := g.litellmRaw(t, litellmAdmin(), litellmBody("request", []string{litellmTCKNText}, nil)); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	g.bridgeEvents(t, 0)
}

// The admin key has no organisation: the row goes to the default one, as a
// proxy request without a key does.
func TestLiteLLM_AdminKeyCallIsRecordedForTheDefaultOrg(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
	resp, _ := g.litellmRaw(t, map[string]string{"X-Tamga-Admin-Key": scanAdminKey, "X-Tamga-Org-Id": "someone-else"},
		[]byte(`{"input_type":"request","texts":["`+litellmTCKN+`"],"request_data":{},"model":"gpt 4o <script>"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	e := g.bridgeEvents(t, 1)[0]
	if e.OrgID != "" || e.OrgVerified {
		t.Fatalf("org %q verified %v: the header of an admin call is not an organisation", e.OrgID, e.OrgVerified)
	}
	// A model name that is not one is not recorded.
	if e.Model != "" {
		t.Fatalf("model = %q", e.Model)
	}
	row, ok := store.RequestLogFromEvent(e, bridgeOrg, nil)
	if !ok || row.OrgID != bridgeOrg {
		t.Fatalf("row org = %q, want the default", row.OrgID)
	}
}

func TestLiteLLMModel(t *testing.T) {
	for in, want := range map[string]string{
		"gpt-4o":                               "gpt-4o",
		"anthropic/claude-sonnet-4-5@20250929": "anthropic/claude-sonnet-4-5@20250929",
		"bedrock/us.anthropic.claude-3:0":      "bedrock/us.anthropic.claude-3:0",
		"":                                     "",
		"gpt 4o":                               "",
		"jane.doe@example.com wrote this":      "",
		"gpt-4o\n":                             "",
		strings.Repeat("a", litellmMaxModel):   strings.Repeat("a", litellmMaxModel),
		strings.Repeat("a", litellmMaxModel+1): "",
		"m\u00f6del":                           "",
	} {
		if got := litellmModel(in); got != want {
			t.Errorf("litellmModel(%q) = %q, want %q", in, got, want)
		}
	}
}

// A block over the bridge fires what a block on the proxy path fires: the
// saved destinations, from the event, and the rule's own notify URLs.
func TestLiteLLM_BlocksAlert(t *testing.T) {
	saved := newAlertReceiver(t, http.StatusOK, false)
	onRule := newAlertReceiver(t, http.StatusOK, false)
	guard := webhooks.NewGuard(webhooks.GuardConfig{AllowNets: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
	hooks := webhooks.NewMemoryStore()
	hooks.UseGuard(guard)
	disp := webhooks.NewDispatcher(hooks, webhooks.DispatcherConfig{DefaultOrg: "default", Guard: guard})
	t.Cleanup(func() { disp.Stop(200 * time.Millisecond) })
	if _, err := hooks.Create(webhooks.Webhook{Kind: webhooks.KindGeneric, URL: saved.URL, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	notifying := strings.Replace(litellmPolicy, "    types: [tc_kimlik, credit_card]\n",
		"    types: [tc_kimlik, credit_card]\n    notify:\n      - channel: slack\n        webhook: \""+onRule.URL+"\"\n", 1)
	if notifying == litellmPolicy {
		t.Fatal("the policy was not changed")
	}
	g := newScanRig(t, scanRigOptions{policy: notifying, alerts: disp})
	key := bridgeKey(t, g)

	type doc struct {
		RequestID string `json:"request_id"`
		OrgID     string `json:"org_id"`
		Provider  string `json:"provider"`
		Model     string `json:"model"`
		Action    string `json:"action"`
		Direction string `json:"direction"`
		Findings  []struct{ Type, Category string }
	}
	read := func(h alertHit) doc {
		var d doc
		if err := json.Unmarshal([]byte(h.body), &d); err != nil {
			t.Fatalf("alert body: %v: %s", err, h.body)
		}
		return d
	}

	// A clean call tells nobody.
	g.litellmRaw(t, key, litellmToolBody("request", []string{litellmClean}, nil, nil))
	resp, _ := g.litellmRaw(t, key, litellmToolBody("request", []string{"marker-qzxj " + litellmTCKNText}, nil, nil))
	id := resp.Header.Get("X-Tamga-Request-Id")
	eventually(t, "the alert for the blocked request", func() bool { return saved.count() == 1 && onRule.count() == 1 })

	d := read(saved.all()[0])
	if d.RequestID != id || d.OrgID != bridgeOrg || d.Provider != "litellm" || d.Model != "fake-model" ||
		d.Action != "BLOCK" || d.Direction != "input" || len(d.Findings) != 1 || d.Findings[0].Category != "tc_kimlik" {
		t.Fatalf("alert for the blocked request: %s", saved.all()[0].body)
	}
	if body := onRule.all()[0].body; !strings.Contains(body, id) || strings.Contains(body, litellmTCKN) {
		t.Fatalf("rule notify: %s", body)
	}

	// A blocked answer is told as a decision on the output, once.
	resp, _ = g.litellmRaw(t, key, litellmToolBody("response", []string{"marker-qzxj " + litellmTCKNText}, nil, nil))
	answerID := resp.Header.Get("X-Tamga-Request-Id")
	eventually(t, "the alert for the blocked answer", func() bool { return saved.count() == 2 })
	settle()
	if saved.count() != 2 || onRule.count() != 1 {
		t.Fatalf("alerts: %d to the saved destination and %d to the rule's, want 2 and 1", saved.count(), onRule.count())
	}
	d = read(saved.all()[1])
	if d.RequestID != answerID || d.Action != "BLOCK" || d.Direction != "output" || d.Provider != "litellm" || len(d.Findings) != 1 {
		t.Fatalf("alert for the blocked answer: %s", saved.all()[1].body)
	}
	for _, h := range append(saved.all(), onRule.all()...) {
		for _, text := range []string{"qzxj", litellmTCKN, "numaras"} {
			if strings.Contains(h.body, text) {
				t.Fatalf("an alert holds %q: %s", text, h.body)
			}
		}
	}
}

// The record is published without waiting. A bus whose buffer is full drops
// it and counts the drop; the reply is the same and as fast.
func TestLiteLLM_RecordingDoesNotHoldTheReply(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
	// A bus nobody reads: its buffer fills and every further publish is
	// dropped. The handler keeps the rig's configuration otherwise.
	stuck := events.NewBus()
	g.bridge.scan.cfg.Bus = stuck
	for i := 0; i < 1500; i++ {
		stuck.Publish(events.Event{EventType: "filler"})
	}
	dropped := stuck.Dropped()
	if dropped == 0 {
		t.Fatal("the bus did not fill")
	}

	start := time.Now()
	out, _, raw := g.litellm(t, "request", []string{litellmTCKNText}, nil)
	if out.Action != "BLOCKED" {
		t.Fatalf("the verdict changed with the record lost: %s", raw)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the reply took %v with the bus full", took)
	}
	if stuck.Dropped() != dropped+1 {
		t.Fatalf("dropped = %d, want %d: the lost record is counted", stuck.Dropped(), dropped+1)
	}

	// With no bus at all the route still answers.
	g.bridge.scan.cfg.Bus = nil
	if out, _, raw := g.litellm(t, "request", []string{litellmTCKNText}, nil); out.Action != "BLOCKED" {
		t.Fatalf("without a bus: %s", raw)
	}
}

// quotingScanner fails with what it read in its error, as a careless
// scanner or a remote one would.
type quotingScanner struct{}

func (quotingScanner) Name() string { return "quoting" }
func (quotingScanner) Scan(_ context.Context, content []byte) ([]scanner.Finding, error) {
	return nil, &quotedError{text: string(content)}
}

type quotedError struct{ text string }

func (e *quotedError) Error() string { return "cannot parse: " + e.text }

// A scanner whose error quotes its input: none of the input reaches the
// process log, on the proxy path, the scan API or the LiteLLM route, and
// none of it reaches a caller.
func TestScannerErrorTextIsNotLoggedOnAnyPath(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	prev, prevLevel := log.Logger, zerolog.GlobalLevel()
	log.Logger = zerolog.New(lockedWriter{&mu, &buf})
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	t.Cleanup(func() {
		log.Logger = prev
		zerolog.SetGlobalLevel(prevLevel)
	})

	g := newScanRig(t, scanRigOptions{policy: litellmPolicy, scanners: []scanner.Scanner{quotingScanner{}}})
	const text = "marker-qzxj my card is 4532015112830366"
	chat, _ := json.Marshal(map[string]interface{}{"model": "gpt-4o", "messages": []map[string]string{{"role": "user", "content": text}}})

	var replies []string
	resp := post(t, g.url, "application/json", string(chat))
	proxied, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	replies = append(replies, string(proxied))
	if resp.Header.Get("X-Tamga-Scan-Degraded") != "true" {
		t.Fatalf("the proxy path did not see the scanner fail (status %d)", resp.StatusCode)
	}
	scanBody, _ := json.Marshal(map[string]string{"text": text})
	resp2, raw := g.scanRaw(t, scanAdmin(), scanBody)
	replies = append(replies, raw)
	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("scan API status = %d, want 503", resp2.StatusCode)
	}
	for _, direction := range []string{"request", "response"} {
		resp3, raw := g.litellmRaw(t, litellmAdmin(), litellmToolBody(direction, []string{text},
			[]map[string]interface{}{litellmTool("lookup", "marker-qzxj tool", "x")},
			[]map[string]interface{}{litellmToolCall("lookup", map[string]interface{}{"q": "marker-qzxj argument"})}))
		replies = append(replies, raw)
		if resp3.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("bridge %s status = %d, want 503: %s", direction, resp3.StatusCode, raw)
		}
	}

	mu.Lock()
	logged := buf.String()
	mu.Unlock()
	for _, part := range []string{"qzxj", "4532015112830366", "my card", "cannot parse"} {
		if strings.Contains(logged, part) {
			t.Fatalf("the log holds %q:\n%s", part, logged)
		}
		for _, reply := range replies {
			if strings.Contains(reply, part) {
				t.Fatalf("a reply holds %q: %s", part, reply)
			}
		}
	}
	// What the log does say: which scanner, and what kind of error.
	if n := strings.Count(logged, `"scanner":"quoting"`); n < 4 {
		t.Fatalf("the failing scanner is named %d times, want at least 4:\n%s", n, logged)
	}
	if !strings.Contains(logged, `"error_kind":"*proxy.quotedError"`) {
		t.Fatalf("the log does not give the kind of error:\n%s", logged)
	}
}
