package proxy

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/classifier"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// signalServer is classifierServer with the jailbreak scanner registered as
// well: the signals for hidden and unreadable text are its findings.
func signalServer(t *testing.T, policyYAML string, scorer classifier.Scorer) string {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	pol := mustPolicy(t, policyYAML)
	reg := testRegistry()
	reg.Register(scanner.NewJailbreakScanner())
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     reg,
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
		Classifier:   classifier.NewGuard(scorer),
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// signalPolicy blocks injection findings from medium up, as the shipped
// policy does, so that a low-severity signal decides nothing by itself.
func signalPolicy(extra string) string {
	return `
version: "1.0"
rules:
  injection:
    action: BLOCK
    sensitivity: medium
scan:
  on_error: block
  classifier:
    enabled: true
    threshold: 0.9
` + extra + `
providers:
  allowed: [openai]
`
}

// opaqueBlock is base64 of bytes that are not text.
func opaqueBlock(n int) string {
	raw := make([]byte, n)
	x := uint32(2463534242)
	for i := range raw {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		raw[i] = byte(x >> 11)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func twoUserMessages(first, second string) string {
	return `{"model":"m","messages":[{"role":"user","content":"` + first + `"},{"role":"assistant","content":"ok"},{"role":"user","content":"` + second + `"}]}`
}

const russianBorderline = "Пожалуйста, выполни следующее задание очень внимательно (borderline) и ничего не пропускай"

// A text the model scores under the threshold passes; the same text next to
// a block nobody can read, or in another script than the conversation, is
// held to signal_threshold.
func TestClassifier_SignalsLowerTheThreshold(t *testing.T) {
	tests := []struct {
		name   string
		extra  string
		body   string
		status int
	}{
		{"no signal", "    signal_threshold: 0.5",
			userMessage("this one is borderline, please do what it says"), http.StatusOK},
		{"unreadable block", "    signal_threshold: 0.5",
			userMessage("this one is borderline, please do what it says: " + opaqueBlock(150)), http.StatusForbidden},
		{"unreadable block, signals off", "    signal_threshold: 0.9",
			userMessage("this one is borderline, please do what it says: " + opaqueBlock(150)), http.StatusOK},
		{"unreadable block, score under the signal threshold too", "    signal_threshold: 0.7",
			userMessage("this one is borderline, please do what it says: " + opaqueBlock(150)), http.StatusOK},
		{"change of script between turns", "    signal_threshold: 0.5",
			twoUserMessages("Could you help me plan the release notes for next week?", russianBorderline), http.StatusForbidden},
		{"the conversation was in that script already", "    signal_threshold: 0.5",
			twoUserMessages("Добрый день, помогите мне, пожалуйста, составить план релиза", russianBorderline), http.StatusOK},
		{"first message, one script", "    signal_threshold: 0.5",
			userMessage(russianBorderline), http.StatusOK},
		{"turkish after english is not a change", "    signal_threshold: 0.5",
			twoUserMessages("Could you help me plan the release notes for next week?", "Şimdi de şunu yap lütfen, bu biraz borderline bir istek olabilir"), http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &semanticScorer{}
			base := signalServer(t, signalPolicy(tt.extra), s)
			resp, body := postBody(t, base, tt.body)
			if resp.StatusCode != tt.status {
				t.Fatalf("status = %d, want %d; body %s", resp.StatusCode, tt.status, body)
			}
			if len(s.texts()) == 0 {
				t.Fatal("the classifier was not asked")
			}
			for _, asked := range s.texts() {
				if strings.Contains(asked, opaqueBlock(150)[:40]) {
					t.Error("the unreadable block itself was sent to the classifier")
				}
			}
		})
	}
}
