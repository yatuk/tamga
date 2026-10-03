package proxy

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
)

// streamUpstream is a provider that answers every request with an OpenAI
// style event stream. reply builds the text from the request it received;
// the text is sent in pieces of pieceSize with gap between them.
type streamUpstream struct {
	mu        sync.Mutex
	bodies    []string
	reply     func(requestBody string) string
	pieceSize int
	gap       time.Duration
}

func (u *streamUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	u.mu.Lock()
	u.bodies = append(u.bodies, string(b))
	u.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	fl := w.(http.Flusher)
	for _, p := range pieces(u.reply(string(b)), u.pieceSize) {
		_, _ = w.Write(openAIChunk(p))
		fl.Flush()
		time.Sleep(u.gap)
	}
	_, _ = w.Write(openAIFinish)
	_, _ = w.Write(openAIDone)
	fl.Flush()
}

func (u *streamUpstream) received() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.bodies...)
}

func streamProxy(t *testing.T, policyYAML string, up *streamUpstream) string {
	t.Helper()
	upstream := httptest.NewServer(up)
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	pol := mustPolicy(t, policyYAML)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// readStream posts body and reads the event stream, returning the wire
// text, the assembled model text and how long the first event took.
func readStream(t *testing.T, base, body string) (wire, text string, first time.Duration, resp *http.Response) {
	t.Helper()
	start := time.Now()
	resp, err := http.Post(base+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	r := bufio.NewReader(resp.Body)
	var all strings.Builder
	for {
		line, err := r.ReadString('\n')
		if first == 0 && strings.HasPrefix(line, "data:") {
			first = time.Since(start)
		}
		all.WriteString(line)
		if err != nil {
			break
		}
	}
	wire = all.String()
	for _, block := range strings.Split(wire, "\n\n") {
		if _, data, _, ok := splitEvent([]byte(block + "\n\n")); ok {
			if ev, ok := extract.ParseStreamEvent(data); ok {
				for _, d := range ev.Deltas {
					text += d.Text
				}
			}
		}
	}
	return wire, text, first, resp
}

const streamRequest = `{"model":"gpt-4o","stream":true,"messages":[{"role":"system","content":"You are a support assistant."},{"role":"user","content":"Who do I contact?"}]}`

// Finding B7 for streams: the policy said redact, and a streamed response
// was either passed whole or cut off.
func TestStreamE2E_RedactsAValueSplitAcrossEvents(t *testing.T) {
	up := &streamUpstream{pieceSize: 5, reply: func(string) string {
		return "You can write to ayse.yilmaz@example.com, and the office is open from nine to five on every weekday of the year."
	}}
	base := streamProxy(t, `
version: "1.0"
output_rules:
  enabled: true
  redact_on: [email]
  block_on: [tc_kimlik]
  streaming:
    enabled: true
providers:
  allowed: [openai]
`, up)
	wire, text, _, resp := readStream(t, base, streamRequest)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Tamga-Stream-Scan") != "enabled" {
		t.Fatalf("status = %d, X-Tamga-Stream-Scan = %q", resp.StatusCode, resp.Header.Get("X-Tamga-Stream-Scan"))
	}
	if strings.Contains(wire, "example.com") || strings.Contains(wire, "yilmaz@") {
		t.Fatalf("the address reached the client:\n%s", wire)
	}
	want := "You can write to [email_REDACTED], and the office is open from nine to five on every weekday of the year."
	if text != want {
		t.Fatalf("\n got %q\nwant %q", text, want)
	}
	if !strings.HasSuffix(strings.TrimSpace(wire), "data: [DONE]") {
		t.Fatalf("the stream did not end normally:\n%s", wire)
	}
}

func TestStreamE2E_BlockedValueEndsTheStream(t *testing.T) {
	up := &streamUpstream{pieceSize: 4, reply: func(string) string {
		return "The customer's TC kimlik number on file is 10000000146 and the account was opened last spring in Ankara."
	}}
	base := streamProxy(t, `
version: "1.0"
output_rules:
  enabled: true
  block_on: [tc_kimlik]
  streaming:
    enabled: true
providers:
  allowed: [openai]
`, up)
	wire, _, _, _ := readStream(t, base, streamRequest)
	if strings.Contains(wire, "100000001") || !strings.Contains(wire, "content_policy_violation") {
		t.Fatalf("the number reached the client or the stream was not ended:\n%s", wire)
	}
	if strings.Contains(wire, "[DONE]") {
		t.Fatalf("a blocked stream must not finish normally:\n%s", wire)
	}
}

// Finding B8: with the vault on, a streamed response was collected whole
// before any of it was sent.
func TestStreamE2E_VaultRestoresWithoutHoldingTheStream(t *testing.T) {
	placeholder := regexp.MustCompile(`\[TAMGA_[A-Z_]+_\d+\]`)
	up := &streamUpstream{pieceSize: 6, gap: 15 * time.Millisecond, reply: func(req string) string {
		// Answer with whatever placeholder the request carried.
		return "Of course. I will send the summary to " + placeholder.FindString(req) + " today, and a copy to the team as well, as you asked earlier."
	}}
	base := streamProxy(t, `
version: "1.0"
rules:
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email]
vault:
  enabled: true
providers:
  allowed: [openai]
`, up)
	body := `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"Send the summary to user@example.com please."}]}`
	wire, text, first, _ := readStream(t, base, body)

	if got := up.received(); len(got) != 1 || strings.Contains(got[0], "user@example.com") || !placeholder.MatchString(got[0]) {
		t.Fatalf("the provider should have received a placeholder: %v", got)
	}
	if strings.Contains(wire, "TAMGA_") {
		t.Fatalf("a placeholder reached the client:\n%s", wire)
	}
	want := "Of course. I will send the summary to user@example.com today, and a copy to the team as well, as you asked earlier."
	if text != want {
		t.Fatalf("\n got %q\nwant %q", text, want)
	}
	// About 20 pieces 15 ms apart: the whole answer takes some 300 ms. The
	// first event has to arrive long before that.
	if first > 150*time.Millisecond {
		t.Fatalf("the first event took %v: the stream was held until it was complete", first)
	}
}

func TestStreamE2E_CanaryDoesNotHoldTheStreamOrChangeTheRequest(t *testing.T) {
	up := &streamUpstream{pieceSize: 6, gap: 15 * time.Millisecond, reply: func(string) string {
		return "Please contact the support desk; they answer within one working day and can escalate to the right team for you."
	}}
	const pol = `
version: "1.0"
canary:
  enabled: true
  block_on_leak: true
providers:
  allowed: [openai]
`
	base := streamProxy(t, pol, up)
	_, text, first, _ := readStream(t, base, streamRequest)
	if !strings.HasPrefix(text, "Please contact the support desk") || !strings.HasSuffix(text, "for you.") {
		t.Fatalf("text = %q", text)
	}
	if first > 150*time.Millisecond {
		t.Fatalf("the first event took %v: the stream was held until it was complete", first)
	}

	// The same request twice gives the provider the same body, so its
	// prompt cache and ours still work.
	readStream(t, base, streamRequest)
	got := up.received()
	if len(got) != 2 || got[0] != got[1] {
		t.Fatalf("the two forwarded bodies differ:\n%s\n%s", got[0], got[1])
	}
	if !strings.Contains(got[0], canaryMarkerPrefix) {
		t.Fatalf("no canary in the forwarded body: %s", got[0])
	}
}

func TestStreamE2E_CanaryLeakInAStreamIsCaught(t *testing.T) {
	up := &streamUpstream{pieceSize: 7, reply: func(req string) string {
		// A model that recites its system prompt, canary included.
		var doc struct {
			Messages []struct{ Role, Content string }
		}
		_ = json.Unmarshal([]byte(req), &doc)
		return "My instructions are: " + doc.Messages[0].Content + " That is everything I was told before this conversation."
	}}
	base := streamProxy(t, `
version: "1.0"
canary:
  enabled: true
  block_on_leak: true
providers:
  allowed: [openai]
`, up)
	wire, _, _, _ := readStream(t, base, streamRequest)
	if strings.Contains(wire, canaryMarkerPrefix) {
		t.Fatalf("the canary token reached the client:\n%s", wire)
	}
	if !strings.Contains(wire, "system_prompt_leak") {
		t.Fatalf("the stream was not ended for the leak:\n%s", wire)
	}
}

// With nothing to do, the proxy does not touch the stream at all.
func TestStreamE2E_UntouchedWhenNothingIsEnabled(t *testing.T) {
	up := &streamUpstream{pieceSize: 9, reply: func(string) string { return "A plain answer with nothing in it worth a second look." }}
	base := streamProxy(t, "version: \"1.0\"\nproviders:\n  allowed: [openai]\n", up)
	wire, _, _, resp := readStream(t, base, streamRequest)
	var want strings.Builder
	for _, p := range pieces("A plain answer with nothing in it worth a second look.", 9) {
		want.Write(openAIChunk(p))
	}
	want.Write(openAIFinish)
	want.Write(openAIDone)
	if wire != want.String() {
		t.Fatalf("the stream was changed:\n got %q\nwant %q", wire, want.String())
	}
	if resp.Header.Get("X-Tamga-Stream-Scan") != "" {
		t.Fatal("X-Tamga-Stream-Scan set although stream scanning is off")
	}
}
