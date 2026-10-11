package api

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/store"
)

// With data.hash_findings on and no database, what the console reads from
// memory (the event list, the event detail, the export and the live feed)
// shows the digest of a match, as it does from the database, and neither the
// masked value nor what the metadata said of it.
func TestHashFindings_MemoryAndLiveFeedShowTheDigest(t *testing.T) {
	h := withFindingHashKey(t)
	const masked, cardMasked, bin = "10*******46", "453201******0366", "453201"
	hashing := func() *policy.Policy { return &policy.Policy{Data: &policy.DataControl{HashFindings: true}} }

	recent := events.NewRecentBuffer(8)
	broker := events.NewBroker(8)
	bus := events.NewBus()
	bus.SetPrepare(store.EventFindingHasher(zerolog.Nop(), hashing))
	bus.Subscribe(events.RecentBufferHandler(recent))
	bus.Subscribe(broker.Publish)
	bus.Start()
	defer bus.Stop()

	cfg := Config{AdminKey: "test-key", Recent: recent, Broker: broker, DefaultOrgID: "org-1", Started: time.Now()}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()
	get := func(ctx context.Context, path string) *http.Response {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+path, nil)
		adminHeaders(cfg.AdminKey)(req)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d", path, resp.StatusCode)
		}
		return resp
	}

	// The live feed is opened first: it sends what is published after.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	live := get(ctx, "/api/v1/live/events")
	defer func() { _ = live.Body.Close() }()
	for deadline := time.Now().Add(5 * time.Second); broker.Size() == 0; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the live feed did not subscribe")
		}
	}

	bus.Publish(events.Event{
		RequestID: "evt-hash-1",
		OrgID:     "org-1",
		EventType: "request_blocked",
		Action:    "BLOCK",
		Provider:  "openai",
		Model:     "gpt-4o",
		Findings: []scanner.Finding{
			{Type: "pii", Category: "tc_kimlik", Severity: "critical", Match: masked},
			{Type: "pii", Category: "credit_card", Severity: "critical", Match: cardMasked,
				Metadata: map[string]string{"bin": bin, "bank_name": "Test Bankasi", "card_brand": "visa"}},
		},
	})

	var liveData string
	sc := bufio.NewScanner(live.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		if line := sc.Text(); strings.HasPrefix(line, "data: ") {
			liveData = line
			break
		}
	}
	if liveData == "" {
		t.Fatalf("the live feed sent no event: %v", sc.Err())
	}

	read := func(path string) string {
		resp := get(context.Background(), path)
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	want := h.Hash(masked)
	for name, body := range map[string]string{
		"the live feed":    liveData,
		"the event list":   read("/api/v1/events"),
		"the event detail": read("/api/v1/events/evt-hash-1"),
		"the export":       read("/api/v1/events/export?format=json"),
	} {
		if !strings.Contains(body, want) || !strings.Contains(body, h.Hash(cardMasked)) {
			t.Errorf("%s does not show the digests: %s", name, body)
		}
		for _, leak := range []string{masked, cardMasked, bin, "Test Bankasi"} {
			if strings.Contains(body, leak) {
				t.Errorf("%s holds %q: %s", name, leak, body)
			}
		}
	}
	// The finding is still told apart by what kind it is.
	if detail := read("/api/v1/events/evt-hash-1"); !strings.Contains(detail, `"category":"tc_kimlik"`) {
		t.Errorf("the event detail lost the category: %s", detail)
	}
}
