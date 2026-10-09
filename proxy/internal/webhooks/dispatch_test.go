package webhooks

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/scanner"
)

// fastDispatch keeps the retry policy and shortens the waits.
var fastDispatch = DispatcherConfig{
	DefaultOrg:     "default",
	AttemptTimeout: 2 * time.Second,
	Backoff:        5 * time.Millisecond,
}

// hit is one request a receiver saw.
type hit struct {
	header http.Header
	body   string
}

type receiver struct {
	*httptest.Server
	mu   sync.Mutex
	hits []hit
}

// newReceiver answers every request with status.
func newReceiver(t *testing.T, status int) *receiver {
	t.Helper()
	r := &receiver{}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.hits = append(r.hits, hit{header: req.Header.Clone(), body: string(b)})
		r.mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte("receiver says: token=RESPONSE_BODY_SECRET"))
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *receiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.hits)
}

func (r *receiver) all() []hit {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]hit(nil), r.hits...)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func statsOf(d *Dispatcher, k Kind) KindStats {
	for _, s := range d.Stats() {
		if s.Kind == string(k) {
			return s
		}
	}
	return KindStats{Kind: string(k)}
}

func blockEvent(id string) events.Event {
	return events.Event{
		RequestID: id,
		OrgID:     "acme",
		Provider:  "openai",
		Model:     "gpt-4o",
		EventType: "request_blocked",
		Action:    "BLOCK",
		Body:      []byte(livePrompt),
		Findings:  []scanner.Finding{{Type: "secret", Category: "aws_access_key", Severity: "critical", Match: "AKIAIOSF************"}},
	}
}

func mustCreate(t *testing.T, s *MemoryStore, w Webhook) Webhook {
	t.Helper()
	out, err := s.Create(w)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestWants(t *testing.T) {
	critical := []AlertFinding{{Type: "secret", Category: "aws_access_key", Severity: "critical"}}
	medium := []AlertFinding{{Type: "pii", Category: "email", Severity: "medium"}}
	for _, tc := range []struct {
		name string
		w    Webhook
		a    Alert
		want bool
	}{
		{"block", Webhook{}, Alert{Action: "BLOCK", Findings: critical}, true},
		{"strip", Webhook{}, Alert{Action: "STRIP", Findings: critical}, true},
		{"warn", Webhook{}, Alert{Action: "WARN", Findings: medium}, true},
		{"redact, no correlation", Webhook{}, Alert{Action: "REDACT", Findings: medium}, false},
		{"redact, threshold of one bounds nothing", Webhook{ThresholdCount: 1}, Alert{Action: "REDACT", Findings: medium}, false},
		{"redact, threshold", Webhook{ThresholdCount: 5}, Alert{Action: "REDACT", Findings: medium}, true},
		{"redact, cooldown", Webhook{CooldownSecs: 60}, Alert{Action: "REDACT", Findings: medium}, true},
		{"pass", Webhook{CooldownSecs: 60}, Alert{Action: "PASS"}, false},
		{"log", Webhook{CooldownSecs: 60}, Alert{Action: "LOG", Findings: medium}, false},
		{"below severity", Webhook{Rule: &AlertRule{SeverityAtLeast: "high"}}, Alert{Action: "BLOCK", Findings: medium}, false},
		{"at severity", Webhook{Rule: &AlertRule{SeverityAtLeast: "high"}}, Alert{Action: "BLOCK", Findings: critical}, true},
		{"severity nobody knows filters nothing", Webhook{Rule: &AlertRule{SeverityAtLeast: "loud"}}, Alert{Action: "BLOCK", Findings: medium}, true},
	} {
		if got := wants(tc.w, tc.a); got != tc.want {
			t.Errorf("%s: wants = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCorrelationKey(t *testing.T) {
	base := Alert{OrgID: "acme", Direction: "input", Action: "BLOCK", RequestID: "r1",
		Findings: []AlertFinding{{Type: "pii", Category: "email", Severity: "low"}, {Type: "secret", Category: "aws_access_key", Severity: "critical"}}}
	if got := correlationKey(base); got != "acme|input|BLOCK|aws_access_key" {
		t.Fatalf("key = %q", got)
	}
	same := base
	same.RequestID = "r2"
	if correlationKey(same) != correlationKey(base) {
		t.Fatal("two requests with the same finding must share a key")
	}
	for name, change := range map[string]func(*Alert){
		"org":       func(a *Alert) { a.OrgID = "globex" },
		"action":    func(a *Alert) { a.Action = "WARN" },
		"direction": func(a *Alert) { a.Direction = "output" },
		"category":  func(a *Alert) { a.Findings = a.Findings[:1] },
	} {
		other := base
		change(&other)
		if correlationKey(other) == correlationKey(base) {
			t.Errorf("a different %s must be a different key", name)
		}
	}
	if got := correlationKey(Alert{OrgID: "acme", Direction: "output", Action: "BLOCK"}); got != "acme|output|BLOCK|policy" {
		t.Fatalf("key without findings = %q", got)
	}
}

func TestDispatcher_DeliversAndMarksFired(t *testing.T) {
	rcv := newReceiver(t, http.StatusOK)
	store := NewMemoryStore()
	w := mustCreate(t, store, Webhook{Kind: KindOpsgenie, URL: rcv.URL + "/v2/alerts", Enabled: true, AuthToken: "GKEY", Headers: map[string]string{"X-Team": "soc"}})
	off := mustCreate(t, store, Webhook{Kind: KindGeneric, URL: rcv.URL + "/disabled", Enabled: false})
	d := NewDispatcher(store, fastDispatch)
	defer d.Stop(time.Second)

	before := time.Now().UTC().Add(-time.Second)
	d.HandleEvent(blockEvent("req-1"))
	waitFor(t, "the delivery", func() bool { return statsOf(d, KindOpsgenie).Sent == 1 })

	hits := rcv.all()
	if len(hits) != 1 {
		t.Fatalf("receiver saw %d requests, want 1 (the disabled destination gets none)", len(hits))
	}
	h := hits[0]
	if h.header.Get("Authorization") != "GenieKey GKEY" || h.header.Get("X-Team") != "soc" || h.header.Get("Content-Type") != "application/json" {
		t.Fatalf("headers: %v", h.header)
	}
	if !strings.Contains(h.body, `"alias":"tamga-acme_input_BLOCK_aws_access_key"`) || !strings.Contains(h.body, "req-1") {
		t.Fatalf("body: %s", h.body)
	}
	got, _ := store.Get(w.ID)
	if !got.LastFired.After(before) {
		t.Fatalf("last fired = %v, want it set by the delivery", got.LastFired)
	}
	if o, _ := store.Get(off.ID); !o.LastFired.IsZero() {
		t.Fatal("a disabled destination did not fire")
	}
}

func TestDispatcher_RetriesServerErrorsOnly(t *testing.T) {
	bad := newReceiver(t, http.StatusInternalServerError)
	refused := newReceiver(t, http.StatusBadRequest)
	store := NewMemoryStore()
	w500 := mustCreate(t, store, Webhook{Kind: KindSlack, URL: bad.URL, Enabled: true})
	mustCreate(t, store, Webhook{Kind: KindGeneric, URL: refused.URL, Enabled: true})
	d := NewDispatcher(store, fastDispatch)
	defer d.Stop(time.Second)

	d.HandleEvent(blockEvent("req-1"))
	waitFor(t, "both to fail", func() bool {
		return statsOf(d, KindSlack).Failed == 1 && statsOf(d, KindGeneric).Failed == 1
	})
	time.Sleep(100 * time.Millisecond)
	if n := bad.count(); n != defaultMaxAttempts {
		t.Fatalf("a 500 was tried %d times, want %d", n, defaultMaxAttempts)
	}
	if n := refused.count(); n != 1 {
		t.Fatalf("a 400 was tried %d times, want 1", n)
	}
	if s := statsOf(d, KindSlack); s.Sent != 0 {
		t.Fatalf("stats: %+v", s)
	}
	if got, _ := store.Get(w500.ID); !got.LastFired.IsZero() {
		t.Fatal("a destination that never accepted the alert has not fired")
	}
}

func TestDispatcher_RetriesNetworkErrors(t *testing.T) {
	var tries atomic.Int32
	// The first two connections are cut before an answer; the third is served.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tries.Add(1) < 3 {
			conn, _, _ := w.(http.Hijacker).Hijack()
			_ = conn.Close()
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	store := NewMemoryStore()
	mustCreate(t, store, Webhook{Kind: KindGeneric, URL: srv.URL, Enabled: true})
	d := NewDispatcher(store, fastDispatch)
	defer d.Stop(time.Second)

	d.HandleEvent(blockEvent("req-1"))
	waitFor(t, "the third attempt", func() bool { return statsOf(d, KindGeneric).Sent == 1 })
	if n := tries.Load(); n != 3 {
		t.Fatalf("%d attempts, want 3", n)
	}
}

func TestDispatcher_CooldownAloneSuppresses(t *testing.T) {
	rcv := newReceiver(t, http.StatusOK)
	store := NewMemoryStore()
	mustCreate(t, store, Webhook{Kind: KindGeneric, URL: rcv.URL, Enabled: true, CooldownSecs: 60})
	d := NewDispatcher(store, fastDispatch)
	defer d.Stop(time.Second)

	d.HandleEvent(blockEvent("req-1"))
	d.HandleEvent(blockEvent("req-2"))
	d.HandleEvent(blockEvent("req-3"))
	// A different organisation is a different alert and is not held back.
	other := blockEvent("req-4")
	other.OrgID = "globex"
	d.HandleEvent(other)

	waitFor(t, "two deliveries", func() bool { return statsOf(d, KindGeneric).Sent == 2 })
	if s := statsOf(d, KindGeneric); s.Suppressed != 2 {
		t.Fatalf("stats: %+v, want 2 suppressed", s)
	}
	time.Sleep(50 * time.Millisecond)
	if n := rcv.count(); n != 2 {
		t.Fatalf("receiver saw %d, want 2", n)
	}
}

func TestDispatcher_ThresholdHoldsUntilMet(t *testing.T) {
	rcv := newReceiver(t, http.StatusOK)
	store := NewMemoryStore()
	// No window given: the documented default applies.
	mustCreate(t, store, Webhook{Kind: KindGeneric, URL: rcv.URL, Enabled: true, ThresholdCount: 3})
	d := NewDispatcher(store, fastDispatch)
	defer d.Stop(time.Second)

	d.HandleEvent(blockEvent("req-1"))
	d.HandleEvent(blockEvent("req-2"))
	if s := statsOf(d, KindGeneric); s.Suppressed != 2 {
		t.Fatalf("stats: %+v, want 2 suppressed", s)
	}
	d.HandleEvent(blockEvent("req-3"))
	waitFor(t, "the third to fire", func() bool { return rcv.count() == 1 })
	if body := rcv.all()[0].body; !strings.Contains(body, `"correlated_count":3`) || !strings.Contains(body, "req-3") {
		t.Fatalf("body: %s", body)
	}
}

func TestDispatcher_BlocksPerMinute(t *testing.T) {
	rcv := newReceiver(t, http.StatusOK)
	store := NewMemoryStore()
	mustCreate(t, store, Webhook{Kind: KindGeneric, URL: rcv.URL, Enabled: true, Rule: &AlertRule{BlocksPerMinute: 2}})
	d := NewDispatcher(store, fastDispatch)
	defer d.Stop(time.Second)

	// "More than 2 a minute": the third block is the first to be sent.
	d.HandleEvent(blockEvent("req-1"))
	d.HandleEvent(blockEvent("req-2"))
	d.HandleEvent(blockEvent("req-3"))
	waitFor(t, "the third block", func() bool { return rcv.count() == 1 })
	if s := statsOf(d, KindGeneric); s.Suppressed != 2 {
		t.Fatalf("stats: %+v", s)
	}
	// The rate is about blocks: a warning is not held back by it.
	warn := blockEvent("req-4")
	warn.EventType, warn.Action = "request_scanned", "WARN"
	d.HandleEvent(warn)
	waitFor(t, "the warning", func() bool { return rcv.count() == 2 })
}

// A receiver that never answers holds a worker, not the caller. With one
// worker and room for one more, everything after that is dropped.
func TestDispatcher_FullQueueDropsWithoutBlocking(t *testing.T) {
	release := make(chan struct{})
	var seen atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		<-release
	}))
	defer srv.Close()
	defer close(release)

	store := NewMemoryStore()
	mustCreate(t, store, Webhook{Kind: KindGeneric, URL: srv.URL, Enabled: true})
	d := NewDispatcher(store, DispatcherConfig{QueueSize: 1, Workers: 1, AttemptTimeout: 30 * time.Second, MaxAttempts: 1})
	defer d.Stop(10 * time.Millisecond)

	d.HandleEvent(blockEvent("req-0"))
	waitFor(t, "the worker to be busy", func() bool { return seen.Load() == 1 })

	start := time.Now()
	for i := 0; i < 50; i++ {
		d.HandleEvent(blockEvent("req-n"))
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("50 events took %v with a stuck receiver: the caller was blocked", took)
	}
	s := statsOf(d, KindGeneric)
	if s.Dropped != 49 || s.Sent != 0 {
		t.Fatalf("stats: %+v, want 49 dropped (one in flight, one queued)", s)
	}
}

func TestDispatcher_StopCutsOffAfterTheDrain(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	store := NewMemoryStore()
	mustCreate(t, store, Webhook{Kind: KindGeneric, URL: srv.URL, Enabled: true})
	d := NewDispatcher(store, DispatcherConfig{AttemptTimeout: 30 * time.Second})

	for i := 0; i < 10; i++ {
		d.HandleEvent(blockEvent("req-n"))
	}
	start := time.Now()
	d.Stop(100 * time.Millisecond)
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("Stop took %v", took)
	}
	if s := statsOf(d, KindGeneric); s.Failed != 10 {
		t.Fatalf("stats: %+v, want all 10 accounted for as failed", s)
	}
	// Late callers are turned away, not panicked at.
	d.HandleEvent(blockEvent("late"))
	d.NotifyURLs([]string{srv.URL}, Alert{RequestID: "late", Action: "BLOCK"})
	d.Stop(time.Second)
	if s := statsOf(d, KindGeneric); s.Dropped != 1 {
		t.Fatalf("stats: %+v, want the late event dropped", s)
	}
}

func TestDispatcher_StopDrainsWhatIsQueued(t *testing.T) {
	rcv := newReceiver(t, http.StatusOK)
	store := NewMemoryStore()
	mustCreate(t, store, Webhook{Kind: KindGeneric, URL: rcv.URL, Enabled: true})
	d := NewDispatcher(store, fastDispatch)
	for i := 0; i < 20; i++ {
		d.HandleEvent(blockEvent("req-n"))
	}
	d.Stop(5 * time.Second)
	if n := rcv.count(); n != 20 {
		t.Fatalf("%d delivered before Stop returned, want 20", n)
	}
}

func TestDispatcher_NotifyURLs(t *testing.T) {
	rcv := newReceiver(t, http.StatusOK)
	d := NewDispatcher(nil, fastDispatch)
	defer d.Stop(time.Second)

	// A placeholder nobody filled in is not a URL.
	d.NotifyURLs([]string{"${SLACK_WEBHOOK_URL}", "", rcv.URL + "/hook"}, Alert{
		RequestID: "req-9", Action: "REDACT", OrgID: "acme",
		Findings: []AlertFinding{{Type: "pii", Category: "email", Severity: "medium"}},
	})
	waitFor(t, "the notify", func() bool { return statsOf(d, KindRuleNotify).Sent == 1 })
	if s := statsOf(d, KindRuleNotify); s.Failed != 0 || s.Dropped != 0 {
		t.Fatalf("stats: %+v", s)
	}
	h := rcv.all()[0]
	if h.header.Get("Content-Type") != "application/json" || !strings.Contains(h.body, `"request_id":"req-9"`) ||
		!strings.Contains(h.body, `"text":"[Tamga] REDACT on input: pii/email (medium)`) {
		t.Fatalf("notify: %v %s", h.header, h.body)
	}
	// A bus event does nothing without a store to read destinations from.
	d.HandleEvent(blockEvent("req-1"))
}

// A webhook URL is a credential. Whatever goes wrong, the log names the kind
// and the destination id and never the URL's path or query, nor what the
// receiver answered.
func TestDispatcher_LogsNeverCarryTheURL(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	prev := log.Logger
	log.Logger = zerolog.New(lockedWriter{&mu, &buf}).Level(zerolog.DebugLevel)
	defer func() { log.Logger = prev }()

	bad := newReceiver(t, http.StatusInternalServerError)
	ok := newReceiver(t, http.StatusOK)
	gone := httptest.NewServer(http.NotFoundHandler())
	goneURL := gone.URL
	gone.Close()

	const secretPath = "/services/T000/B000/XXXXSECRETPATHXXXX"
	const secretQuery = "?token=XXXXSECRETQUERYXXXX"
	store := NewMemoryStore()
	w := mustCreate(t, store, Webhook{Kind: KindSlack, URL: bad.URL + secretPath + secretQuery, Enabled: true})
	mustCreate(t, store, Webhook{Kind: KindTeams, URL: goneURL + secretPath + secretQuery, Enabled: true})
	mustCreate(t, store, Webhook{Kind: KindGeneric, URL: ok.URL + secretPath + secretQuery, Enabled: true})
	mustCreate(t, store, Webhook{Kind: KindDatadog, URL: "http://bad host/" + secretPath + secretQuery, Enabled: true})
	d := NewDispatcher(store, fastDispatch)

	d.HandleEvent(blockEvent("req-1"))
	d.NotifyURLs([]string{goneURL + secretPath + secretQuery}, Alert{RequestID: "req-1", Action: "BLOCK"})
	waitFor(t, "every outcome", func() bool {
		return statsOf(d, KindSlack).Failed == 1 && statsOf(d, KindTeams).Failed == 1 && statsOf(d, KindDatadog).Failed == 1 &&
			statsOf(d, KindGeneric).Sent == 1 && statsOf(d, KindRuleNotify).Failed == 1
	})
	d.Stop(time.Second)
	// And the drop path.
	d.HandleEvent(blockEvent("req-2"))

	mu.Lock()
	out := buf.String()
	mu.Unlock()
	for _, bad := range []string{"SECRETPATH", "SECRETQUERY", "/services/", "token=", "RESPONSE_BODY_SECRET", liveSecret[:8], "zebrafish"} {
		if strings.Contains(out, bad) {
			t.Errorf("log carries %q:\n%s", bad, out)
		}
	}
	for _, want := range []string{`"destination_id":"` + w.ID + `"`, `"kind":"slack"`, `"kind":"rule_notify"`, `"error":"http_status"`, `"status":500`, `"error":"bad_url"`, `"reason":"stopped"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %s:\n%s", want, out)
		}
	}
}

type lockedWriter struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (w lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// A rule that redacts can match every request. Its URL is told once a minute
// about the same thing, and at once about a different one.
func TestDispatcher_NotifyURLsAreSpacedOut(t *testing.T) {
	rcv := newReceiver(t, http.StatusOK)
	d := NewDispatcher(nil, fastDispatch)
	defer d.Stop(time.Second)

	email := Alert{Action: "REDACT", OrgID: "acme", Findings: []AlertFinding{{Type: "pii", Category: "email", Severity: "medium"}}}
	for i := 0; i < 5; i++ {
		d.NotifyURLs([]string{rcv.URL + "/hook"}, email)
	}
	waitFor(t, "the first notify", func() bool { return statsOf(d, KindRuleNotify).Sent == 1 })
	if s := statsOf(d, KindRuleNotify); s.Suppressed != 4 {
		t.Fatalf("suppressed = %d, want 4: %+v", s.Suppressed, s)
	}

	block := Alert{Action: "BLOCK", OrgID: "acme", Findings: []AlertFinding{{Type: "secret", Category: "aws_access_key", Severity: "critical"}}}
	d.NotifyURLs([]string{rcv.URL + "/hook"}, block)
	waitFor(t, "the block", func() bool { return statsOf(d, KindRuleNotify).Sent == 2 })
}
