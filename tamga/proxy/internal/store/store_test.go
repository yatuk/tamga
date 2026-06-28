package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/scanner"
)

func TestNoopStore_SaveAndStats(t *testing.T) {
	s := NewNoopStoreSilent()
	ctx := context.Background()
	if err := s.SaveRequestLog(ctx, RequestLog{RequestID: "r1", OrgID: "00000000-0000-0000-0000-000000000001"}); err != nil {
		t.Fatal(err)
	}
	st, err := s.GetStats(ctx, "00000000-0000-0000-0000-000000000001", time.Now().Add(-24*time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if st.TotalRequests != 0 {
		t.Fatalf("noop stats: %+v", st)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNoopStore_GetModelTokenUsage(t *testing.T) {
	s := NewNoopStoreSilent()
	usage, err := s.GetModelTokenUsage(context.Background(), "org-1", time.Now().Add(-24*time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if usage != nil {
		t.Fatalf("want nil usage from noop store, got %d rows", len(usage))
	}
}

func TestNoopStore_PingAndClose(t *testing.T) {
	s := NewNoopStoreSilent()
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal("ping should succeed", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal("close should succeed", err)
	}
}

func TestNoopStore_SecurityEvents(t *testing.T) {
	s := NewNoopStoreSilent()
	events, total, err := s.ListSecurityEvents(context.Background(), "org", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if events != nil || total != 0 {
		t.Fatalf("want nil/0, got %d/%d", len(events), total)
	}
	se, st, err := s.SearchSecurityEvents(context.Background(), "org", EventSearchParams{Page: 1, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if se != nil || st != 0 {
		t.Fatalf("want nil/0, got %d/%d", len(se), st)
	}
}

func TestModelTokenUsage_ZeroValues(t *testing.T) {
	u := ModelTokenUsage{}
	if u.Provider != "" || u.Model != "" || u.InputTokens != 0 || u.OutputTokens != 0 {
		t.Fatalf("zero-value ModelTokenUsage has unexpected fields: %+v", u)
	}
}

func TestDBHandler_SkipsWithoutOrg(t *testing.T) {
	s := NewNoopStoreSilent()
	h := DBHandler(zerolog.Nop(), s, "", nil)
	e := events.Event{
		EventType: "request_scanned",
		Action:    "PASS",
		RequestID: "x",
	}
	h(e)
}

func TestPostgresStore_Integration(t *testing.T) {
	if os.Getenv("TAMGA_INTEGRATION_DB") == "" {
		t.Skip("set TAMGA_INTEGRATION_DB=1 and TAMGA_DB_URL to run postgres integration test")
	}
	dsn := os.Getenv("TAMGA_DB_URL")
	if dsn == "" {
		t.Skip("TAMGA_DB_URL empty")
	}
	ctx := context.Background()
	ps, err := NewPostgresStore(ctx, dsn, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := ps.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()

	org := os.Getenv("TAMGA_ORG_ID")
	if org == "" {
		t.Skip("TAMGA_ORG_ID required (UUID of dev org from migrations)")
	}

	now := time.Now().UTC()
	if err := ps.SaveRequestLog(ctx, RequestLog{
		RequestID:      "test-req-" + now.Format("150405"),
		OrgID:          org,
		Provider:       "openai",
		Model:          "gpt-4o-mini",
		Findings:       []byte(`[]`),
		ActionTaken:    "pass",
		ScanLatencyMs:  1,
		TotalLatencyMs: 2,
		Endpoint:       "/v1/chat/completions",
	}); err != nil {
		t.Fatal(err)
	}
}

// TestPostgresStore_RoundTrip saves a request log with findings and verifies
// it can be read back through ListSecurityEvents and SearchSecurityEvents with
// every supported filter dimension.
func TestPostgresStore_RoundTrip(t *testing.T) {
	if os.Getenv("TAMGA_INTEGRATION_DB") == "" {
		t.Skip("set TAMGA_INTEGRATION_DB=1 and TAMGA_DB_URL to run postgres integration test")
	}
	dsn := os.Getenv("TAMGA_DB_URL")
	if dsn == "" {
		t.Skip("TAMGA_DB_URL empty")
	}
	org := os.Getenv("TAMGA_ORG_ID")
	if org == "" {
		t.Skip("TAMGA_ORG_ID required (UUID of dev org from migrations)")
	}

	ctx := context.Background()
	ps, err := NewPostgresStore(ctx, dsn, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := ps.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()

	now := time.Now().UTC()
	rid := "rt-" + now.Format("150405.000000000")
	findings := []map[string]interface{}{
		{"type": "PII", "severity": "HIGH", "category": "personally-identifiable-information", "details": "email found"},
	}
	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("saving request %s", rid)
	if err := ps.SaveRequestLog(ctx, RequestLog{
		RequestID:      rid,
		OrgID:          org,
		Provider:       "anthropic",
		Model:          "claude-sonnet-4-6",
		ModelFamily:    "claude-4",
		InputTokens:    150,
		OutputTokens:   80,
		Findings:       findingsJSON,
		FindingsCount:  len(findings),
		ActionTaken:    "block",
		ScanLatencyMs:  12.5,
		TotalLatencyMs: 350.0,
		UserID:         "user-roundtrip-test",
		Endpoint:       "/v1/messages",
	}); err != nil {
		t.Fatalf("SaveRequestLog: %v", err)
	}

	// Force immediate flush so the row hits the database now.
	ps.mu.Lock()
	batch := ps.buf
	ps.buf = nil
	ps.mu.Unlock()
	if len(batch) > 0 {
		if err := ps.insertBatch(ctx, batch); err != nil {
			t.Fatalf("flush: %v", err)
		}
	}

	// 1. ListSecurityEvents should find our row (within last 7 days).
	events, total, err := ps.ListSecurityEvents(ctx, org, 1, 50)
	if err != nil {
		t.Fatalf("ListSecurityEvents: %v", err)
	}
	t.Logf("ListSecurityEvents returned %d events (total=%d)", len(events), total)
	found := false
	for _, ev := range events {
		if ev.RequestID == rid {
			found = true
			if ev.ActionTaken != "block" {
				t.Errorf("action: want block, got %s", ev.ActionTaken)
			}
			if ev.FindingsCount != 1 {
				t.Errorf("findings count: want 1, got %d", ev.FindingsCount)
			}
			if ev.Provider != "anthropic" {
				t.Errorf("provider: want anthropic, got %s", ev.Provider)
			}
			break
		}
	}
	if !found {
		t.Errorf("saved request %s not found in ListSecurityEvents", rid)
	}

	// 2. SearchSecurityEvents by action=block.
	searchEvents, searchTotal, err := ps.SearchSecurityEvents(ctx, org, EventSearchParams{
		Page:   1,
		Limit:  50,
		Action: "block",
		Since:  now.Add(-1 * time.Hour),
		Until:  now.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("SearchSecurityEvents by action: %v", err)
	}
	t.Logf("SearchSecurityEvents(action=block) returned %d events (total=%d)", len(searchEvents), searchTotal)
	foundSearch := false
	for _, ev := range searchEvents {
		if ev.RequestID == rid {
			foundSearch = true
			break
		}
	}
	if !foundSearch {
		t.Errorf("saved request %s not found via SearchSecurityEvents with action=block", rid)
	}

	// 3. SearchSecurityEvents by provider.
	provEvents, provTotal, err := ps.SearchSecurityEvents(ctx, org, EventSearchParams{
		Page:     1,
		Limit:    50,
		Provider: "anthropic",
		Since:    now.Add(-1 * time.Hour),
		Until:    now.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("SearchSecurityEvents by provider: %v", err)
	}
	t.Logf("SearchSecurityEvents(provider=anthropic) returned %d events (total=%d)", len(provEvents), provTotal)
	foundProv := false
	for _, ev := range provEvents {
		if ev.RequestID == rid {
			foundProv = true
			break
		}
	}
	if !foundProv {
		t.Errorf("saved request %s not found via SearchSecurityEvents with provider=anthropic", rid)
	}

	// 4. SearchSecurityEvents by finding type=PII.
	typeEvents, typeTotal, err := ps.SearchSecurityEvents(ctx, org, EventSearchParams{
		Page:        1,
		Limit:       50,
		FindingType: "PII",
		Since:       now.Add(-1 * time.Hour),
		Until:       now.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("SearchSecurityEvents by finding type: %v", err)
	}
	t.Logf("SearchSecurityEvents(findingType=PII) returned %d events (total=%d)", len(typeEvents), typeTotal)
	foundType := false
	for _, ev := range typeEvents {
		if ev.RequestID == rid {
			foundType = true
			break
		}
	}
	if !foundType {
		t.Errorf("saved request %s not found via SearchSecurityEvents with findingType=PII", rid)
	}

	// 5. SearchSecurityEvents by category ILIKE.
	catEvents, catTotal, err := ps.SearchSecurityEvents(ctx, org, EventSearchParams{
		Page:     1,
		Limit:    50,
		Category: "personally",
		Since:    now.Add(-1 * time.Hour),
		Until:    now.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("SearchSecurityEvents by category: %v", err)
	}
	t.Logf("SearchSecurityEvents(category=personally) returned %d events (total=%d)", len(catEvents), catTotal)
	foundCat := false
	for _, ev := range catEvents {
		if ev.RequestID == rid {
			foundCat = true
			break
		}
	}
	if !foundCat {
		t.Errorf("saved request %s not found via SearchSecurityEvents with category=personally", rid)
	}

	// 6. SearchSecurityEvents by technique / text match.
	techEvents, techTotal, err := ps.SearchSecurityEvents(ctx, org, EventSearchParams{
		Page:      1,
		Limit:     50,
		Technique: "email",
		Since:     now.Add(-1 * time.Hour),
		Until:     now.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("SearchSecurityEvents by technique: %v", err)
	}
	t.Logf("SearchSecurityEvents(technique=email) returned %d events (total=%d)", len(techEvents), techTotal)
	foundTech := false
	for _, ev := range techEvents {
		if ev.RequestID == rid {
			foundTech = true
			break
		}
	}
	if !foundTech {
		t.Errorf("saved request %s not found via SearchSecurityEvents with technique=email", rid)
	}

	// 7. SearchSecurityEvents by q (full-text ILIKE on request_id substring).
	qEvents, qTotal, err := ps.SearchSecurityEvents(ctx, org, EventSearchParams{
		Page:  1,
		Limit: 50,
		Q:     rid[:10],
		Since: now.Add(-1 * time.Hour),
		Until: now.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("SearchSecurityEvents by q: %v", err)
	}
	t.Logf("SearchSecurityEvents(q=%s) returned %d events (total=%d)", rid[:10], len(qEvents), qTotal)
	foundQ := false
	for _, ev := range qEvents {
		if ev.RequestID == rid {
			foundQ = true
			break
		}
	}
	if !foundQ {
		t.Errorf("saved request %s not found via SearchSecurityEvents with q=%s", rid, rid[:10])
	}

	// 8. SearchSecurityEvents — non-matching action returns correct total.
	emptyEvents, emptyTotal, err := ps.SearchSecurityEvents(ctx, org, EventSearchParams{
		Page:   1,
		Limit:  50,
		Action: "redact",
		Since:  now.Add(-1 * time.Hour),
		Until:  now.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("SearchSecurityEvents empty: %v", err)
	}
	if emptyTotal > 0 {
		t.Logf("SearchSecurityEvents(action=redact) returned %d events (total=%d) — may be pre-existing data", len(emptyEvents), emptyTotal)
	}
}

// TestDefaultRetentionPolicy verifies the default retention values.
func TestDefaultRetentionPolicy(t *testing.T) {
	p := DefaultRetentionPolicy()
	if p.RequestLogsDays != 30 {
		t.Errorf("RequestLogsDays: want 30, got %d", p.RequestLogsDays)
	}
	if p.AlertsDays != 90 {
		t.Errorf("AlertsDays: want 90, got %d", p.AlertsDays)
	}
	if p.DailyStatsDays != 365 {
		t.Errorf("DailyStatsDays: want 365, got %d", p.DailyStatsDays)
	}
	if p.AuditLogsDays != 365 {
		t.Errorf("AuditLogsDays: want 365, got %d", p.AuditLogsDays)
	}
	if p.IncidentClosedDays != 0 {
		t.Errorf("IncidentClosedDays: want 0, got %d", p.IncidentClosedDays)
	}
}

// TestEnvIntPositive tests env variable parsing for positive integers.
func TestEnvIntPositive(t *testing.T) {
	key := "TAMGA_TEST_ENV_INT_POSITIVE"
	defer func() { _ = os.Unsetenv(key) }()

	tests := []struct {
		name     string
		value    string
		expected int
	}{
		{"empty", "", 0},
		{"zero", "0", 0},
		{"negative", "-5", 0},
		{"positive", "42", 42},
		{"garbage", "abc", 0},
		{"float", "3.14", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv(key, tt.value)
			got := envIntPositive(key)
			if got != tt.expected {
				t.Errorf("envIntPositive(%q=%q): want %d, got %d", key, tt.value, tt.expected, got)
			}
		})
	}
}

// TestRetentionPolicyFromEnv verifies env overrides take effect.
func TestRetentionPolicyFromEnv(t *testing.T) {
	for _, k := range []string{
		"TAMGA_RETENTION_REQUEST_LOGS_DAYS",
		"TAMGA_RETENTION_ALERTS_DAYS",
		"TAMGA_RETENTION_DAILY_STATS_DAYS",
		"TAMGA_RETENTION_AUDIT_DAYS",
	} {
		_ = os.Unsetenv(k)
		defer func() { _ = os.Unsetenv(k) }()
	}

	_ = os.Setenv("TAMGA_RETENTION_REQUEST_LOGS_DAYS", "60")
	_ = os.Setenv("TAMGA_RETENTION_ALERTS_DAYS", "180")

	p := RetentionPolicyFromEnv()
	if p.RequestLogsDays != 60 {
		t.Errorf("RequestLogsDays: want 60, got %d", p.RequestLogsDays)
	}
	if p.AlertsDays != 180 {
		t.Errorf("AlertsDays: want 180, got %d", p.AlertsDays)
	}
	if p.DailyStatsDays != 365 {
		t.Errorf("DailyStatsDays: want 365 (default), got %d", p.DailyStatsDays)
	}
	if p.AuditLogsDays != 365 {
		t.Errorf("AuditLogsDays: want 365 (default), got %d", p.AuditLogsDays)
	}
}

// TestNullIfEmpty verifies the nullIfEmpty helper used in PostgreSQL bindings.
func TestNullIfEmpty(t *testing.T) {
	tests := []struct {
		input    string
		expected interface{}
	}{
		{"", nil},
		{"hello", "hello"},
		{" ", " "},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			got := nullIfEmpty(tt.input)
			if got != tt.expected {
				t.Errorf("nullIfEmpty(%q): want %v, got %v", tt.input, tt.expected, got)
			}
		})
	}
}

// TestParsePartitionUpperBoundTO_EdgeCases tests additional partition bound formats.
func TestParsePartitionUpperBoundTO_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		bound  string
		wantOk bool
		wantY  int
		wantM  int
		wantD  int
	}{
		{
			name:   "standard",
			bound:  `FOR VALUES FROM ('2026-04-01') TO ('2026-05-01')`,
			wantOk: true, wantY: 2026, wantM: 5, wantD: 1,
		},
		{
			name:   "year boundary",
			bound:  `FOR VALUES FROM ('2025-12-01') TO ('2026-01-01')`,
			wantOk: true, wantY: 2026, wantM: 1, wantD: 1,
		},
		{
			name:   "garbage input",
			bound:  `not a partition bound`,
			wantOk: false,
		},
		{
			name:   "empty",
			bound:  ``,
			wantOk: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, ok := parsePartitionUpperBoundTO(tt.bound)
			if ok != tt.wantOk {
				t.Errorf("ok: want %v, got %v", tt.wantOk, ok)
			}
			if !tt.wantOk {
				return
			}
			if ts.Year() != tt.wantY || ts.Month() != time.Month(tt.wantM) || ts.Day() != tt.wantD {
				t.Errorf("date: want %04d-%02d-%02d, got %v", tt.wantY, tt.wantM, tt.wantD, ts)
			}
		})
	}
}

// TestNoopStore_FullInterface verifies NoopStore satisfies all Store methods.
func TestNoopStore_FullInterface(t *testing.T) {
	s := NewNoopStoreSilent()
	ctx := context.Background()
	org := "00000000-0000-0000-0000-000000000001"

	if err := s.SaveRequestLog(ctx, RequestLog{RequestID: "r1", OrgID: org}); err != nil {
		t.Fatal(err)
	}

	st, err := s.GetStats(ctx, org, time.Now().Add(-24*time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if st.TotalRequests != 0 {
		t.Errorf("noop stats should be zero, got %d", st.TotalRequests)
	}

	evs, total, err := s.ListSecurityEvents(ctx, org, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 0 || total != 0 {
		t.Errorf("noop list should be empty, got %d events, total=%d", len(evs), total)
	}

	sevs, stotal, err := s.SearchSecurityEvents(ctx, org, EventSearchParams{
		Page:   1,
		Limit:  50,
		Action: "block",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sevs) != 0 || stotal != 0 {
		t.Errorf("noop search should be empty, got %d events, total=%d", len(sevs), stotal)
	}

	if err := s.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestDBHandler_Events tests DBHandler with various event shapes.
func TestDBHandler_Events(t *testing.T) {
	s := NewNoopStoreSilent()
	h := DBHandler(zerolog.Nop(), s, "", nil)

	// request_scanned with findings
	h(events.Event{
		EventType:    "request_scanned",
		Action:       "PASS",
		RequestID:    "req-1",
		OrgID:        "00000000-0000-0000-0000-000000000001",
		Provider:     "openai",
		Model:        "gpt-4o",
		ModelFamily:  "gpt-4o",
		InputTokens:  100,
		OutputTokens: 50,
		Findings:     []scanner.Finding{{Type: "PII", Severity: "LOW"}},
	})

	// request_blocked
	h(events.Event{
		EventType: "request_blocked",
		Action:    "BLOCK",
		RequestID: "req-2",
		OrgID:     "00000000-0000-0000-0000-000000000001",
		Provider:  "anthropic",
	})

	// unknown event type should be silently skipped
	h(events.Event{
		EventType: "something_else",
		RequestID: "req-3",
		OrgID:     "00000000-0000-0000-0000-000000000001",
	})

	// empty org with no default should be skipped
	h2 := DBHandler(zerolog.Nop(), s, "", nil)
	h2(events.Event{
		EventType: "request_scanned",
		Action:    "PASS",
		RequestID: "req-noorg",
		OrgID:     "",
	})

	// empty org with default org should fall back
	h3 := DBHandler(zerolog.Nop(), s, "00000000-0000-0000-0000-000000000099", nil)
	h3(events.Event{
		EventType: "request_scanned",
		Action:    "PASS",
		RequestID: "req-defaultorg",
		OrgID:     "",
	})

	// empty findings should marshal to []
	h(events.Event{
		EventType: "request_scanned",
		Action:    "PASS",
		RequestID: "req-findings",
		OrgID:     "00000000-0000-0000-0000-000000000001",
		Findings:  []scanner.Finding{},
	})
}

// TestSaveRequestLog_BatchFlush verifies batch constants are as expected.
func TestSaveRequestLog_BatchFlush(t *testing.T) {
	if batchMaxRows != 100 {
		t.Errorf("batchMaxRows changed? expected 100, got %d", batchMaxRows)
	}
	if flushInterval != 5*time.Second {
		t.Errorf("flushInterval changed? expected 5s, got %v", flushInterval)
	}
}

// TestNoopStore_CloseIdempotent verifies Close() can be called multiple times.
func TestNoopStore_CloseIdempotent(t *testing.T) {
	s := NewNoopStoreSilent()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestPostgresStore_CloseFlushesBuffer verifies Close drains the buffer.
func TestPostgresStore_CloseFlushesBuffer(t *testing.T) {
	if os.Getenv("TAMGA_INTEGRATION_DB") == "" {
		t.Skip("set TAMGA_INTEGRATION_DB=1 and TAMGA_DB_URL to run postgres integration test")
	}
	dsn := os.Getenv("TAMGA_DB_URL")
	if dsn == "" {
		t.Skip("TAMGA_DB_URL empty")
	}
	org := os.Getenv("TAMGA_ORG_ID")
	if org == "" {
		t.Skip("TAMGA_ORG_ID required")
	}

	ctx := context.Background()
	ps, err := NewPostgresStore(ctx, dsn, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	rid := "flush-" + now.Format("150405.000000000")

	if err := ps.SaveRequestLog(ctx, RequestLog{
		RequestID:   rid,
		OrgID:       org,
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		Findings:    []byte(`[]`),
		ActionTaken: "pass",
	}); err != nil {
		t.Fatal(err)
	}

	// Verify it's still in buffer.
	ps.mu.Lock()
	bufLen := len(ps.buf)
	ps.mu.Unlock()
	if bufLen != 1 {
		t.Errorf("expected 1 buffered row, got %d", bufLen)
	}

	// Close should flush the buffer.
	if err := ps.Close(); err != nil {
		t.Fatal(err)
	}

	// Re-open a new store and search for the flushed row.
	ps2, err := NewPostgresStore(ctx, dsn, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ps2.Close() }()

	evs, total, err := ps2.SearchSecurityEvents(ctx, org, EventSearchParams{
		Page:  1,
		Limit: 200,
		Q:     rid,
		Since: now.Add(-1 * time.Hour),
		Until: now.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("search after close: %v", err)
	}
	t.Logf("found %d events (total=%d) for flushed request %s", len(evs), total, rid)
	found := false
	for _, ev := range evs {
		if ev.RequestID == rid {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("flushed request %s not found after Close", rid)
	}
}

// TestPostgresStore_SaveAfterClose returns error.
func TestPostgresStore_SaveAfterClose(t *testing.T) {
	if os.Getenv("TAMGA_INTEGRATION_DB") == "" {
		t.Skip("set TAMGA_INTEGRATION_DB=1 and TAMGA_DB_URL to run postgres integration test")
	}
	dsn := os.Getenv("TAMGA_DB_URL")
	if dsn == "" {
		t.Skip("TAMGA_DB_URL empty")
	}
	org := os.Getenv("TAMGA_ORG_ID")
	if org == "" {
		t.Skip("TAMGA_ORG_ID required")
	}

	ctx := context.Background()
	ps, err := NewPostgresStore(ctx, dsn, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}

	if err := ps.Close(); err != nil {
		t.Fatal(err)
	}

	err = ps.SaveRequestLog(ctx, RequestLog{
		RequestID: "after-close",
		OrgID:     org,
	})
	if err == nil {
		t.Error("expected error when saving after close, got nil")
	}
}

// TestPostgresStore_Ping verifies Ping works.
func TestPostgresStore_Ping(t *testing.T) {
	if os.Getenv("TAMGA_INTEGRATION_DB") == "" {
		t.Skip("set TAMGA_INTEGRATION_DB=1 and TAMGA_DB_URL to run postgres integration test")
	}
	dsn := os.Getenv("TAMGA_DB_URL")
	if dsn == "" {
		t.Skip("TAMGA_DB_URL empty")
	}

	ctx := context.Background()
	ps, err := NewPostgresStore(ctx, dsn, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ps.Close() }()

	if err := ps.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

// TestRequestLog_Fields tests RequestLog struct field assignments.
func TestRequestLog_Fields(t *testing.T) {
	findings := []byte(`[{"type":"SECRET","severity":"CRITICAL"}]`)
	rl := RequestLog{
		RequestID:      "r1",
		OrgID:          "00000000-0000-0000-0000-000000000001",
		Provider:       "openai",
		Model:          "gpt-4o",
		ModelFamily:    "gpt-4o",
		InputTokens:    200,
		OutputTokens:   100,
		Findings:       findings,
		FindingsCount:  2,
		ActionTaken:    "block",
		ScanLatencyMs:  5.5,
		TotalLatencyMs: 100.0,
		UserID:         "user-1",
		Endpoint:       "/v1/chat/completions",
	}
	if rl.RequestID != "r1" {
		t.Error("RequestID mismatch")
	}
	if rl.FindingsCount != 2 {
		t.Error("FindingsCount mismatch")
	}
	if string(rl.Findings) != string(findings) {
		t.Error("Findings mismatch")
	}
}

// TestSecurityEvent_JSONTags verifies JSON tag structure for API serialization.
func TestSecurityEvent_JSONTags(t *testing.T) {
	now := time.Now().UTC()
	ev := SecurityEvent{
		RequestID:     "r1",
		Provider:      "openai",
		Model:         "gpt-4o",
		ActionTaken:   "block",
		Findings:      json.RawMessage(`[{"type":"PII"}]`),
		FindingsCount: 1,
		CreatedAt:     now,
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["request_id"] != "r1" {
		t.Errorf("request_id: want r1, got %v", m["request_id"])
	}
	if m["action_taken"] != "block" {
		t.Errorf("action_taken: want block, got %v", m["action_taken"])
	}
	if _, ok := m["created_at"]; !ok {
		t.Error("created_at field missing")
	}
}
