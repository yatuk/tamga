package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// These tests call the route as LiteLLM does: the request has every field
// LiteLLM's GenericGuardrailAPIRequest has, and the reply is read the way
// GenericGuardrailAPIResponse.from_dict reads it. The caller is this file,
// not a LiteLLM process.

const litellmPath = "/api/v1/beta/litellm_basic_guardrail_api"

// A request is judged by the rules, an answer by output_rules. The two
// disagree about a secret on purpose: the rules block it, output_rules does
// not name it.
const litellmPolicy = `
version: "3.2"
name: litellm-test
rules:
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email, phone_tr, iban]
  pii:
    action: BLOCK
    sensitivity: low
    types: [tc_kimlik, credit_card]
  secret_detection:
    action: BLOCK
    sensitivity: low
  injection_detection:
    action: BLOCK
    sensitivity: low
    applies_to: [tool]
output_rules:
  enabled: true
  redact_on: [email]
  block_on: [tc_kimlik]
providers:
  allowed: [openai]
`

const (
	// litellmTCKN passes the TCKN checksum and belongs to no one.
	litellmTCKN     = "10000000146"
	litellmTCKNText = "Müşterinin TC kimlik numarası " + litellmTCKN + " olarak kayıtlı."
	litellmClean    = "What is the capital of France?"
)

// litellmReply is a reply as LiteLLM reads it: action defaults to NONE, and
// the other fields to nothing.
type litellmReply struct {
	Action        string   `json:"action"`
	BlockedReason string   `json:"blocked_reason"`
	Texts         []string `json:"texts"`
}

type litellmFailure struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

func litellmAdmin() map[string]string { return map[string]string{"X-Tamga-Admin-Key": scanAdminKey} }

// litellmBody is the body LiteLLM posts: every field of its request model.
// structured is the conversation in OpenAI's shape, or nil.
func litellmBody(inputType string, texts []string, structured []map[string]interface{}) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"input_type":       inputType,
		"litellm_call_id":  "c3f1a8e2-7b1d-4f0e-9d53-1f2a6b7c8d90",
		"litellm_trace_id": "5a0b6c2e-90f4-4d1c-8a3e-0c9e2b7d1f44",
		"texts":            texts,
		"request_data": map[string]interface{}{
			"user_api_key_hash":        "88dc28d0f030c55ed4ab77ed8faf098196cb1c05df778539800c9f1243fe6b4b",
			"user_api_key_alias":       "team-a-key",
			"user_api_key_user_id":     "user-17",
			"user_api_key_user_email":  nil,
			"user_api_key_team_id":     "team-a",
			"user_api_key_team_alias":  nil,
			"user_api_key_end_user_id": nil,
			"user_api_key_org_id":      nil,
		},
		"request_headers":                     map[string]string{"content-type": "application/json", "authorization": "[present]"},
		"litellm_version":                     "1.104.2",
		"images":                              nil,
		"tools":                               []map[string]interface{}{{"type": "function", "function": map[string]interface{}{"name": "lookup"}}},
		"structured_messages":                 structured,
		"tool_calls":                          nil,
		"additional_provider_specific_params": map[string]interface{}{},
		"model":                               "fake-model",
	})
	return b
}

// litellmRaw posts body to the route as JSON with the given headers. A
// header set to "-" is removed, and "?" is the query string.
func (g *scanRig) litellmRaw(t *testing.T, headers map[string]string, body []byte) (*http.Response, string) {
	t.Helper()
	path := litellmPath
	if q := headers["?"]; q != "" {
		path += "?" + q
	}
	req, err := http.NewRequest(http.MethodPost, g.url+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		switch {
		case k == "?":
		case v == "-":
			req.Header.Del(k)
		default:
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// litellm calls the route with the admin key and decodes a 200. It also
// holds every reply to the contract: no field LiteLLM does not know, texts
// only with GUARDRAIL_INTERVENED, a reason only with BLOCKED.
func (g *scanRig) litellm(t *testing.T, inputType string, texts []string, structured []map[string]interface{}) (litellmReply, *http.Response, string) {
	t.Helper()
	resp, raw := g.litellmRaw(t, litellmAdmin(), litellmBody(inputType, texts, structured))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, raw)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatalf("reply is not JSON: %v: %s", err, raw)
	}
	for k := range fields {
		if k != "action" && k != "blocked_reason" && k != "texts" {
			t.Fatalf("reply has field %q, which is not in the contract: %s", k, raw)
		}
	}
	var out litellmReply
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	switch out.Action {
	case "NONE":
		if out.Texts != nil || out.BlockedReason != "" {
			t.Fatalf("NONE with texts or a reason: %s", raw)
		}
	case "BLOCKED":
		if out.Texts != nil || out.BlockedReason == "" {
			t.Fatalf("BLOCKED needs a reason and no texts: %s", raw)
		}
	case "GUARDRAIL_INTERVENED":
		if len(out.Texts) != len(texts) || out.BlockedReason != "" {
			t.Fatalf("GUARDRAIL_INTERVENED needs one text for each one sent (%d) and no reason: %s", len(texts), raw)
		}
	default:
		t.Fatalf("action %q is not in the contract: %s", out.Action, raw)
	}
	return out, resp, raw
}

func TestLiteLLM_RequestWithTCKNIsBlocked(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
	out, resp, raw := g.litellm(t, "request", []string{"You are a helpful assistant.", litellmTCKNText}, nil)

	if out.Action != "BLOCKED" {
		t.Fatalf("action = %q, want BLOCKED: %s", out.Action, raw)
	}
	// The reason names the kind of finding. LiteLLM passes it on to its own
	// caller, so it holds nothing of the number, masked or not.
	if !strings.Contains(out.BlockedReason, "pii/tc_kimlik") {
		t.Fatalf("blocked_reason does not name the finding: %q", out.BlockedReason)
	}
	for i := 0; i+4 <= len(litellmTCKN); i++ {
		if strings.Contains(raw, litellmTCKN[i:i+4]) {
			t.Fatalf("reply contains %q of the number: %s", litellmTCKN[i:i+4], raw)
		}
	}
	if strings.Contains(raw, "kimlik numarası") || strings.Contains(raw, "helpful") {
		t.Fatalf("reply echoes the request: %s", raw)
	}
	id := resp.Header.Get("X-Tamga-Request-Id")
	if id == "" || !strings.Contains(out.BlockedReason, id) {
		t.Fatalf("blocked_reason does not carry the request id %q: %q", id, out.BlockedReason)
	}
	if got := resp.Header.Get("X-Tamga-Action"); got != "BLOCK" {
		t.Fatalf("X-Tamga-Action = %q, want BLOCK", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	// And the proxy path blocks the same message.
	if status := g.proxyPost(t, "user", litellmTCKNText, ""); status != http.StatusForbidden {
		t.Fatalf("proxy answered %d, want 403", status)
	}
}

func TestLiteLLM_RequestRedactReturnsMaskedTexts(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy + "vault:\n  enabled: true\n"})
	texts := []string{
		"You are a helpful assistant.",
		"",
		"Please write to " + scanEmail + " about the invoice.",
		"Or to john@example.org, \"quoted\" <b> café.",
	}
	out, resp, raw := g.litellm(t, "request", texts, nil)

	if out.Action != "GUARDRAIL_INTERVENED" {
		t.Fatalf("action = %q, want GUARDRAIL_INTERVENED: %s", out.Action, raw)
	}
	want := []string{
		texts[0],
		"",
		"Please write to [email_REDACTED] about the invoice.",
		"Or to [email_REDACTED], \"quoted\" <b> café.",
	}
	for i := range want {
		if out.Texts[i] != want[i] {
			t.Fatalf("texts[%d] = %q, want %q", i, out.Texts[i], want[i])
		}
	}
	if got := resp.Header.Get("X-Tamga-Action"); got != "REDACT" {
		t.Fatalf("X-Tamga-Action = %q, want REDACT", got)
	}
	// The markers are the scan API's, text for text.
	for i, text := range texts {
		if text == "" {
			continue
		}
		verdict, _ := g.scan(t, text, "user")
		predicted := text
		if verdict.RedactedText != nil {
			predicted = *verdict.RedactedText
		}
		if out.Texts[i] != predicted {
			t.Fatalf("texts[%d] = %q, the scan API says %q", i, out.Texts[i], predicted)
		}
	}
	// With the vault on the proxy keeps the address to put back in the
	// answer. Nothing can be put back over the bridge, so nothing is kept.
	if n := g.vault.sets.Load(); n != 0 {
		t.Fatalf("%d vault writes, want 0", n)
	}
}

func TestLiteLLM_CleanRequestPasses(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
	cases := map[string][]string{
		"one text":    {litellmClean},
		"empty texts": {"", ""},
		"no texts":    {},
	}
	for name, texts := range cases {
		t.Run(name, func(t *testing.T) {
			out, resp, raw := g.litellm(t, "request", texts, nil)
			if out.Action != "NONE" {
				t.Fatalf("action = %q, want NONE: %s", out.Action, raw)
			}
			if strings.Contains(raw, "texts") || strings.Contains(raw, "blocked_reason") {
				t.Fatalf("NONE carries more than the action: %s", raw)
			}
			if got := resp.Header.Get("X-Tamga-Action"); got != "PASS" {
				t.Fatalf("X-Tamga-Action = %q, want PASS", got)
			}
		})
	}
	// "texts": null, as LiteLLM's model allows.
	resp, raw := g.litellmRaw(t, litellmAdmin(), []byte(`{"input_type":"response","texts":null,"request_data":{}}`))
	if resp.StatusCode != http.StatusOK || !strings.Contains(raw, `"action":"NONE"`) {
		t.Fatalf("null texts: status = %d: %s", resp.StatusCode, raw)
	}
}

// The answer of a model goes by output_rules, as on the proxy path, and by
// nothing else.
func TestLiteLLM_ResponseHook(t *testing.T) {
	cases := []struct {
		name   string
		policy string
		texts  []string
		action string
		texts2 []string // what comes back, for GUARDRAIL_INTERVENED
		reason string
	}{
		{name: "tckn in block_on", policy: litellmPolicy,
			texts:  []string{"Kayıtlara göre numara " + litellmTCKN + " olarak görünüyor."},
			action: "BLOCKED", reason: "Response blocked by Tamga output policy: pii/tc_kimlik."},
		{name: "email in redact_on", policy: litellmPolicy,
			texts:  []string{"Sure.", "You can reach her at " + scanEmail + "."},
			action: "GUARDRAIL_INTERVENED", texts2: []string{"Sure.", "You can reach her at [email_REDACTED]."}},
		{name: "blocked and redacted together", policy: litellmPolicy,
			texts:  []string{"Write to " + scanEmail + ".", "The number is " + litellmTCKN + "."},
			action: "BLOCKED", reason: "pii/tc_kimlik"},
		{name: "clean", policy: litellmPolicy, texts: []string{"Paris."}, action: "NONE"},
		// The rules block a secret in a request. output_rules does not name
		// it, so an answer holding one is sent, as the proxy would send it.
		{name: "a kind only the input rules name", policy: litellmPolicy,
			texts: []string{"The key is " + scanSecret + "."}, action: "NONE"},
		// The proxy does not read answers at all without output_rules.
		{name: "output_rules off", policy: scanPolicy,
			texts: []string{"The number is " + litellmTCKN + ", write to " + scanEmail + "."}, action: "NONE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newScanRig(t, scanRigOptions{policy: tc.policy})
			out, _, raw := g.litellm(t, "response", tc.texts, nil)
			if out.Action != tc.action {
				t.Fatalf("action = %q, want %q: %s", out.Action, tc.action, raw)
			}
			if !strings.Contains(out.BlockedReason, tc.reason) {
				t.Fatalf("blocked_reason = %q, want it to contain %q", out.BlockedReason, tc.reason)
			}
			for i := range tc.texts2 {
				if out.Texts[i] != tc.texts2[i] {
					t.Fatalf("texts[%d] = %q, want %q", i, out.Texts[i], tc.texts2[i])
				}
			}
			if out.Action != "GUARDRAIL_INTERVENED" && (strings.Contains(raw, litellmTCKN) || strings.Contains(raw, scanEmail)) {
				t.Fatalf("reply echoes a value: %s", raw)
			}
		})
	}
}

// The same answer, through the proxy: what the bridge says is what the proxy
// does with it.
func TestLiteLLM_ResponseHookAgreesWithTheProxyPath(t *testing.T) {
	answers := map[string]string{
		"tckn":  "Kayıtlara göre numara " + litellmTCKN + " olarak görünüyor.",
		"email": "You can reach her at " + scanEmail + ".",
		"clean": "Paris.",
	}
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
	for name, answer := range answers {
		t.Run(name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"choices": []map[string]interface{}{{"index": 0, "message": map[string]string{"role": "assistant", "content": answer}}},
			})
			proxyURL := outputServer(t, litellmPolicy, string(body))
			resp, sent := ask(t, proxyURL, "/v1/chat/completions")

			out, _, raw := g.litellm(t, "response", []string{answer}, nil)
			switch out.Action {
			case "BLOCKED":
				if resp.StatusCode != http.StatusForbidden {
					t.Fatalf("bridge says BLOCKED, proxy answered %d", resp.StatusCode)
				}
			case "GUARDRAIL_INTERVENED":
				if resp.StatusCode != http.StatusOK || !strings.Contains(sent, strings.Trim(mustJSON(t, out.Texts[0]), `"`)) {
					t.Fatalf("bridge returns %q, proxy sent (%d) %s", out.Texts[0], resp.StatusCode, sent)
				}
			case "NONE":
				if resp.StatusCode != http.StatusOK || !strings.Contains(sent, answer) {
					t.Fatalf("bridge says NONE, proxy sent (%d) %s: %s", resp.StatusCode, sent, raw)
				}
			}
		})
	}
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// applies_to is honoured when LiteLLM says which message a text came from.
func TestLiteLLM_RolesComeFromStructuredMessages(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
	system := "You are a helpful assistant."

	cases := []struct {
		name       string
		texts      []string
		structured []map[string]interface{}
		want       string
	}{
		{"tool result", []string{system, "look it up", scanInjection},
			[]map[string]interface{}{
				{"role": "system", "content": system},
				{"role": "user", "content": "look it up"},
				{"role": "assistant", "content": nil, "tool_calls": []interface{}{}},
				{"role": "tool", "tool_call_id": "call_1", "content": scanInjection},
			}, "BLOCKED"},
		{"tool result in parts", []string{"look it up", scanInjection},
			[]map[string]interface{}{
				{"role": "user", "content": []map[string]string{{"type": "text", "text": "look it up"}}},
				{"role": "tool", "content": []map[string]string{{"type": "text", "text": scanInjection}}},
			}, "BLOCKED"},
		{"the user's own words", []string{system, scanInjection},
			[]map[string]interface{}{
				{"role": "system", "content": system},
				{"role": "user", "content": scanInjection},
			}, "NONE"},
		{"no structured_messages: the user's", []string{scanInjection}, nil, "NONE"},
		// The texts are not the messages' texts, so the messages say nothing
		// about them.
		{"structured_messages of another conversation", []string{scanInjection},
			[]map[string]interface{}{{"role": "tool", "content": "something else"}}, "NONE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, _, raw := g.litellm(t, "request", tc.texts, tc.structured)
			if out.Action != tc.want {
				t.Fatalf("action = %q, want %q: %s", out.Action, tc.want, raw)
			}
			if tc.want == "BLOCKED" && !strings.Contains(out.BlockedReason, "injection") {
				t.Fatalf("blocked_reason = %q", out.BlockedReason)
			}
		})
	}
}

func TestLiteLLM_Auth(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy, devMode: true})
	revoked, err := g.keys.Create("revoked", apikeys.ScopeProxy)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.keys.Delete(revoked.ID); err != nil {
		t.Fatal(err)
	}
	proxyKey := g.key(t, apikeys.ScopeProxy)
	readKey := g.key(t, apikeys.ScopeRead)

	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		// The rig runs the management API in dev mode, which lets every
		// other route through without a key. Not this one.
		{"no key", nil, http.StatusUnauthorized},
		{"unknown key as LiteLLM sends it", map[string]string{"x-api-key": "tk_not_a_key"}, http.StatusUnauthorized},
		{"unknown key", map[string]string{"X-Tamga-Key": "tk_not_a_key"}, http.StatusUnauthorized},
		{"revoked key", map[string]string{"x-api-key": revoked.RawKey}, http.StatusUnauthorized},
		{"wrong admin key", map[string]string{"X-Tamga-Admin-Key": "wrong"}, http.StatusUnauthorized},
		// x-api-key is read as X-Tamga-Key is: it takes a key from the
		// store, not the admin key.
		{"admin key as LiteLLM's api_key", map[string]string{"x-api-key": scanAdminKey}, http.StatusUnauthorized},
		{"store key in the admin header", map[string]string{"X-Tamga-Admin-Key": readKey}, http.StatusUnauthorized},
		{"proxy key in the query", map[string]string{"?": "key=" + proxyKey}, http.StatusUnauthorized},
		{"proxy key in the query as api_key", map[string]string{"?": "api_key=" + proxyKey + "&x-api-key=" + proxyKey}, http.StatusUnauthorized},
		{"bearer token alone", map[string]string{"Authorization": "Bearer " + proxyKey}, http.StatusUnauthorized},
		{"bad X-Tamga-Key beside a good x-api-key", map[string]string{"X-Tamga-Key": "tk_not_a_key", "x-api-key": proxyKey}, http.StatusUnauthorized},
		{"proxy key as LiteLLM sends it", map[string]string{"x-api-key": proxyKey}, http.StatusOK},
		{"proxy key", map[string]string{"X-Tamga-Key": proxyKey}, http.StatusOK},
		{"read key", map[string]string{"x-api-key": readKey}, http.StatusOK},
		{"write key", map[string]string{"x-api-key": g.key(t, apikeys.ScopeWrite)}, http.StatusOK},
		{"admin-scope key", map[string]string{"x-api-key": g.key(t, apikeys.ScopeAdmin)}, http.StatusOK},
		{"admin key in a static header", litellmAdmin(), http.StatusOK},
	}
	refusals := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := g.litellmRaw(t, tc.headers, litellmBody("request", []string{litellmTCKNText}, nil))
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.want, body)
			}
			if tc.want != http.StatusUnauthorized {
				return
			}
			var fail litellmFailure
			if err := json.Unmarshal([]byte(body), &fail); err != nil || fail.Error.Code != "tamga_unauthorized" {
				t.Fatalf("401 body: %s", body)
			}
			// No verdict, and nothing LiteLLM could read as one.
			if strings.Contains(body, "action") || strings.Contains(body, "tc_kimlik") {
				t.Fatalf("401 carries a verdict: %s", body)
			}
			refusals[fail.Error.Code+"|"+fail.Error.Message] = true
		})
	}
	// Unknown, revoked, misplaced: which it was is not said.
	if len(refusals) != 1 {
		t.Fatalf("401 bodies differ: %v", refusals)
	}
}

// The route has no scope of its own to get wrong: like the scan route it
// takes a key of any scope. What a key's scope limits is everything else,
// and the header LiteLLM sends a key in is a credential nowhere else.
func TestLiteLLM_KeyOpensNothingElse(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
	proxyKey := g.key(t, apikeys.ScopeProxy)
	adminScope := g.key(t, apikeys.ScopeAdmin)

	do := func(method, path, header, key string) int {
		req, _ := http.NewRequest(method, g.url+"/api/v1"+path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(header, key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	routes := append(managementRoutes(t),
		[2]string{http.MethodGet, "/beta/litellm_basic_guardrail_api"},
		[2]string{http.MethodPut, "/beta/litellm_basic_guardrail_api"},
		[2]string{http.MethodPost, "/beta/litellm_basic_guardrail_api/extra"},
		[2]string{http.MethodPost, "/beta"})
	for _, rt := range routes {
		// A proxy key has the wrong scope for every management route.
		if got := do(rt[0], rt[1], "X-Tamga-Admin-Key", proxyKey); got != http.StatusForbidden {
			t.Errorf("%s %s with a proxy key as X-Tamga-Admin-Key: status = %d, want 403", rt[0], rt[1], got)
		}
		// And x-api-key is no credential there, whatever the key's scope.
		for name, key := range map[string]string{"proxy": proxyKey, "admin-scope": adminScope} {
			if got := do(rt[0], rt[1], "x-api-key", key); got != http.StatusUnauthorized {
				t.Errorf("%s %s with a %s key as x-api-key: status = %d, want 401", rt[0], rt[1], name, got)
			}
		}
	}
	if got := do(http.MethodPost, "/beta/litellm_basic_guardrail_api", "x-api-key", proxyKey); got != http.StatusBadRequest {
		t.Fatalf("POST to the route with the proxy key: status = %d, want 400 (let in, no input_type)", got)
	}
}

func TestLiteLLM_Limits(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
	if litellmMaxBodyBytes != 1048576 || !strings.Contains(litellmErrTooLarge, "1048576") ||
		!strings.Contains(litellmErrTextsLarge, "262144") || litellmMaxTexts != 256 || !strings.Contains(litellmErrTooMany, "256") {
		t.Fatal("the limits are not the documented ones")
	}
	wrap := func(texts []string, pad int) []byte {
		b, _ := json.Marshal(map[string]interface{}{
			"input_type": "request", "texts": texts, "request_data": map[string]string{}, "images": []string{strings.Repeat("A", pad)},
		})
		return b
	}
	many := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "hello"
		}
		return out
	}

	cases := []struct {
		name string
		body []byte
		want int
		msg  string
	}{
		{"texts at the scan limit", wrap([]string{strings.Repeat("a", scanMaxBodyBytes)}, 0), http.StatusOK, ""},
		{"texts one byte over", wrap([]string{strings.Repeat("a", scanMaxBodyBytes), "b"}, 0), http.StatusRequestEntityTooLarge, litellmErrTextsLarge},
		{"256 texts", wrap(many(litellmMaxTexts), 0), http.StatusOK, ""},
		{"257 texts", wrap(many(litellmMaxTexts+1), 0), http.StatusRequestEntityTooLarge, litellmErrTooMany},
		// What is not text counts toward the body, not toward the texts.
		{"a large body with little text", wrap([]string{"hello"}, 900*1024), http.StatusOK, ""},
		{"body over the limit", wrap([]string{"hello"}, litellmMaxBodyBytes), http.StatusRequestEntityTooLarge, litellmErrTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, raw := g.litellmRaw(t, litellmAdmin(), tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("body of %d bytes: status = %d, want %d: %.200s", len(tc.body), resp.StatusCode, tc.want, raw)
			}
			if tc.want == http.StatusOK {
				return
			}
			var fail litellmFailure
			if err := json.Unmarshal([]byte(raw), &fail); err != nil || fail.Error.Code != "tamga_too_large" || fail.Error.Message != tc.msg {
				t.Fatalf("413 body: %.200s", raw)
			}
		})
	}

	// Without a Content-Length the limit is met while reading.
	over := wrap([]string{"hello"}, litellmMaxBodyBytes)
	req, _ := http.NewRequest(http.MethodPost, g.url+litellmPath, io.MultiReader(bytes.NewReader(over)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tamga-Admin-Key", scanAdminKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("chunked body over the limit: status = %d, want 413", resp.StatusCode)
	}
}

func TestLiteLLM_MalformedBody(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
	tckn := `"` + litellmTCKN + `"`

	cases := []struct {
		name    string
		body    string
		headers map[string]string
		want    int
	}{
		{"not JSON", `texts=` + litellmTCKN, nil, http.StatusBadRequest},
		{"cut short", `{"input_type":"request","texts":[` + tckn, nil, http.StatusBadRequest},
		{"not an object", `[` + tckn + `]`, nil, http.StatusBadRequest},
		{"empty body", ``, nil, http.StatusBadRequest},
		{"no input_type", `{"texts":[` + tckn + `]}`, nil, http.StatusBadRequest},
		{"null input_type", `{"input_type":null,"texts":[` + tckn + `]}`, nil, http.StatusBadRequest},
		{"unknown input_type", `{"input_type":"both","texts":[` + tckn + `]}`, nil, http.StatusBadRequest},
		{"input_type in capitals", `{"input_type":"Request","texts":[` + tckn + `]}`, nil, http.StatusBadRequest},
		{"texts is a string", `{"input_type":"request","texts":` + tckn + `}`, nil, http.StatusBadRequest},
		{"texts holds a number", `{"input_type":"request","texts":[` + litellmTCKN + `]}`, nil, http.StatusBadRequest},
		{"texts holds an object", `{"input_type":"request","texts":[{"text":` + tckn + `}]}`, nil, http.StatusBadRequest},
		{"structured_messages is a string", `{"input_type":"request","texts":["hello"],"structured_messages":"x"}`, nil, http.StatusBadRequest},
		{"invalid UTF-8", "{\"input_type\":\"request\",\"texts\":[\"caf\xe9\"]}", nil, http.StatusBadRequest},
		// Which of the two would be scanned is the decoder's choice, not the
		// caller's.
		{"duplicate key", `{"input_type":"request","texts":["hello"],"texts":[` + tckn + `]}`, nil, http.StatusBadRequest},
		{"no content type", `{"input_type":"request","texts":["hello"]}`, map[string]string{"Content-Type": "-"}, http.StatusUnsupportedMediaType},
		{"text/plain", `{"input_type":"request","texts":["hello"]}`, map[string]string{"Content-Type": "text/plain"}, http.StatusUnsupportedMediaType},
		{"the least LiteLLM may send", `{"input_type":"request","request_data":{}}`, nil, http.StatusOK},
		{"unknown fields are ignored", `{"input_type":"request","texts":["hello"],"future_field":{"a":1}}`, nil, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			headers := litellmAdmin()
			for k, v := range tc.headers {
				headers[k] = v
			}
			resp, body := g.litellmRaw(t, headers, []byte(tc.body))
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.want, body)
			}
			// Nothing of the request comes back, refused or not.
			if strings.Contains(body, litellmTCKN) || strings.Contains(body, "hello") {
				t.Fatalf("response echoes the request: %s", body)
			}
			if tc.want == http.StatusOK {
				return
			}
			// A refusal is not a reply: LiteLLM reads a 2xx body for an
			// action, and takes any other status for a failed guardrail.
			var fail litellmFailure
			if err := json.Unmarshal([]byte(body), &fail); err != nil || !strings.HasPrefix(fail.Error.Code, "tamga_") || fail.Error.RequestID == "" {
				t.Fatalf("error body: %s", body)
			}
			if strings.Contains(body, `"action"`) {
				t.Fatalf("a refusal carries an action: %s", body)
			}
		})
	}
}

// unplacedScanner reports an address it cannot point at, as a scanner does
// for a value it saw only in a normalised view of the text.
type unplacedScanner struct{}

func (unplacedScanner) Name() string { return "unplaced" }
func (unplacedScanner) Scan(_ context.Context, content []byte) ([]scanner.Finding, error) {
	if !bytes.Contains(content, []byte("marker-unplaced")) {
		return nil, nil
	}
	return []scanner.Finding{{Type: "pii", Category: "email", Severity: "high", Confidence: 0.99, Match: "j***@***"}}, nil
}

// A REDACT that cannot be carried out is a block, as on the proxy path: the
// text is not returned as it came.
func TestLiteLLM_RedactThatCannotBeDoneBlocks(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy, scanners: []scanner.Scanner{unplacedScanner{}}})
	out, resp, raw := g.litellm(t, "request", []string{"an address is in here somewhere, marker-unplaced"}, nil)

	if out.Action != "BLOCKED" {
		t.Fatalf("action = %q, want BLOCKED: %s", out.Action, raw)
	}
	if !strings.Contains(out.BlockedReason, "REDACT could not be carried out (unplaced)") {
		t.Fatalf("blocked_reason = %q", out.BlockedReason)
	}
	if got := resp.Header.Get("X-Tamga-Redact-Fallback"); got != "block:unplaced" {
		t.Fatalf("X-Tamga-Redact-Fallback = %q", got)
	}
	if strings.Contains(raw, "marker-unplaced") || strings.Contains(raw, "j***") {
		t.Fatalf("reply echoes the text or the match: %s", raw)
	}
	// The scan API reaches the same verdict.
	if verdict, sraw := g.scan(t, "an address is in here somewhere, marker-unplaced", "user"); verdict.Action != "BLOCK" {
		t.Fatalf("scan API says %q: %s", verdict.Action, sraw)
	}
}

// A scan that did not run to the end gives no reply LiteLLM could act on:
// not NONE, which would send the text on unread, and not BLOCKED, which
// would call a failure a finding.
func TestLiteLLM_ScanUnavailableFailsClosed(t *testing.T) {
	check := func(t *testing.T, resp *http.Response, raw, code string) {
		t.Helper()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503: %s", resp.StatusCode, raw)
		}
		if resp.Header.Get("Retry-After") == "" {
			t.Fatal("503 without Retry-After")
		}
		var fail litellmFailure
		if err := json.Unmarshal([]byte(raw), &fail); err != nil || fail.Error.Code != code {
			t.Fatalf("code = %q, want %q: %s", fail.Error.Code, code, raw)
		}
		for _, banned := range []string{"action", "NONE", "BLOCKED", "boom", "marker-incomplete", litellmTCKN} {
			if strings.Contains(raw, banned) {
				t.Fatalf("503 contains %q: %s", banned, raw)
			}
		}
	}

	for _, direction := range []string{"request", "response"} {
		t.Run("a scanner fails, "+direction, func(t *testing.T) {
			failing := &spyScanner{name: "failing", quote: true}
			failing.fail.Store(true)
			g := newScanRig(t, scanRigOptions{policy: litellmPolicy, scanners: []scanner.Scanner{failing}})
			resp, raw := g.litellmRaw(t, litellmAdmin(), litellmBody(direction, []string{"hello there marker-incomplete"}, nil))
			check(t, resp, raw, "tamga_scan_unavailable")
			if failing.calls.Load() == 0 {
				t.Fatal("the failing scanner was not run")
			}
		})
	}

	// The default scan.on_error forwards on the proxy path. Not here.
	for _, onError := range []string{"pass", "block"} {
		t.Run("classifier not configured, on_error "+onError, func(t *testing.T) {
			g := newScanRig(t, scanRigOptions{policy: classifierPolicy(onError, "")})
			resp, raw := g.litellmRaw(t, litellmAdmin(), litellmBody("request", []string{"hello there"}, nil))
			check(t, resp, raw, "tamga_scan_unavailable")
		})
	}

	t.Run("no policy loaded", func(t *testing.T) {
		g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
		g.bridge.scan.cfg.GetPolicy = func() *policy.Policy { return nil }
		resp, raw := g.litellmRaw(t, litellmAdmin(), litellmBody("request", []string{"hello there"}, nil))
		check(t, resp, raw, "tamga_scan_unavailable")
	})

	t.Run("every slot in use", func(t *testing.T) {
		g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
		slots := cap(g.handler.slots)
		for i := 0; i < slots; i++ {
			g.handler.slots <- struct{}{}
		}
		resp, raw := g.litellmRaw(t, litellmAdmin(), litellmBody("request", []string{"hello there"}, nil))
		check(t, resp, raw, "tamga_scan_busy")
		for i := 0; i < slots; i++ {
			<-g.handler.slots
		}
		if out, _, raw := g.litellm(t, "request", []string{"hello there"}, nil); out.Action != "NONE" {
			t.Fatalf("after the slots freed: %s", raw)
		}
		if len(g.handler.slots) != 0 {
			t.Fatalf("%d slots still held after the calls returned", len(g.handler.slots))
		}
	})
}

func TestLiteLLM_RateLimit(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy + `
rate_limit:
  max_requests_per_minute: 2
  action_on_exceed: WARN
`})
	key := g.key(t, apikeys.ScopeProxy)
	headers := map[string]string{"x-api-key": key}
	body := litellmBody("request", []string{"hello there"}, nil)

	for i := 0; i < 2; i++ {
		if resp, raw := g.litellmRaw(t, headers, body); resp.StatusCode != http.StatusOK {
			t.Fatalf("call %d: status = %d: %s", i+1, resp.StatusCode, raw)
		}
	}
	resp, raw := g.litellmRaw(t, headers, body)
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("third call: status = %d, Retry-After = %q, want 429 with one: %s", resp.StatusCode, resp.Header.Get("Retry-After"), raw)
	}
	if !strings.Contains(raw, `"code":"tamga_rate_limited"`) || strings.Contains(raw, "action") {
		t.Fatalf("429 body: %s", raw)
	}
	// Counted apart from the key's scans and from its proxy requests.
	if resp, raw := g.scanRaw(t, map[string]string{"X-Tamga-Key": key}, scanBody("hello there", "")); resp.StatusCode != http.StatusOK {
		t.Fatalf("scan with the key whose bridge calls are used up: status = %d: %s", resp.StatusCode, raw)
	}
	if status := g.proxyPost(t, "user", "hello", key); status != http.StatusOK {
		t.Fatalf("proxy request with the key whose bridge calls are used up: status = %d", status)
	}
}

// With no rate limit in the policy the route falls back to its own, which is
// not the scan API's.
func TestLiteLLM_DefaultRateLimit(t *testing.T) {
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
	resp, raw := g.litellmRaw(t, map[string]string{"x-api-key": g.key(t, apikeys.ScopeProxy)}, litellmBody("request", []string{"hello there"}, nil))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, raw)
	}
	if got := resp.Header.Get("X-RateLimit-Limit"); got != "600" {
		t.Fatalf("X-RateLimit-Limit = %q, want 600", got)
	}
}

// A call leaves what a scan leaves: one log line, and no record.
func TestLiteLLM_LeavesNoRecord(t *testing.T) {
	opState := &spyScanner{name: "operator_state"}
	g := newScanRig(t, scanRigOptions{
		policy:   litellmPolicy + "vault:\n  enabled: true\n",
		scanners: []scanner.Scanner{opState},
	})

	g.litellm(t, "request", []string{"write to " + scanEmail}, nil)
	g.litellm(t, "request", []string{litellmTCKNText}, nil)
	g.litellm(t, "request", []string{litellmClean}, nil)
	g.litellm(t, "response", []string{"write to " + scanEmail}, nil)
	g.litellm(t, "response", []string{litellmTCKNText}, nil)
	g.litellmRaw(t, nil, litellmBody("request", []string{litellmTCKNText}, nil))

	time.Sleep(150 * time.Millisecond)
	if n := g.eventCount(); n != 0 {
		t.Fatalf("%d events published by bridge calls, want 0", n)
	}
	if got := g.recent.Latest(10); len(got) != 0 {
		t.Fatalf("recent buffer holds %d events after bridge calls, want 0", len(got))
	}
	if n := g.vault.sets.Load(); n != 0 {
		t.Fatalf("%d vault writes by bridge calls, want 0", n)
	}
	if n := opState.calls.Load(); n != 0 {
		t.Fatalf("operator_state scanner run %d times by bridge calls, want 0", n)
	}
}

// STRIP replaces the text the finding is in, whole, with the proxy's own
// placeholder, and the other texts come back as they were sent.
func TestLiteLLM_StripReturnsThePlaceholder(t *testing.T) {
	stripping := strings.Replace(litellmPolicy,
		"action: BLOCK\n    sensitivity: low\n    applies_to: [tool]", "action: STRIP\n    sensitivity: low\n    applies_to: [tool]", 1)
	if stripping == litellmPolicy {
		t.Fatal("the policy was not changed")
	}
	g := newScanRig(t, scanRigOptions{policy: stripping})
	texts := []string{"look it up", scanInjection}
	out, resp, raw := g.litellm(t, "request", texts, []map[string]interface{}{
		{"role": "user", "content": "look it up"},
		{"role": "tool", "content": scanInjection},
	})
	if out.Action != "GUARDRAIL_INTERVENED" || out.Texts[0] != texts[0] || out.Texts[1] != stripPlaceholder {
		t.Fatalf("want the tool text replaced by the placeholder: %s", raw)
	}
	if got := resp.Header.Get("X-Tamga-Action"); got != "STRIP" {
		t.Fatalf("X-Tamga-Action = %q, want STRIP", got)
	}
}

// One line is logged per call, as for a scan, and nothing logged holds a
// text, a value found in one, a key, or what LiteLLM says about its caller.
func TestLiteLLM_LogsNoText(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	prev, prevLevel := log.Logger, zerolog.GlobalLevel()
	log.Logger = zerolog.New(lockedWriter{&mu, &buf})
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	t.Cleanup(func() {
		log.Logger = prev
		zerolog.SetGlobalLevel(prevLevel)
	})

	failing := &spyScanner{name: "failing"}
	g := newScanRig(t, scanRigOptions{policy: litellmPolicy, scanners: []scanner.Scanner{failing}})
	created, err := g.keys.CreateWith(apikeys.Params{Label: "litellm", Scope: apikeys.ScopeProxy, OrgID: "org_gw", Role: "analyst"})
	if err != nil {
		t.Fatal(err)
	}
	key := map[string]string{"x-api-key": created.RawKey}

	g.litellmRaw(t, key, litellmBody("request", []string{"marker-qzxj what is the capital of France?"}, nil))
	g.litellmRaw(t, key, litellmBody("request", []string{"marker-qzxj write to " + scanEmail, "marker-qzxj again"}, nil))
	g.litellmRaw(t, key, litellmBody("request", []string{"marker-qzxj " + litellmTCKNText}, nil))
	g.litellmRaw(t, key, litellmBody("response", []string{"marker-qzxj " + litellmTCKNText}, nil))
	g.litellmRaw(t, key, []byte(`{"input_type":"request","texts":["marker-qzxj`))
	g.litellmRaw(t, map[string]string{"x-api-key": "tk_marker-qzxj"}, litellmBody("request", []string{"hello"}, nil))
	failing.fail.Store(true)
	g.litellmRaw(t, key, litellmBody("request", []string{"marker-qzxj " + scanEmail}, nil))

	mu.Lock()
	logged := buf.String()
	mu.Unlock()

	var lines []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(logged), "\n") {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		if m["message"] == "scan api call" {
			t.Fatalf("a bridge call was logged as a scan API call: %s", line)
		}
		if m["message"] == "litellm bridge call" {
			lines = append(lines, m)
		}
	}
	if len(lines) != 7 {
		t.Fatalf("%d call lines for 7 calls:\n%s", len(lines), logged)
	}
	outcomes := make([]string, len(lines))
	for i, m := range lines {
		outcomes[i], _ = m["outcome"].(string)
	}
	if got, want := strings.Join(outcomes, ","), "PASS,REDACT,BLOCK,BLOCK,invalid,unauthorized,scan_incomplete"; got != want {
		t.Fatalf("outcomes = %s, want %s", got, want)
	}
	redact := lines[1]
	byType, _ := redact["findings_by_type"].(map[string]interface{})
	if redact["key_id"] != created.ID || redact["org_id"] != "org_gw" || redact["key_role"] != "analyst" ||
		redact["direction"] != "request" || redact["texts"] != float64(2) ||
		redact["findings"] != float64(1) || byType["pii"] != float64(1) ||
		redact["litellm_call_id"] != "c3f1a8e2-7b1d-4f0e-9d53-1f2a6b7c8d90" {
		t.Fatalf("call line lacks its fields: %v", redact)
	}
	if lines[3]["direction"] != "response" {
		t.Fatalf("direction of the answer's line = %v", lines[3]["direction"])
	}

	for _, secret := range []string{"qzxj", scanEmail, "jane", litellmTCKN, "kimlik", "capital of France", created.RawKey, scanAdminKey,
		"user-17", "team-a", "88dc28d0", "fake-model", "lookup"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("log contains %q:\n%s", secret, logged)
		}
	}
}
