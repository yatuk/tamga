package webhooks

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/egress/egresstest"
)

// loopbackGuard is the guard the tests that talk to httptest servers ask
// for: the strict default, plus loopback on purpose.
func loopbackGuard() *Guard {
	return NewGuard(GuardConfig{AllowNets: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
}

func loopbackStore() *MemoryStore {
	s := NewMemoryStore()
	s.UseGuard(loopbackGuard())
	return s
}

// storeUnchecked puts a destination in the working copy the way a load from
// the database does: without the check Create makes.
func storeUnchecked(s *MemoryStore, w Webhook) {
	s.mu.Lock()
	s.data[w.ID] = w
	s.mu.Unlock()
}

func TestStore_CreateAndUpdateRejectAtSaveTime(t *testing.T) {
	s := NewMemoryStore()
	for _, u := range []string{
		"https://127.0.0.1/hook",
		"https://10.0.0.8:5432/",
		"https://169.254.169.254/latest/meta-data/",
		"https://[::ffff:7f00:1]/",
		"https://localhost/hook",
		"http://hooks.example.com/services/x",
		"ftp://example.com/x",
		"file:///etc/passwd",
		"https://2130706433/",
		"https://example.com/ hook",
		"not a url",
	} {
		if _, err := s.Create(Webhook{Label: "x", URL: u}); err == nil {
			t.Errorf("Create accepted %q", u)
		}
	}
	if n := len(s.List()); n != 0 {
		t.Fatalf("%d refused destinations were stored", n)
	}

	w, err := s.Create(Webhook{Label: "ok", URL: "https://hooks.example.com/services/x", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{
		"http://127.0.0.1/", "http://169.254.169.254/", "https://127.0.0.1/", "https://169.254.169.254/",
		"https://192.168.1.1/", "https://[::ffff:7f00:1]/", "https://2130706433/", "https://localhost/",
		"http://hooks.example.com/services/x", "gopher://example.com/", "not a url",
	} {
		_, errUpdate := s.Update(w.ID, Webhook{URL: u, Enabled: true})
		_, errCreate := s.Create(Webhook{URL: u})
		if errUpdate == nil || errCreate == nil || errUpdate.Error() != errCreate.Error() {
			t.Errorf("%q: Update says %v, Create says %v; want the same refusal", u, errUpdate, errCreate)
		}
	}
	if got, _ := s.Get(w.ID); got.URL != w.URL {
		t.Fatalf("a refused update changed the URL to %q", got.URL)
	}
	if _, err := s.Update(w.ID, Webhook{URL: "https://hooks.example.com/services/y", Enabled: true}); err != nil {
		t.Fatalf("a good update was refused: %v", err)
	}

	// A destination saved before the check existed can still be switched
	// off, as long as its URL is left alone.
	storeUnchecked(s, Webhook{ID: "legacy", URL: "http://10.0.0.8/hook", Enabled: true})
	got, err := s.Update("legacy", Webhook{URL: "http://10.0.0.8/hook", Enabled: false, Label: "old"})
	if err != nil || got.Enabled {
		t.Fatalf("switching a legacy destination off: %+v %v", got, err)
	}
}

// captureLog swaps the global logger for one that writes to a buffer.
func captureLog(t *testing.T) func() string {
	t.Helper()
	var buf bytes.Buffer
	var mu sync.Mutex
	prev := log.Logger
	log.Logger = zerolog.New(lockedWriter{&mu, &buf}).Level(zerolog.DebugLevel)
	t.Cleanup(func() { log.Logger = prev })
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		return buf.String()
	}
}

// The probe gives one answer for everything the guard refuses, and the same
// answer for a name that does not exist, so it cannot be used to find out
// what is on the internal network or what resolves there.
func TestStore_ProbeGivesOneFixedOutcome(t *testing.T) {
	logged := captureLog(t)
	redirector := egresstest.NewServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://db.internal.example/x", http.StatusTemporaryRedirect)
	})
	res := egresstest.NewResolver().
		Add("db.internal.example", "10.0.0.8").
		Add("meta.example", "169.254.169.254").
		Add("mixed.example", "93.184.216.34", "10.0.0.8").
		Add("front.example", "127.0.0.1")
	rec := &egresstest.DialRecorder{Routes: map[string]string{"127.0.0.1:" + redirector.Port(): redirector.Addr()}}
	s := NewMemoryStore()
	// front.example is allowed so that a reachable receiver can redirect.
	s.UseGuard(NewGuard(GuardConfig{AllowHosts: []string{"front.example"}, Resolver: res, Dial: rec.Dial}))
	d := NewDispatcher(s, DispatcherConfig{})
	defer d.Stop(time.Second)

	const secret = "/services/T000/SECRETPATH?token=SECRETQUERY"
	dests := map[string]string{
		"literal":      "https://10.0.0.8:5432" + secret,
		"loopback":     "http://127.0.0.1:" + redirector.Port() + secret,
		"private-name": "https://db.internal.example" + secret,
		"metadata":     "https://meta.example" + secret,
		"mixed":        "https://mixed.example" + secret,
		"no-such-name": "https://nxdomain.internal.example" + secret,
		"redirects":    "http://front.example:" + redirector.Port() + secret,
	}
	var answers []string
	for id, u := range dests {
		storeUnchecked(s, Webhook{ID: id, Kind: KindSlack, URL: u, Enabled: true})
		start := time.Now()
		status, err := s.Test(context.Background(), id)
		if status != 0 || err != ErrDestinationNotAllowed {
			t.Errorf("%s: status %d, error %v; want exactly the fixed outcome", id, status, err)
		}
		if took := time.Since(start); took > time.Second {
			t.Errorf("%s: the refusal took %v", id, took)
		}
		if err != nil {
			answers = append(answers, err.Error())
		}
	}
	for _, a := range answers {
		if a != "destination address is not allowed" {
			t.Errorf("answer %q", a)
		}
	}
	for _, addr := range rec.All() {
		if addr != "127.0.0.1:"+redirector.Port() {
			t.Errorf("the probe dialled %s", addr)
		}
	}
	if n := len(redirector.All()); n != 1 {
		t.Errorf("the redirecting receiver was reached %d times, want once (by its allowlisted name)", n)
	}
	// Six refusals by the guard; the name that does not resolve is not one.
	if got := statsOf(d, KindSlack).Blocked; got != 6 {
		t.Errorf("blocked counter = %d, want 6", got)
	}
	out := logged()
	for _, bad := range []string{"SECRET", "/services/", "token=", "10.0.0.8", "127.0.0.1", "internal.example", "meta.example", "front.example", "169.254"} {
		if strings.Contains(out, bad) {
			t.Errorf("log carries %q:\n%s", bad, out)
		}
	}
	for _, want := range []string{`"error":"blocked"`, `"kind":"slack"`, `"destination_id":"literal"`, `"destination_id":"redirects"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %s:\n%s", want, out)
		}
	}

	// A legacy plain http destination on a public host is told about the
	// scheme: that is the caller's own input, not something learned.
	storeUnchecked(s, Webhook{ID: "plain", Kind: KindTeams, URL: "http://hooks.example.com/x", Enabled: true})
	if _, err := s.Test(context.Background(), "plain"); err != ErrInsecureScheme {
		t.Errorf("plain http: %v", err)
	}
	if n := res.Lookups(); n > 6 {
		t.Errorf("%d lookups", n)
	}
}

// A failure that is not the guard's is reported as a class, never with the
// transport's text, which names the URL and the address.
func TestStore_ProbeErrorsCarryNoURL(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	u := gone.URL + "/services/SECRETPATH"
	gone.Close()
	s := loopbackStore()
	w, err := s.Create(Webhook{Kind: KindGeneric, URL: u})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Test(context.Background(), w.ID)
	if err == nil || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("probe error: %v", err)
	}
	if err.Error() != "test failed: network" {
		t.Errorf("probe error = %q", err.Error())
	}
}

// Live delivery to a refused destination: counted as blocked, not retried,
// nothing connected to, and the log names the kind and the id only.
func TestDispatcher_BlockedDestinations(t *testing.T) {
	logged := captureLog(t)
	// A real listener on loopback that must never be reached.
	victim := newReceiver(t, http.StatusOK)
	res := egresstest.NewResolver().Add("db.internal.example", "10.0.0.8").Add("rebind.example", "93.184.216.34").Add("rebind.example", "127.0.0.1")
	rec := &egresstest.DialRecorder{}
	guard := NewGuard(GuardConfig{Resolver: res, Dial: rec.Dial})
	s := NewMemoryStore()
	s.UseGuard(guard)
	const secret = "/services/T000/SECRETPATH?token=SECRETQUERY"
	storeUnchecked(s, Webhook{ID: "literal", Kind: KindSlack, URL: victim.URL + secret, Enabled: true})
	storeUnchecked(s, Webhook{ID: "named", Kind: KindTeams, URL: "https://db.internal.example" + secret, Enabled: true})
	d := NewDispatcher(s, DispatcherConfig{DefaultOrg: "default", Backoff: 5 * time.Millisecond, Guard: guard})

	d.HandleEvent(blockEvent("req-1"))
	d.NotifyURLs([]string{
		victim.URL + secret,                       // plain http to loopback
		"https://169.254.169.254/latest" + secret, // metadata
		"https://db.internal.example" + secret,    // a name that resolves inside
	}, Alert{RequestID: "req-1", OrgID: "acme", Action: "BLOCK"})
	waitFor(t, "the refusals", func() bool {
		return statsOf(d, KindSlack).Blocked == 1 && statsOf(d, KindTeams).Blocked == 1 && statsOf(d, KindRuleNotify).Blocked == 3
	})
	d.Stop(time.Second)

	for _, k := range []Kind{KindSlack, KindTeams, KindRuleNotify} {
		if st := statsOf(d, k); st.Sent != 0 || st.Failed != 0 || st.Skipped != 0 {
			t.Errorf("%s: %+v, want blocked only", k, st)
		}
	}
	if n := victim.count(); n != 0 {
		t.Errorf("the loopback listener was reached %d times", n)
	}
	if got := rec.All(); len(got) != 0 {
		t.Errorf("connections were attempted: %v", got)
	}
	// Not retried: one lookup for each of the two deliveries to the name.
	if n := res.Lookups(); n != 2 {
		t.Errorf("%d lookups, want 2", n)
	}
	out := logged()
	for _, bad := range []string{"SECRET", "/services/", "token=", "127.0.0.1", "169.254", "internal.example", `"host"`} {
		if strings.Contains(out, bad) {
			t.Errorf("log carries %q:\n%s", bad, out)
		}
	}
	for _, want := range []string{`"error":"blocked"`, `"kind":"slack"`, `"destination_id":"literal"`, `"destination_id":"named"`, `"kind":"rule_notify"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %s:\n%s", want, out)
		}
	}
}

// A destination whose name is public when it is saved and tested, and
// internal by the time an alert is sent.
func TestDispatcher_RebindingBetweenProbeAndDelivery(t *testing.T) {
	res := egresstest.NewResolver().Add("rebind.example", "93.184.216.34").Add("rebind.example", "169.254.169.254")
	rec := &egresstest.DialRecorder{}
	guard := NewGuard(GuardConfig{Resolver: res, Dial: rec.Dial})
	s := NewMemoryStore()
	s.UseGuard(guard)
	w, err := s.Create(Webhook{Kind: KindGeneric, URL: "https://rebind.example/hook", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	// The probe goes to the public address (and fails there: the recorder
	// connects to nothing).
	if _, err := s.Test(context.Background(), w.ID); err == nil || err == ErrDestinationNotAllowed {
		t.Fatalf("probe: %v", err)
	}
	d := NewDispatcher(s, DispatcherConfig{DefaultOrg: "default", Guard: guard})
	defer d.Stop(time.Second)
	d.HandleEvent(blockEvent("req-1"))
	waitFor(t, "the refusal", func() bool { return statsOf(d, KindGeneric).Blocked == 1 })
	if got := rec.All(); len(got) != 1 || got[0] != "93.184.216.34:443" {
		t.Errorf("dialled %v, want only the public address the probe was judged on", got)
	}
}
