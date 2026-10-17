package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// The language check end to end: a policy that allows Turkish and English,
// and requests written in other languages. The texts were written for these
// tests.
const (
	germanText  = "Hallo, das Produkt, das ich letzte Woche bestellt habe, kam beschädigt an. Eine Ecke des Kartons war eingedrückt und das Gerät funktioniert nicht. Wie kann ich die Rücksendung einleiten?"
	turkishText = "Merhaba, geçen hafta sipariş ettiğim ürün hasarlı geldi. Kutunun bir köşesi ezilmişti ve içindeki cihaz çalışmıyor. İade sürecini nasıl başlatabilirim?"
	javaTrace   = "Exception in thread \"main\" java.lang.IllegalStateException: Could not open the connection to the database server\n" +
		"\tat com.example.db.Pool.open(Pool.java:118)\n\tat com.example.db.Pool.get(Pool.java:64)\n" +
		"\tat com.example.service.OrderService.load(OrderService.java:212)\n" +
		"Caused by: java.net.ConnectException: Connection refused: the remote host did not answer in time\n\t... 12 more"
)

func languagePolicy(section string) string {
	return `
version: "1.0"
name: language-test
language:
` + section + `
providers:
  allowed: [openai]
`
}

// languageServer is a proxy with the language check, in front of a provider
// that records what it was sent.
func languageServer(t *testing.T, policyYAML string) (string, *int32, *atomic.Value) {
	t.Helper()
	var hits int32
	var last atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		b, _ := io.ReadAll(r.Body)
		last.Store(string(b))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	pol := mustPolicy(t, policyYAML)
	getPolicy := func() *policy.Policy { return pol }
	registry := testRegistry()
	registry.Register(scanner.NewLanguageScanner(func() scanner.LanguageSpec { return getPolicy().LanguageSpec() }))
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     registry,
		GetPolicy:    getPolicy,
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &hits, &last
}

func languageChatBody(t *testing.T, messages ...map[string]string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"model": "gpt-4o-mini", "messages": messages})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLanguage_GermanIsBlockedWhenOnlyTurkishAndEnglishAreAllowed(t *testing.T) {
	base, hits, _ := languageServer(t, languagePolicy("  enabled: true\n  allow: [tr, en]\n  action: BLOCK"))
	resp, body := postRaw(t, base, languageChatBody(t, map[string]string{"role": "user", "content": germanText}))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403: %s", resp.StatusCode, body)
	}
	if atomic.LoadInt32(hits) != 0 {
		t.Fatal("a blocked request reached the provider")
	}
	// The refusal does not quote the text.
	if strings.Contains(body, "Produkt") {
		t.Fatalf("the refusal quotes the text: %s", body)
	}
	// A Turkish request under the same policy goes through.
	resp, body = postRaw(t, base, languageChatBody(t, map[string]string{"role": "user", "content": turkishText}))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Turkish: status %d: %s", resp.StatusCode, body)
	}
}

func TestLanguage_SameRequestPassesWhenTheSectionIsOff(t *testing.T) {
	for name, section := range map[string]string{
		"disabled": "  enabled: false\n  allow: [tr, en]\n  action: BLOCK",
		"allowed":  "  enabled: true\n  allow: [tr, en, de]\n  action: BLOCK",
	} {
		base, hits, _ := languageServer(t, languagePolicy(section))
		resp, body := postRaw(t, base, languageChatBody(t, map[string]string{"role": "user", "content": germanText}))
		if resp.StatusCode != http.StatusOK || atomic.LoadInt32(hits) != 1 {
			t.Fatalf("%s: status %d, %d upstream calls: %s", name, resp.StatusCode, atomic.LoadInt32(hits), body)
		}
	}
}

func TestLanguage_EnglishTraceInATurkishMessagePasses(t *testing.T) {
	// Only Turkish is allowed, so the English of the trace would count if
	// it were read as prose.
	base, hits, _ := languageServer(t, languagePolicy("  enabled: true\n  allow: [tr]\n  action: BLOCK"))
	content := turkishText + "\n\nAldığım hata şu:\n\n" + javaTrace
	resp, body := postRaw(t, base, languageChatBody(t, map[string]string{"role": "user", "content": content}))
	if resp.StatusCode != http.StatusOK || atomic.LoadInt32(hits) != 1 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
}

func TestLanguage_ScopeLeavesTheSystemPromptAlone(t *testing.T) {
	base, hits, _ := languageServer(t, languagePolicy("  enabled: true\n  allow: [tr, en]\n  action: BLOCK"))
	resp, body := postRaw(t, base, languageChatBody(t,
		map[string]string{"role": "system", "content": germanText},
		map[string]string{"role": "user", "content": turkishText}))
	if resp.StatusCode != http.StatusOK || atomic.LoadInt32(hits) != 1 {
		t.Fatalf("a German system prompt with scope [user, tool]: status %d: %s", resp.StatusCode, body)
	}
}

func TestLanguage_StripTakesOutAToolResult(t *testing.T) {
	base, hits, last := languageServer(t, languagePolicy("  enabled: true\n  allow: [tr, en]\n  action: STRIP\n  scope: [tool]"))
	body := `{"model":"gpt-4o","messages":[
  {"role":"user","content":"Bu sayfayı özetler misin?"},
  {"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"fetch","arguments":"{\"url\":\"https://example.invalid\"}"}}]},
  {"role":"tool","tool_call_id":"c1","content":"` + germanText + `"}
]}`
	resp, respBody := postRaw(t, base, body)
	if resp.StatusCode != http.StatusOK || atomic.LoadInt32(hits) != 1 {
		t.Fatalf("status %d: %s", resp.StatusCode, respBody)
	}
	sent := sentBody(t, last)
	if strings.Contains(sent, "Produkt") || !strings.Contains(sent, stripPlaceholder) {
		t.Fatalf("forwarded body: %s", sent)
	}
	if !strings.Contains(sent, "Bu sayfayı özetler misin?") {
		t.Fatalf("the user's message changed: %s", sent)
	}
}

func TestLanguage_WarnForwardsTheRequest(t *testing.T) {
	base, hits, _ := languageServer(t, languagePolicy("  enabled: true\n  allow: [tr, en]"))
	resp, body := postRaw(t, base, languageChatBody(t, map[string]string{"role": "user", "content": germanText}))
	if resp.StatusCode != http.StatusOK || atomic.LoadInt32(hits) != 1 {
		t.Fatalf("WARN, the default action: status %d: %s", resp.StatusCode, body)
	}
}

func TestLanguage_ScanAPIGivesTheVerdict(t *testing.T) {
	yaml := languagePolicy("  enabled: true\n  allow: [tr, en]\n  action: BLOCK")
	spec := mustPolicy(t, yaml).LanguageSpec()
	g := newScanRig(t, scanRigOptions{
		policy:   yaml,
		scanners: []scanner.Scanner{scanner.NewLanguageScanner(func() scanner.LanguageSpec { return spec })},
	})
	res, raw := g.scan(t, germanText, "user")
	if res.Action != "BLOCK" || len(res.Findings) != 1 || res.Findings[0].Type != "language" ||
		res.Findings[0].Category != "language_not_allowed" || res.Findings[0].Position != "none" {
		t.Fatalf("scan: %s", raw)
	}
	if res, raw := g.scan(t, turkishText, "user"); res.Action != "PASS" {
		t.Fatalf("Turkish: %s", raw)
	}
}
