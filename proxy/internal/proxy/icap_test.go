package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/icap"
	"github.com/yatuk/tamga/internal/icap/icaptest"
	"github.com/yatuk/tamga/internal/policy"
)

// These tests are a web proxy talking ICAP to Tamga: icaptest writes the
// messages as Squid writes them (Preview: 0 from the OPTIONS reply, then
// the rest after 100 Continue), and reads the replies.

const icapPolicy = `
version: "1.0"
name: icap-test
rules:
  pii:
    action: BLOCK
    sensitivity: low
    types: [tc_kimlik]
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email]
  secret_detection:
    action: BLOCK
    sensitivity: low
  injection_detection:
    action: STRIP
    sensitivity: low
    applies_to: [tool]
providers:
  allowed: [openai, anthropic, gemini]
`

const icapRoutePolicy = `
version: "1.0"
name: icap-route
rules:
  pii:
    action: ROUTE
    route_to: internal-llm
    sensitivity: low
    types: [tc_kimlik]
providers:
  allowed: [openai]
  pools:
    internal-llm:
      internal: true
      dialect: openai
      model: internal-model-1
      providers:
        - name: vllm-1
          base_url: https://example.invalid:8443
          api_key_env: ICAP_TEST_POOL_API_KEY
          priority: 1
`

const (
	icapTCKN  = "10000000146" // passes the checksum, belongs to no one
	icapEmail = "ayse.yilmaz@example.com"
	icapClean = "What is the capital of France?"
)

type icapRig struct {
	addr   string
	keys   *apikeys.MemoryStore
	cfg    *config.Config
	mu     sync.Mutex
	events []events.Event
}

func newICAPRig(t *testing.T, policyYAML string, tune func(*config.Config)) *icapRig {
	t.Helper()
	g := &icapRig{keys: apikeys.NewMemoryStore(), cfg: &config.Config{}}
	if tune != nil {
		tune(g.cfg)
	}
	bus := events.NewBus()
	bus.Subscribe(func(e events.Event) {
		g.mu.Lock()
		g.events = append(g.events, e)
		g.mu.Unlock()
	})
	bus.Start()
	t.Cleanup(bus.Stop)
	pol := mustPolicy(t, policyYAML)
	cfg := HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		Config:    g.cfg,
		Keys:      g.keys,
		Bus:       bus,
	}
	scan, bridge := NewScanHandlers(cfg, ScanOptions{})
	t.Cleanup(func() {
		_ = scan.(*scanHandler).fallback.Close()
		_ = bridge.(*litellmHandler).fallback.Close()
	})
	h, err := NewICAPHandler(scan)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &icap.Server{Handler: h}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	g.addr = ln.Addr().String()
	return g
}

func (g *icapRig) conn(t *testing.T) *icaptest.Conn {
	t.Helper()
	c, err := icaptest.Dial(g.addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// waitEvents waits for n events and then a little longer, so that one too
// many is seen.
func (g *icapRig) waitEvents(t *testing.T, n int) []events.Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		g.mu.Lock()
		got := len(g.events)
		g.mu.Unlock()
		if got >= n {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.events) != n {
		t.Fatalf("%d events, want %d", len(g.events), n)
	}
	return append([]events.Event(nil), g.events...)
}

// icapPost is the encapsulated head of a POST to rawURL.
func icapPost(rawURL string, body []byte, extra ...string) string {
	u, _ := url.Parse(rawURL)
	head := "POST " + rawURL + " HTTP/1.1\r\nHost: " + u.Host + "\r\nContent-Type: application/json\r\nContent-Length: " + strconv.Itoa(len(body))
	for _, e := range extra {
		head += "\r\n" + e
	}
	return head
}

func openAIChat(role, text string) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"model":    "gpt-4o-mini",
		"messages": []map[string]string{{"role": role, "content": text}},
	})
	return b
}

func (g *icapRig) send(t *testing.T, rawURL string, body []byte, hdr map[string]string, extra ...string) *icaptest.Reply {
	t.Helper()
	rep, err := g.conn(t).Do(icaptest.Request{HTTP: icapPost(rawURL, body, extra...), Body: body, Preview: 0, Chunk: 37, Header: hdr})
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

type icapError struct {
	Error struct {
		Code          string   `json:"code"`
		Message       string   `json:"message"`
		Type          string   `json:"type"`
		RequestID     string   `json:"request_id"`
		FindingsCount int      `json:"findings_count"`
		FindingTypes  []string `json:"finding_types"`
	} `json:"error"`
}

func icapRefused(t *testing.T, rep *icaptest.Reply, status int) icapError {
	t.Helper()
	if rep.Status != 200 || rep.Response == nil {
		t.Fatalf("want an HTTP response, got ICAP %d %q", rep.Status, rep.RawHTTP)
	}
	if rep.Response.StatusCode != status {
		t.Fatalf("HTTP %d, want %d: %s", rep.Response.StatusCode, status, rep.Body)
	}
	if got := rep.Response.Header.Get("Content-Length"); got != strconv.Itoa(len(rep.Body)) {
		t.Errorf("Content-Length %q for %d bytes", got, len(rep.Body))
	}
	var e icapError
	if err := json.Unmarshal(rep.Body, &e); err != nil {
		t.Fatalf("body is not JSON: %q", rep.Body)
	}
	if e.Error.RequestID == "" || rep.Response.Header.Get("X-Tamga-Request-Id") != e.Error.RequestID {
		t.Errorf("request id %q / %q", e.Error.RequestID, rep.Response.Header.Get("X-Tamga-Request-Id"))
	}
	return e
}

func TestICAP_OptionsAdvertisesPreviewAndISTag(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	c := g.conn(t)
	if err := c.Write("OPTIONS icap://" + g.addr + "/reqmod ICAP/1.0\r\nHost: " + g.addr + "\r\nEncapsulated: null-body=0\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	rep, err := c.Read()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != 200 || rep.Header.Get("Methods") != "REQMOD" || rep.Header.Get("Preview") != "0" || rep.Header.Get("Allow") != "204" {
		t.Fatalf("OPTIONS %d %v", rep.Status, rep.Header)
	}
	if tag := rep.Header.Get("ISTag"); !strings.HasPrefix(tag, `"tamga-`) || len(tag) > 34 {
		t.Errorf("ISTag %q", tag)
	}
}

func TestICAP_RedactRewritesAnAPIRequest(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	body := openAIChat("user", "Write to "+icapEmail+" about the invoice.")
	rep := g.send(t, "https://api.openai.com/v1/chat/completions", body,
		map[string]string{"X-Client-Username": "jdoe", "X-Client-IP": "10.0.0.7"}, "Authorization: Bearer sk-test-not-a-real-key")
	if rep.Status != 200 || rep.Request == nil {
		t.Fatalf("want a changed request, got ICAP %d", rep.Status)
	}
	if bytes.Contains(rep.Body, []byte(icapEmail)) || !json.Valid(rep.Body) {
		t.Fatalf("forwarded body %s", rep.Body)
	}
	var doc struct {
		Model    string                     `json:"model"`
		Messages []struct{ Content string } `json:"messages"`
	}
	if err := json.Unmarshal(rep.Body, &doc); err != nil || doc.Model != "gpt-4o-mini" || !strings.Contains(doc.Messages[0].Content, "about the invoice") {
		t.Fatalf("forwarded body %s", rep.Body)
	}
	if got := rep.Request.Header.Get("Content-Length"); got != strconv.Itoa(len(rep.Body)) {
		t.Errorf("Content-Length %q for %d bytes", got, len(rep.Body))
	}
	if rep.Request.Header.Get("Authorization") != "Bearer sk-test-not-a-real-key" || rep.Request.URL.String() != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("request head changed: %q", rep.RawHTTP)
	}
	e := g.waitEvents(t, 1)[0]
	if e.Action != "REDACT" || e.EventType != "request_scanned" || e.Provider != "openai" || e.Endpoint != "icap api.openai.com/v1/*" ||
		e.UserID != "jdoe" || e.Model != "gpt-4o-mini" || e.PolicyName != "icap-test" || len(e.Findings) == 0 {
		t.Fatalf("event %+v", e)
	}
	for _, f := range e.Findings {
		if strings.Contains(f.Match, icapEmail) {
			t.Fatal("the event holds the address")
		}
	}
}

func TestICAP_BlockAnswersWith403(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	body, _ := json.Marshal(map[string]interface{}{
		"model": "claude-sonnet-4-5", "max_tokens": 64,
		"messages": []map[string]string{{"role": "user", "content": "Müşterinin TC kimlik numarası " + icapTCKN}},
	})
	rep := g.send(t, "https://api.anthropic.com/v1/messages", body, nil)
	e := icapRefused(t, rep, 403)
	if e.Error.Code != "tamga_blocked" || e.Error.Type != "security_violation" || e.Error.FindingsCount == 0 || len(e.Error.FindingTypes) != 1 || e.Error.FindingTypes[0] != "pii" {
		t.Fatalf("refusal %+v", e)
	}
	if bytes.Contains(rep.Body, []byte(icapTCKN)) || rep.Response.Header.Get("X-Tamga-Action") != "BLOCK" {
		t.Fatalf("refusal %s %v", rep.Body, rep.Response.Header)
	}
	ev := g.waitEvents(t, 1)[0]
	if ev.Action != "BLOCK" || ev.EventType != "request_blocked" || ev.Provider != "anthropic" || ev.RequestID != e.Error.RequestID {
		t.Fatalf("event %+v", ev)
	}
}

func TestICAP_CleanRequestIs204AndRecorded(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"` + icapClean + `"}]}]}`)
	rep := g.send(t, "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent", body, nil)
	if rep.Status != 204 {
		t.Fatalf("ICAP %d", rep.Status)
	}
	e := g.waitEvents(t, 1)[0]
	if e.Action != "PASS" || e.Provider != "gemini" || e.Model != "gemini-2.5-flash" {
		t.Fatalf("event %+v", e)
	}
}

func TestICAP_UnrecognisedRequestsPassUnread(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	tckn := openAIChat("user", "TC "+icapTCKN)
	for _, u := range []string{
		"https://example.invalid/v1/chat/completions",
		"https://api.openai.com/v1/files",
		"https://api.openai.com.example.invalid/v1/chat/completions",
		"https://openai.azure.com/openai/v1/chat/completions",
	} {
		if rep := g.send(t, u, tckn, nil); rep.Status != 204 {
			t.Errorf("%s: ICAP %d", u, rep.Status)
		}
	}
	// A GET to a known endpoint, with no body.
	rep, err := g.conn(t).Do(icaptest.Request{HTTP: "GET https://api.openai.com/v1/chat/completions HTTP/1.1\r\nHost: api.openai.com", Preview: 0, Header: map[string]string{"Allow": "204"}})
	if err != nil || rep.Status != 204 {
		t.Fatalf("GET: %+v %v", rep, err)
	}
	g.waitEvents(t, 0)
}

func TestICAP_AzureAndMistralAreRead(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	tckn := openAIChat("user", "TC "+icapTCKN)
	for _, u := range []string{
		"https://my-resource.openai.azure.com/openai/deployments/gpt4o/chat/completions?api-version=2024-10-21",
		"https://api.mistral.ai/v1/chat/completions",
	} {
		icapRefused(t, g.send(t, u, tckn, nil), 403)
	}
	evs := g.waitEvents(t, 2)
	if evs[0].Provider != "azure" || evs[0].Endpoint != "icap *.openai.azure.com/openai/*" || evs[1].Provider != "mistral" {
		t.Fatalf("events %+v / %+v", evs[0], evs[1])
	}
}

func TestICAP_StripRewritesAToolResult(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	body, _ := json.Marshal(map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Summarise the page."},
			{"role": "assistant", "tool_calls": []map[string]interface{}{{"id": "call_1", "type": "function", "function": map[string]string{"name": "fetch", "arguments": `{"url":"https://example.invalid/"}`}}}},
			{"role": "tool", "tool_call_id": "call_1", "content": planted},
		},
	})
	rep := g.send(t, "https://api.openai.com/v1/chat/completions", body, nil)
	if rep.Status != 200 || rep.Request == nil {
		t.Fatalf("want a changed request, got ICAP %d %s", rep.Status, rep.Body)
	}
	if bytes.Contains(rep.Body, []byte("Ignore all previous instructions")) || !bytes.Contains(rep.Body, []byte("Summarise the page.")) {
		t.Fatalf("forwarded %s", rep.Body)
	}
	if e := g.waitEvents(t, 1)[0]; e.Action != "STRIP" {
		t.Fatalf("event %+v", e)
	}
}

func TestICAP_RouteIsABlock(t *testing.T) {
	t.Cleanup(policy.SetUpstreamRules(&policy.UpstreamRules{AllowHostPorts: []string{"example.invalid:8443"}}))
	g := newICAPRig(t, icapRoutePolicy, nil)
	rep := g.send(t, "https://api.openai.com/v1/chat/completions", openAIChat("user", "TC "+icapTCKN), nil)
	icapRefused(t, rep, 403)
	if rep.Response.Header.Get("X-Tamga-Route-Fallback") != "block:icap" {
		t.Fatalf("headers %v", rep.Response.Header)
	}
	if e := g.waitEvents(t, 1)[0]; e.Action != "BLOCK" || e.RoutePool != "internal-llm" {
		t.Fatalf("event %+v", e)
	}
}

func TestICAP_WebChatIsReadAndNeverRewritten(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	// The shape chatgpt.com is believed to send. Not verified.
	chat := func(text string) []byte {
		b, _ := json.Marshal(map[string]interface{}{
			"action": "next",
			"messages": []map[string]interface{}{{
				"id": "aaa1", "author": map[string]string{"role": "user"},
				"content": map[string]interface{}{"content_type": "text", "parts": []string{text}},
			}},
			"model": "auto",
		})
		return b
	}
	if rep := g.send(t, "https://chatgpt.com/backend-api/conversation", chat(icapClean), nil); rep.Status != 204 {
		t.Fatalf("clean: ICAP %d", rep.Status)
	}
	// A REDACT on a body Tamga cannot rewrite with confidence is a BLOCK.
	rep := g.send(t, "https://chatgpt.com/backend-api/f/conversation", chat("Mail "+icapEmail), nil)
	icapRefused(t, rep, 403)
	if rep.Response.Header.Get("X-Tamga-Redact-Fallback") != "block:unverified_format" {
		t.Fatalf("headers %v", rep.Response.Header)
	}
	// claude.ai, with the identifiers in the path left out of the record.
	claude, _ := json.Marshal(map[string]interface{}{"prompt": "TC " + icapTCKN, "timezone": "Europe/Istanbul"})
	icapRefused(t, g.send(t, "https://claude.ai/api/organizations/0000-org/chat_conversations/0000-conv/completion", claude, nil), 403)
	evs := g.waitEvents(t, 3)
	if evs[2].Endpoint != "icap claude.ai/api/organizations/*/chat_conversations/*/completion" || evs[2].Provider != "anthropic" {
		t.Fatalf("event %+v", evs[2])
	}
}

func TestICAP_GeminiWebFormIsRead(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	// f.req holds JSON with more JSON in a string, as the Gemini web app is
	// believed to send it. Not verified. "at" is a token and is not read.
	inner, _ := json.Marshal([]interface{}{[]interface{}{"TC kimlik " + icapTCKN, 0, nil}, nil, nil})
	outer, _ := json.Marshal([]interface{}{nil, string(inner)})
	form := url.Values{"f.req": {string(outer)}, "at": {"AJvLN6N-not-a-real-token:1700000000000"}}.Encode()
	head := "POST https://gemini.google.com/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate?bl=x&rt=c HTTP/1.1\r\n" +
		"Host: gemini.google.com\r\nContent-Type: application/x-www-form-urlencoded;charset=UTF-8\r\nContent-Length: " + strconv.Itoa(len(form))
	rep, err := g.conn(t).Do(icaptest.Request{HTTP: head, Body: []byte(form), Preview: 0})
	if err != nil {
		t.Fatal(err)
	}
	icapRefused(t, rep, 403)
	if e := g.waitEvents(t, 1)[0]; e.Provider != "gemini" || len(e.Findings) == 0 || !strings.HasPrefix(e.Findings[0].Path, "f.req[0]") {
		t.Fatalf("event %+v", e)
	}
}

func TestICAP_BodyLimits(t *testing.T) {
	g := newICAPRig(t, icapPolicy, func(c *config.Config) { c.MaxBodyBytes = 2048 })
	big := openAIChat("user", strings.Repeat("lorem ipsum ", 1000))
	rep := g.send(t, "https://api.openai.com/v1/chat/completions", big, nil)
	if e := icapRefused(t, rep, 413); e.Error.Code != "body_too_large" {
		t.Fatalf("refusal %+v", e)
	}
	// Without a preview, the whole body at once.
	rep, err := g.conn(t).Do(icaptest.Request{HTTP: icapPost("https://api.openai.com/v1/chat/completions", big), Body: big, Preview: -1, Header: map[string]string{"Allow": "204"}})
	if err != nil {
		t.Fatal(err)
	}
	icapRefused(t, rep, 413)
	// A gzip body is limited by what it decodes to.
	var zb bytes.Buffer
	zw := gzip.NewWriter(&zb)
	_, _ = zw.Write(big)
	_ = zw.Close()
	icapRefused(t, g.send(t, "https://api.openai.com/v1/chat/completions", zb.Bytes(), nil, "Content-Encoding: gzip"), 413)
}

func TestICAP_GzipBodyIsDecodedAndSentPlain(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	var zb bytes.Buffer
	zw := gzip.NewWriter(&zb)
	_, _ = zw.Write(openAIChat("user", "Mail "+icapEmail))
	_ = zw.Close()
	rep := g.send(t, "https://api.openai.com/v1/chat/completions", zb.Bytes(), nil, "Content-Encoding: gzip")
	if rep.Status != 200 || rep.Request == nil || bytes.Contains(rep.Body, []byte(icapEmail)) || !json.Valid(rep.Body) {
		t.Fatalf("ICAP %d %q", rep.Status, rep.Body)
	}
	if rep.Request.Header.Get("Content-Encoding") != "" {
		t.Fatal("Content-Encoding kept on a body sent plain")
	}
	// An encoding Tamga does not decode is not let through unread.
	icapRefused(t, g.send(t, "https://api.openai.com/v1/chat/completions", []byte("\x00\x01binary"), nil, "Content-Encoding: br"), 415)
}

func TestICAP_MalformedJSONIsRefused(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	dup := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}],"messages":[{"role":"user","content":"TC ` + icapTCKN + `"}]}`)
	e := icapRefused(t, g.send(t, "https://api.openai.com/v1/chat/completions", dup, nil), 400)
	if e.Error.Code != "tamga_invalid_json" {
		t.Fatalf("refusal %+v", e)
	}
	// With on_malformed: raw_scan it is read generically, and blocked or
	// passed, never rewritten.
	g2 := newICAPRig(t, icapPolicy+"scan:\n  on_malformed: raw_scan\n", nil)
	icapRefused(t, g2.send(t, "https://api.openai.com/v1/chat/completions", dup, nil), 403)
	// The value a decoder would drop is read too.
	first := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"TC ` + icapTCKN + `"}],"messages":[{"role":"user","content":"hi"}]}`)
	icapRefused(t, g2.send(t, "https://api.openai.com/v1/chat/completions", first, nil), 403)
	dupMail := bytes.Replace(dup, []byte("TC "+icapTCKN), []byte(icapEmail), 1)
	rep := g2.send(t, "https://api.openai.com/v1/chat/completions", dupMail, nil)
	icapRefused(t, rep, 403)
	if rep.Response.Header.Get("X-Tamga-Redact-Fallback") != "block:malformed_json" {
		t.Fatalf("headers %v", rep.Response.Header)
	}
}

func TestICAP_HTMLRefusalForABrowser(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	rep := g.send(t, "https://api.openai.com/v1/chat/completions", openAIChat("user", "TC "+icapTCKN), nil, "Accept: text/html,application/xhtml+xml")
	if rep.Response == nil || rep.Response.StatusCode != 403 || !strings.HasPrefix(rep.Response.Header.Get("Content-Type"), "text/html") ||
		!bytes.Contains(rep.Body, []byte("Stopped by Tamga")) {
		t.Fatalf("%+v %s", rep.Response, rep.Body)
	}
}

func TestICAP_TamgaKeyFromTheWebProxy(t *testing.T) {
	g := newICAPRig(t, icapPolicy, func(c *config.Config) { c.RequireKey = true })
	body := openAIChat("user", icapClean)
	// TAMGA_REQUIRE_KEY on and no key: no verdict.
	if e := icapRefused(t, g.send(t, "https://api.openai.com/v1/chat/completions", body, nil), 503); e.Error.Code != "tamga_icap_key" {
		t.Fatalf("refusal %+v", e)
	}
	icapRefused(t, g.send(t, "https://api.openai.com/v1/chat/completions", body, map[string]string{"X-Tamga-Key": "tk_not_a_key"}), 503)
	k, err := g.keys.CreateWith(apikeys.Params{Label: "squid", Scope: apikeys.ScopeProxy, OrgID: "00000000-0000-0000-0000-0000000000c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rep := g.send(t, "https://api.openai.com/v1/chat/completions", body, map[string]string{"X-Tamga-Key": k.RawKey}); rep.Status != 204 {
		t.Fatalf("with a key: ICAP %d", rep.Status)
	}
	e := g.waitEvents(t, 1)[0]
	if e.OrgID != "00000000-0000-0000-0000-0000000000c1" || !e.OrgVerified {
		t.Fatalf("event %+v", e)
	}
}

func TestICAP_KeepAliveAcrossVerdicts(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	c := g.conn(t)
	endpoint := "https://api.openai.com/v1/chat/completions"
	for i, tc := range []struct {
		body   []byte
		status int
	}{
		{openAIChat("user", icapClean), 204},
		{openAIChat("user", "TC "+icapTCKN), 200},
		{openAIChat("user", icapEmail), 200},
		{openAIChat("user", icapClean), 204},
	} {
		rep, err := c.Do(icaptest.Request{HTTP: icapPost(endpoint, tc.body), Body: tc.body, Preview: 0, Chunk: 5})
		if err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		if rep.Status != tc.status {
			t.Fatalf("%d: ICAP %d, want %d", i, rep.Status, tc.status)
		}
	}
	g.waitEvents(t, 4)
}

func TestMatchICAP(t *testing.T) {
	for raw, want := range map[string]string{
		"https://API.OpenAI.com./v1/responses":                                   "openai",
		"https://api.openai.com:443/v1/completions":                              "openai",
		"https://api.anthropic.com/v1/messages/count_tokens":                     "anthropic",
		"https://generativelanguage.googleapis.com/v1/models/x:countTokens":      "gemini",
		"https://generativelanguage.googleapis.com/v1beta/models/x:embedContent": "",
		"https://chat.openai.com/backend-api/conversation":                       "openai",
		"https://chatgpt.com/backend-api/conversations":                          "",
		"https://claude.ai/api/organizations/a/chat_conversations/b/title":       "",
		"https://evil.invalid/api.openai.com/v1/chat/completions":                "",
	} {
		u, _ := url.Parse(raw)
		got := ""
		if rt := matchICAP(&http.Request{Method: http.MethodPost, URL: u, Host: u.Host}); rt != nil {
			got = rt.provider
		}
		if got != want {
			t.Errorf("%s: %q, want %q", raw, got, want)
		}
	}
}

func TestICAP_WarnPassesAndIsRecorded(t *testing.T) {
	g := newICAPRig(t, `
version: "1.0"
name: icap-warn
rules:
  pii:
    action: WARN
    sensitivity: low
    types: [tc_kimlik]
providers:
  allowed: [openai]
`, nil)
	if rep := g.send(t, "https://api.openai.com/v1/chat/completions", openAIChat("user", "TC "+icapTCKN), nil); rep.Status != 204 {
		t.Fatalf("ICAP %d", rep.Status)
	}
	if e := g.waitEvents(t, 1)[0]; e.Action != "WARN" || len(e.Findings) == 0 {
		t.Fatalf("event %+v", e)
	}
}

func TestICAP_MalformedMessageIsAnICAPError(t *testing.T) {
	g := newICAPRig(t, icapPolicy, nil)
	c := g.conn(t)
	h := "POST https://api.openai.com/v1/chat/completions HTTP/1.1\r\nHost: api.openai.com\r\n\r\n"
	// A chunk that is longer than its size says.
	raw := "REQMOD icap://" + g.addr + "/reqmod ICAP/1.0\r\nHost: " + g.addr + "\r\nEncapsulated: req-hdr=0, req-body=" + strconv.Itoa(len(h)) + "\r\n\r\n" + h + "2\r\n{}}\r\n0\r\n\r\n"
	if err := c.Write(raw); err != nil {
		t.Fatal(err)
	}
	rep, err := c.Read()
	if err != nil || rep.Status != 400 {
		t.Fatalf("%+v %v", rep, err)
	}
	g.waitEvents(t, 0)
}
