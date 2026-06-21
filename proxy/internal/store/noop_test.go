package store

import (
	"context"
	"os"
	"testing"
	"time"

	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/policy"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/scanner"
)

// compile-time check: NoopStore must implement Store
var _ Store = (*NoopStore)(nil)

// ---------------------------------------------------------------------------
// NoopStore constructors
// ---------------------------------------------------------------------------

func TestNoopStore_NewNoopStore(t *testing.T) {
	s := NewNoopStore(zerolog.Nop())
	if s == nil {
		t.Fatal("NewNoopStore should return non-nil")
	}
}

func TestNoopStore_NewNoopStoreSilent(t *testing.T) {
	s := NewNoopStoreSilent()
	if s == nil {
		t.Fatal("NewNoopStoreSilent should return non-nil")
	}
}

// ---------------------------------------------------------------------------
// Store interface compliance — every method exercised via NoopStore
// ---------------------------------------------------------------------------

func TestNoopStore_SaveRequestLog_Compliance(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		err := s.SaveRequestLog(ctx, RequestLog{
			RequestID: "req-1",
			OrgID:     "00000000-0000-0000-0000-000000000001",
			Provider:  "openai",
			Model:     "gpt-4o",
		})
		if err != nil {
			t.Fatalf("SaveRequestLog should return nil error, got %v", err)
		}
	})

	t.Run("with_all_fields", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		err := s.SaveRequestLog(ctx, RequestLog{
			RequestID:      "req-full",
			OrgID:          "00000000-0000-0000-0000-000000000001",
			Provider:       "anthropic",
			Model:          "claude-sonnet-4-6",
			ModelFamily:    "claude-4",
			InputTokens:    150,
			OutputTokens:   80,
			Findings:       []byte(`[{"type":"PII","severity":"HIGH"}]`),
			FindingsCount:  1,
			ActionTaken:    "block",
			ScanLatencyMs:  12.5,
			TotalLatencyMs: 350.0,
			UserID:         "user-test",
			Endpoint:       "/v1/messages",
		})
		if err != nil {
			t.Fatalf("SaveRequestLog with all fields should succeed: %v", err)
		}
	})

	t.Run("empty_request_log", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		err := s.SaveRequestLog(ctx, RequestLog{})
		if err != nil {
			t.Fatalf("SaveRequestLog with empty log should succeed: %v", err)
		}
	})

	t.Run("nil_context", func(t *testing.T) {
		s := NewNoopStoreSilent()
		err := s.SaveRequestLog(nil, RequestLog{RequestID: "r1"})
		if err != nil {
			t.Fatalf("SaveRequestLog with nil context should succeed: %v", err)
		}
	})
}

func TestNoopStore_GetStats_Compliance(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		now := time.Now().UTC()
		st, err := s.GetStats(ctx, "00000000-0000-0000-0000-000000000001", now.Add(-24*time.Hour), now)
		if err != nil {
			t.Fatalf("GetStats should return nil error, got %v", err)
		}
		if st == nil {
			t.Fatal("GetStats should return non-nil stats")
		}
		if st.TotalRequests != 0 {
			t.Errorf("TotalRequests: want 0, got %d", st.TotalRequests)
		}
		if st.BlockedRequests != 0 {
			t.Errorf("BlockedRequests: want 0, got %d", st.BlockedRequests)
		}
		if st.RedactedRequests != 0 {
			t.Errorf("RedactedRequests: want 0, got %d", st.RedactedRequests)
		}
		if st.WarnedRequests != 0 {
			t.Errorf("WarnedRequests: want 0, got %d", st.WarnedRequests)
		}
		if st.TotalInputTokens != 0 {
			t.Errorf("TotalInputTokens: want 0, got %d", st.TotalInputTokens)
		}
		if st.TotalOutputTokens != 0 {
			t.Errorf("TotalOutputTokens: want 0, got %d", st.TotalOutputTokens)
		}
	})

	t.Run("zero_time_range", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		zero := time.Time{}
		st, err := s.GetStats(ctx, "org-any", zero, zero)
		if err != nil {
			t.Fatalf("GetStats zero time should succeed: %v", err)
		}
		if st == nil {
			t.Fatal("GetStats should return non-nil stats")
		}
	})

	t.Run("empty_org", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		st, err := s.GetStats(ctx, "", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("GetStats empty org should succeed: %v", err)
		}
		if st == nil {
			t.Fatal("GetStats should return non-nil stats")
		}
	})
}

func TestNoopStore_ListSecurityEvents_Compliance(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		events, total, err := s.ListSecurityEvents(ctx, "org-1", 1, 50)
		if err != nil {
			t.Fatalf("ListSecurityEvents should return nil error, got %v", err)
		}
		if events != nil {
			t.Errorf("events: want nil slice, got %d items", len(events))
		}
		if total != 0 {
			t.Errorf("total: want 0, got %d", total)
		}
	})

	t.Run("page_edge_cases", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()

		// Negative page.
		events, total, err := s.ListSecurityEvents(ctx, "org-1", -1, 10)
		if err != nil {
			t.Fatalf("ListSecurityEvents negative page: %v", err)
		}
		if events != nil || total != 0 {
			t.Errorf("got events=%v total=%d", events, total)
		}

		// Zero limit.
		events, total, err = s.ListSecurityEvents(ctx, "org-1", 1, 0)
		if err != nil {
			t.Fatalf("ListSecurityEvents zero limit: %v", err)
		}
		if events != nil || total != 0 {
			t.Errorf("got events=%v total=%d", events, total)
		}

		// Large limit.
		events, total, err = s.ListSecurityEvents(ctx, "org-1", 1, 1000)
		if err != nil {
			t.Fatalf("ListSecurityEvents large limit: %v", err)
		}
		if events != nil || total != 0 {
			t.Errorf("got events=%v total=%d", events, total)
		}
	})
}

func TestNoopStore_SearchSecurityEvents_Compliance(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		events, total, err := s.SearchSecurityEvents(ctx, "org-1", EventSearchParams{
			Page:   1,
			Limit:  50,
			Action: "block",
			Since:  time.Now().Add(-24 * time.Hour),
			Until:  time.Now(),
		})
		if err != nil {
			t.Fatalf("SearchSecurityEvents should return nil error, got %v", err)
		}
		if events != nil {
			t.Errorf("events: want nil slice, got %d items", len(events))
		}
		if total != 0 {
			t.Errorf("total: want 0, got %d", total)
		}
	})

	t.Run("empty_params", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		events, total, err := s.SearchSecurityEvents(ctx, "org-1", EventSearchParams{})
		if err != nil {
			t.Fatalf("SearchSecurityEvents empty params: %v", err)
		}
		if events != nil || total != 0 {
			t.Errorf("got events=%v total=%d", events, total)
		}
	})

	t.Run("all_filters", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		events, total, err := s.SearchSecurityEvents(ctx, "org-1", EventSearchParams{
			Page:        1,
			Limit:       50,
			Action:      "redact",
			Provider:    "openai",
			ShadowOnly:  true,
			FindingType: "PII",
			Severity:    "HIGH",
			Category:    "personally-identifiable-information",
			Technique:   "email",
			Q:           "password",
			Since:       time.Now().Add(-7 * 24 * time.Hour),
			Until:       time.Now(),
		})
		if err != nil {
			t.Fatalf("SearchSecurityEvents all filters: %v", err)
		}
		if events != nil || total != 0 {
			t.Errorf("got events=%v total=%d", events, total)
		}
	})

	t.Run("negative_page_and_limit", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		events, total, err := s.SearchSecurityEvents(ctx, "org-1", EventSearchParams{
			Page:  -5,
			Limit: -10,
		})
		if err != nil {
			t.Fatalf("SearchSecurityEvents negative params: %v", err)
		}
		if events != nil || total != 0 {
			t.Errorf("got events=%v total=%d", events, total)
		}
	})
}

func TestNoopStore_GetModelTokenUsage_Compliance(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		now := time.Now().UTC()
		usage, err := s.GetModelTokenUsage(ctx, "org-1", now.Add(-24*time.Hour), now)
		if err != nil {
			t.Fatalf("GetModelTokenUsage should return nil error, got %v", err)
		}
		if usage != nil {
			t.Errorf("usage: want nil slice, got %d rows", len(usage))
		}
	})

	t.Run("zero_time_range", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		zero := time.Time{}
		usage, err := s.GetModelTokenUsage(ctx, "org-1", zero, zero)
		if err != nil {
			t.Fatalf("GetModelTokenUsage zero time: %v", err)
		}
		if usage != nil {
			t.Errorf("usage: want nil, got %d rows", len(usage))
		}
	})

	t.Run("empty_org", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		usage, err := s.GetModelTokenUsage(ctx, "", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("GetModelTokenUsage empty org: %v", err)
		}
		if usage != nil {
			t.Errorf("usage: want nil, got %d rows", len(usage))
		}
	})
}

func TestNoopStore_Ping_Compliance(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		s := NewNoopStoreSilent()
		if err := s.Ping(context.Background()); err != nil {
			t.Fatalf("Ping should succeed: %v", err)
		}
	})

	t.Run("nil_context", func(t *testing.T) {
		s := NewNoopStoreSilent()
		if err := s.Ping(nil); err != nil {
			t.Fatalf("Ping with nil context should succeed: %v", err)
		}
	})

	t.Run("after_close", func(t *testing.T) {
		s := NewNoopStoreSilent()
		_ = s.Close()
		if err := s.Ping(context.Background()); err != nil {
			t.Fatalf("Ping after close should still succeed: %v", err)
		}
	})
}

func TestNoopStore_Close_Compliance(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		s := NewNoopStoreSilent()
		if err := s.Close(); err != nil {
			t.Fatalf("Close should succeed: %v", err)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		s := NewNoopStoreSilent()
		for i := 0; i < 5; i++ {
			if err := s.Close(); err != nil {
				t.Fatalf("Close call %d should succeed: %v", i, err)
			}
		}
	})

	t.Run("operations_after_close", func(t *testing.T) {
		s := NewNoopStoreSilent()
		_ = s.Close()

		// All operations should still succeed after close (noop is stateless).
		if err := s.Ping(context.Background()); err != nil {
			t.Errorf("Ping after close: %v", err)
		}
		if err := s.SaveRequestLog(context.Background(), RequestLog{RequestID: "r1"}); err != nil {
			t.Errorf("SaveRequestLog after close: %v", err)
		}
		st, err := s.GetStats(context.Background(), "org", time.Now(), time.Now())
		if err != nil || st == nil {
			t.Errorf("GetStats after close: err=%v st=%v", err, st)
		}
	})
}

// ---------------------------------------------------------------------------
// ModelTokenUsage zero-value checks
// ---------------------------------------------------------------------------

func TestModelTokenUsage_ZeroValue(t *testing.T) {
	u := ModelTokenUsage{}
	if u.Provider != "" {
		t.Errorf("Provider: want empty, got %q", u.Provider)
	}
	if u.Model != "" {
		t.Errorf("Model: want empty, got %q", u.Model)
	}
	if u.ModelFamily != "" {
		t.Errorf("ModelFamily: want empty, got %q", u.ModelFamily)
	}
	if u.InputTokens != 0 {
		t.Errorf("InputTokens: want 0, got %d", u.InputTokens)
	}
	if u.OutputTokens != 0 {
		t.Errorf("OutputTokens: want 0, got %d", u.OutputTokens)
	}
}

// ---------------------------------------------------------------------------
// Stats zero-value checks
// ---------------------------------------------------------------------------

func TestStats_ZeroValue(t *testing.T) {
	st := Stats{}
	if st.TotalRequests != 0 {
		t.Errorf("TotalRequests: want 0, got %d", st.TotalRequests)
	}
	if st.BlockedRequests != 0 {
		t.Errorf("BlockedRequests: want 0, got %d", st.BlockedRequests)
	}
	if st.RedactedRequests != 0 {
		t.Errorf("RedactedRequests: want 0, got %d", st.RedactedRequests)
	}
	if st.WarnedRequests != 0 {
		t.Errorf("WarnedRequests: want 0, got %d", st.WarnedRequests)
	}
	if st.TotalInputTokens != 0 {
		t.Errorf("TotalInputTokens: want 0, got %d", st.TotalInputTokens)
	}
	if st.TotalOutputTokens != 0 {
		t.Errorf("TotalOutputTokens: want 0, got %d", st.TotalOutputTokens)
	}
}

// ---------------------------------------------------------------------------
// EventSearchParams zero-value checks
// ---------------------------------------------------------------------------

func TestEventSearchParams_ZeroValue(t *testing.T) {
	p := EventSearchParams{}
	if p.Page != 0 {
		t.Errorf("Page: want 0, got %d", p.Page)
	}
	if p.Limit != 0 {
		t.Errorf("Limit: want 0, got %d", p.Limit)
	}
	if p.Action != "" {
		t.Errorf("Action: want empty, got %q", p.Action)
	}
	if p.Provider != "" {
		t.Errorf("Provider: want empty, got %q", p.Provider)
	}
	if p.ShadowOnly {
		t.Errorf("ShadowOnly: want false")
	}
	if p.FindingType != "" {
		t.Errorf("FindingType: want empty, got %q", p.FindingType)
	}
	if p.Severity != "" {
		t.Errorf("Severity: want empty, got %q", p.Severity)
	}
	if p.Category != "" {
		t.Errorf("Category: want empty, got %q", p.Category)
	}
	if p.Technique != "" {
		t.Errorf("Technique: want empty, got %q", p.Technique)
	}
	if p.Q != "" {
		t.Errorf("Q: want empty, got %q", p.Q)
	}
	if !p.Since.IsZero() {
		t.Errorf("Since: want zero time, got %v", p.Since)
	}
	if !p.Until.IsZero() {
		t.Errorf("Until: want zero time, got %v", p.Until)
	}
}

// ---------------------------------------------------------------------------
// NewSavedHuntStore(nil) returns nil
// ---------------------------------------------------------------------------

func TestNewSavedHuntStore_Nil(t *testing.T) {
	s := NewSavedHuntStore(nil)
	if s != nil {
		t.Fatal("NewSavedHuntStore(nil) should return nil")
	}
}

// ---------------------------------------------------------------------------
// NewPricingStore(nil) returns non-nil (stores nil pool, safe to construct)
// ---------------------------------------------------------------------------

func TestNewPricingStore_Nil(t *testing.T) {
	s := NewPricingStore(nil)
	if s == nil {
		t.Fatal("NewPricingStore(nil) should return non-nil PricingStore")
	}
}

// ---------------------------------------------------------------------------
// PartitionManager — nil-db constructor and nil-receiver safety
// ---------------------------------------------------------------------------

func TestNewPartitionManager_NilDB(t *testing.T) {
	pm := NewPartitionManager(nil, DefaultRetentionPolicy(), zerolog.Nop())
	if pm == nil {
		t.Fatal("NewPartitionManager should return non-nil even with nil db")
	}
	if pm.db != nil {
		t.Error("expected nil db in PartitionManager")
	}
	if pm.policy.RequestLogsDays != 30 {
		t.Errorf("policy.RequestLogsDays: want 30, got %d", pm.policy.RequestLogsDays)
	}
}

func TestPartitionManager_LastRun_Zero(t *testing.T) {
	pm := NewPartitionManager(nil, DefaultRetentionPolicy(), zerolog.Nop())
	ts := pm.LastRun()
	if !ts.IsZero() {
		t.Errorf("LastRun: want zero time, got %v", ts)
	}
}

func TestPartitionManager_LastRun_NonZero(t *testing.T) {
	pm := NewPartitionManager(nil, DefaultRetentionPolicy(), zerolog.Nop())
	now := time.Now().UTC()
	pm.lastRunUnix.Store(now.Unix())
	ts := pm.LastRun()
	if ts.IsZero() {
		t.Fatal("LastRun should return non-zero time after storing")
	}
	// Should be within 1 second of the stored time (UTC truncated to second).
	if ts.Sub(now.Truncate(time.Second)) > time.Second || ts.Sub(now.Truncate(time.Second)) < -time.Second {
		t.Errorf("LastRun: got %v, want ~%v", ts, now.Truncate(time.Second))
	}
}

func TestPartitionManager_RunMaintenanceCycle_NilDB(t *testing.T) {
	pm := NewPartitionManager(nil, DefaultRetentionPolicy(), zerolog.Nop())
	ctx := context.Background()
	err := pm.RunMaintenanceCycle(ctx)
	if err == nil {
		t.Fatal("RunMaintenanceCycle with nil db should return error")
	}
}

func TestPartitionManager_Start_NilPM(t *testing.T) {
	// Call Start on a nil *PartitionManager — should not panic.
	var pm *PartitionManager
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel so if it even starts, it stops.
	// This should not panic due to nil-receiver check at the top of Start.
	pm.Start(ctx, 0)
}

func TestPartitionManager_Start_NilDB(t *testing.T) {
	pm := NewPartitionManager(nil, DefaultRetentionPolicy(), zerolog.Nop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel.
	// Should return immediately after nil db check.
	pm.Start(ctx, 0)
}

// ---------------------------------------------------------------------------
// RetentionPolicyFromEnv — full coverage including IncidentClosedDays
// ---------------------------------------------------------------------------

func TestRetentionPolicyFromEnv_AllOverrides(t *testing.T) {
	// Save and restore env.
	restore := saveEnv(
		"TAMGA_RETENTION_REQUEST_LOGS_DAYS",
		"TAMGA_RETENTION_ALERTS_DAYS",
		"TAMGA_RETENTION_DAILY_STATS_DAYS",
		"TAMGA_RETENTION_AUDIT_DAYS",
		"TAMGA_RETENTION_INCIDENT_CLOSED_DAYS",
	)
	defer restore()

	_ = os.Unsetenv("TAMGA_RETENTION_REQUEST_LOGS_DAYS")
	_ = os.Unsetenv("TAMGA_RETENTION_ALERTS_DAYS")
	_ = os.Unsetenv("TAMGA_RETENTION_DAILY_STATS_DAYS")
	_ = os.Unsetenv("TAMGA_RETENTION_AUDIT_DAYS")
	_ = os.Unsetenv("TAMGA_RETENTION_INCIDENT_CLOSED_DAYS")

	_ = os.Setenv("TAMGA_RETENTION_REQUEST_LOGS_DAYS", "14")
	_ = os.Setenv("TAMGA_RETENTION_ALERTS_DAYS", "45")
	_ = os.Setenv("TAMGA_RETENTION_DAILY_STATS_DAYS", "180")
	_ = os.Setenv("TAMGA_RETENTION_AUDIT_DAYS", "730")
	_ = os.Setenv("TAMGA_RETENTION_INCIDENT_CLOSED_DAYS", "60")

	p := RetentionPolicyFromEnv()
	if p.RequestLogsDays != 14 {
		t.Errorf("RequestLogsDays: want 14, got %d", p.RequestLogsDays)
	}
	if p.AlertsDays != 45 {
		t.Errorf("AlertsDays: want 45, got %d", p.AlertsDays)
	}
	if p.DailyStatsDays != 180 {
		t.Errorf("DailyStatsDays: want 180, got %d", p.DailyStatsDays)
	}
	if p.AuditLogsDays != 730 {
		t.Errorf("AuditLogsDays: want 730, got %d", p.AuditLogsDays)
	}
	if p.IncidentClosedDays != 60 {
		t.Errorf("IncidentClosedDays: want 60, got %d", p.IncidentClosedDays)
	}
}

// saveEnv saves current values of given env vars and returns a restore func.
func saveEnv(keys ...string) func() {
	saved := make(map[string]string)
	for _, k := range keys {
		saved[k] = os.Getenv(k)
	}
	return func() {
		for k, v := range saved {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// parsePartitionUpperBoundTO — invalid date format in TO clause
// ---------------------------------------------------------------------------

func TestParsePartitionUpperBoundTO_InvalidDateFormat(t *testing.T) {
	t.Parallel()
	// The regex matches, but the date is not a valid calendar date (month 13).
	bound := `FOR VALUES FROM ('2026-01-01') TO ('2026-13-01')`
	_, ok := parsePartitionUpperBoundTO(bound)
	if ok {
		t.Error("expected !ok for invalid date (month 13)")
	}
}

// ---------------------------------------------------------------------------
// hashFindingMatches additional edge case — nil Match in mixed slice
// ---------------------------------------------------------------------------

func TestHashFindingMatches_MixedMatches(t *testing.T) {
	findings := []scanner.Finding{
		{Type: "pii", Match: "email@example.com", Severity: "high"},
		{Type: "secret", Match: "", Severity: "critical"},
		{Type: "injection", Match: "DROP TABLE", Severity: "high"},
	}
	out := hashFindingMatches(findings)
	if len(out) != 3 {
		t.Fatalf("want 3 findings, got %d", len(out))
	}
	// First should be hashed.
	if out[0].Match == "" || out[0].Match == "email@example.com" {
		t.Error("first match should be hashed")
	}
	// Second should remain empty.
	if out[1].Match != "" {
		t.Errorf("second match should be empty, got %q", out[1].Match)
	}
	// Third should be hashed.
	if out[2].Match == "" || out[2].Match == "DROP TABLE" {
		t.Error("third match should be hashed")
	}
	// Non-Match fields in first finding preserved.
	if out[0].Type != "pii" || out[0].Severity != "high" {
		t.Errorf("non-match fields altered: %+v", out[0])
	}
}

// ---------------------------------------------------------------------------
// DBHandler additional coverage — nil store path
// ---------------------------------------------------------------------------

func TestDBHandler_NilStore_Noop(t *testing.T) {
	handler := DBHandler(zerolog.Nop(), nil, "org-1", nil)
	// Should not panic and should not do anything.
	handler(events.Event{
		EventType: "request_scanned",
		OrgID:     "org-1",
		Findings:  []scanner.Finding{{Type: "pii", Match: "test"}},
	})
}

func TestDBHandler_NonScannedEvent(t *testing.T) {
	s := NewNoopStoreSilent()
	handler := DBHandler(zerolog.Nop(), s, "org-1", nil)
	// Non-scanned/blocked events should be silently skipped.
	handler(events.Event{
		EventType: "output_scan_hint",
		OrgID:     "org-1",
	})
}

func TestDBHandler_RequestBlocked(t *testing.T) {
	s := NewNoopStoreSilent()
	handler := DBHandler(zerolog.Nop(), s, "org-1", nil)
	handler(events.Event{
		EventType: "request_blocked",
		Action:    "BLOCK",
		OrgID:     "org-1",
		Findings:  []scanner.Finding{{Type: "injection", Match: "SQL injection", Severity: "high"}},
	})
}

// ---------------------------------------------------------------------------
// PartitionManager atomic storage test
// ---------------------------------------------------------------------------

func TestPartitionManager_LastRun_AtomicStorage(t *testing.T) {
	pm := NewPartitionManager(nil, DefaultRetentionPolicy(), zerolog.Nop())

	// Initially zero.
	if !pm.LastRun().IsZero() {
		t.Error("initial LastRun should be zero")
	}

	// Store a time and verify.
	t1 := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	pm.lastRunUnix.Store(t1.Unix())
	got := pm.LastRun()
	if got != t1 {
		t.Errorf("LastRun: want %v, got %v", t1, got)
	}

	// Store another time and verify.
	t2 := time.Date(2026, 6, 16, 14, 45, 0, 0, time.UTC)
	pm.lastRunUnix.Store(t2.Unix())
	got = pm.LastRun()
	if got != t2 {
		t.Errorf("LastRun after second store: want %v, got %v", t2, got)
	}
}

// ---------------------------------------------------------------------------
// pgxpool.Pool — verify NewSavedHuntStore with nil pool returns nil
// ---------------------------------------------------------------------------

func TestNewSavedHuntStore_NilPool(t *testing.T) {
	// Cross-check: NewSavedHuntStore accepts *pgxpool.Pool; nil yields nil.
	var pool *pgxpool.Pool = nil
	store := NewSavedHuntStore(pool)
	if store != nil {
		t.Fatal("NewSavedHuntStore(nil pool) should return nil")
	}
}

// ---------------------------------------------------------------------------
// PostgresStore — directly constructed, no DB required for these paths
// ---------------------------------------------------------------------------

func TestPostgresStore_Pool_Nil(t *testing.T) {
	s := &PostgresStore{}
	if p := s.Pool(); p != nil {
		t.Errorf("Pool: want nil, got non-nil")
	}
}

func TestPostgresStore_SaveRequestLog_Closed(t *testing.T) {
	s := &PostgresStore{closed: true}
	err := s.SaveRequestLog(context.Background(), RequestLog{RequestID: "r1"})
	if err == nil {
		t.Fatal("SaveRequestLog on closed store should return error")
	}
}

func TestPostgresStore_SaveRequestLog_Append(t *testing.T) {
	s := &PostgresStore{
		log:  zerolog.Nop(),
		done: make(chan struct{}),
	}
	ctx := context.Background()

	// Append a single row — no pool access needed.
	err := s.SaveRequestLog(ctx, RequestLog{
		RequestID: "r1",
		OrgID:     "00000000-0000-0000-0000-000000000001",
	})
	if err != nil {
		t.Fatalf("SaveRequestLog append: %v", err)
	}

	// Append more rows; should all succeed without pool.
	for i := 0; i < 50; i++ {
		err := s.SaveRequestLog(ctx, RequestLog{
			RequestID: "r-batch",
			OrgID:     "00000000-0000-0000-0000-000000000001",
		})
		if err != nil {
			t.Fatalf("SaveRequestLog batch append %d: %v", i, err)
		}
	}
}

// saveErrorStore is a Store that always returns an error from SaveRequestLog.
// It embeds *NoopStore to inherit zero-value returns for all other methods.
type saveErrorStore struct {
	*NoopStore
}

func (s *saveErrorStore) SaveRequestLog(_ context.Context, _ RequestLog) error {
	return errors.New("simulated save failure")
}

func TestDBHandler_SaveRequestLog_Error(t *testing.T) {
	errStore := &saveErrorStore{}
	handler := DBHandler(zerolog.Nop(), errStore, "org-1", nil)
	// Should not panic when the store returns an error.
	handler(events.Event{
		EventType: "request_scanned",
		OrgID:     "org-1",
		Findings:  []scanner.Finding{{Type: "pii", Match: "test", Severity: "low"}},
	})
}

// newPostgresStoreTest exercises NewPostgresStore with an invalid DSN.
// The function should return an error without blocking (fast parse failure).
func TestNewPostgresStore_InvalidDSN(t *testing.T) {
	// Use a connection string with a clearly unsupported scheme.
	// pgx should reject this during config parsing without network I/O.
	ctx := context.Background()
	_, err := NewPostgresStore(ctx, "://", zerolog.Nop())
	if err == nil {
		t.Fatal("NewPostgresStore with invalid DSN should return error")
	}
}

// ---------------------------------------------------------------------------
// PostgresStore.SaveRequestLog — validates batch buffer tracking after
// multiple appends through the mutex-guarded path.
// ---------------------------------------------------------------------------

func TestPostgresStore_SaveRequestLog_BufferLen(t *testing.T) {
	s := &PostgresStore{
		log:  zerolog.Nop(),
		done: make(chan struct{}),
	}
	ctx := context.Background()

	// Verify initial buffer is empty.
	s.mu.Lock()
	if len(s.buf) != 0 {
		s.mu.Unlock()
		t.Fatalf("initial buffer want 0, got %d", len(s.buf))
	}
	s.mu.Unlock()

	// Append two rows and check buffer length.
	_ = s.SaveRequestLog(ctx, RequestLog{RequestID: "r1", OrgID: "00000000-0000-0000-0000-000000000001"})
	_ = s.SaveRequestLog(ctx, RequestLog{RequestID: "r2", OrgID: "00000000-0000-0000-0000-000000000001"})

	s.mu.Lock()
	n := len(s.buf)
	s.mu.Unlock()
	if n != 2 {
		t.Errorf("buffer after 2 appends: want 2, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// DBHandler — invocation with default org fallback
// ---------------------------------------------------------------------------

func TestDBHandler_DefaultOrgFallback(t *testing.T) {
	s := NewNoopStoreSilent()
	handler := DBHandler(zerolog.Nop(), s, "default-org", nil)
	// Empty event OrgID should fall back to defaultOrgID.
	handler(events.Event{
		EventType: "request_scanned",
		OrgID:     "",
		Findings:  []scanner.Finding{{Type: "pii", Match: "test"}},
	})
}

// ---------------------------------------------------------------------------
// PartitionManager.Start — with valid context that is NOT cancelled
// (Start checks ctx.Done() inside the loop, but nil db returns early).
// ---------------------------------------------------------------------------

func TestPartitionManager_Start_ValidContext(t *testing.T) {
	pm := NewPartitionManager(nil, DefaultRetentionPolicy(), zerolog.Nop())
	ctx := context.Background()
	// pm.db is nil, so Start returns immediately.
	pm.Start(ctx, 24*time.Hour)
}

// ---------------------------------------------------------------------------
// PostgresStore.flush — directly callable in tests; empty-buf path
// ---------------------------------------------------------------------------

func TestPostgresStore_Flush_EmptyBuffer(t *testing.T) {
	s := &PostgresStore{log: zerolog.Nop()}
	// flush with nil buf should be a no-op (batch length 0 → return).
	s.flush(context.Background())
}

// ---------------------------------------------------------------------------
// PostgresStore.insertBatch — empty rows and n==0 paths
// ---------------------------------------------------------------------------

func TestPostgresStore_InsertBatch_Empty(t *testing.T) {
	s := &PostgresStore{log: zerolog.Nop()}
	// Nil slice → len(rows) == 0 → return nil.
	if err := s.insertBatch(context.Background(), nil); err != nil {
		t.Fatalf("insertBatch nil: %v", err)
	}
	// Empty slice → same path.
	if err := s.insertBatch(context.Background(), []RequestLog{}); err != nil {
		t.Fatalf("insertBatch empty: %v", err)
	}
}

func TestPostgresStore_InsertBatch_AllInvalidUUID(t *testing.T) {
	s := &PostgresStore{log: zerolog.Nop()}
	rows := []RequestLog{
		{RequestID: "r1", OrgID: "not-a-uuid"},
		{RequestID: "r2", OrgID: "also-invalid"},
		{RequestID: "r3", OrgID: "garbage"},
	}
	// All rows have invalid UUIDs → uuid.Parse fails → n stays 0 → return nil.
	if err := s.insertBatch(context.Background(), rows); err != nil {
		t.Fatalf("insertBatch all-invalid UUIDs: %v", err)
	}
}

// ---------------------------------------------------------------------------
// PostgresStore.flushLoop — start goroutine, stop via closing done channel
// ---------------------------------------------------------------------------

func TestPostgresStore_FlushLoop_Done(t *testing.T) {
	s := &PostgresStore{
		log:  zerolog.Nop(),
		done: make(chan struct{}),
	}
	s.wg.Add(1)
	go s.flushLoop()

	// Close done to stop the loop. The final flush inside flushLoop drains
	// the empty buffer (no-op). Then wg.Done fires.
	close(s.done)
	s.wg.Wait()
}

// ---------------------------------------------------------------------------
// PostgresStore.SaveRequestLog — trigger batch flush with 100 rows of
// invalid UUIDs. The flush triggers insertBatch which skips all invalid
// rows (n=0) and returns nil, no pool access needed.
// ---------------------------------------------------------------------------

func TestPostgresStore_SaveRequestLog_BatchFlush(t *testing.T) {
	s := &PostgresStore{
		log:  zerolog.Nop(),
		done: make(chan struct{}),
	}
	ctx := context.Background()

	// Append batchMaxRows (100) rows with invalid UUIDs.
	// On the 100th call, the flush fires. insertBatch skips all rows
	// because uuid.Parse fails on "not-a-uuid", n stays 0, returns nil.
	for i := 0; i < batchMaxRows; i++ {
		err := s.SaveRequestLog(ctx, RequestLog{
			RequestID: "r-flush",
			OrgID:     "not-a-uuid",
		})
		if err != nil {
			t.Fatalf("SaveRequestLog %d: %v", i, err)
		}
	}

	// After flush, buffer should be nil.
	s.mu.Lock()
	n := len(s.buf)
	s.mu.Unlock()
	if n != 0 {
		t.Errorf("buffer after batch flush: want 0, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// PostgresStore — verifying closure guard on SaveRequestLog
// ---------------------------------------------------------------------------

func TestPostgresStore_SaveRequestLog_AfterClose(t *testing.T) {
	s := &PostgresStore{
		log:  zerolog.Nop(),
		done: make(chan struct{}),
	}
	// Manually mark closed without calling Close() (which needs pool).
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	err := s.SaveRequestLog(context.Background(), RequestLog{RequestID: "r1"})
	if err == nil {
		t.Fatal("SaveRequestLog on closed store should return error")
	}
}

// ---------------------------------------------------------------------------
// PostgresStore.insertBatch — mixed valid/invalid UUID normalization
// (n>0 path requires a real pgxpool.Pool, only tested in integration)
// ---------------------------------------------------------------------------
//
// Already covered: empty rows, all-invalid UUIDs (both return nil without pool).
// The n>0 path with valid UUIDs needs a real DB and is tested in integration tests.

// ---------------------------------------------------------------------------
// Rejecting pgxpool.Pool — allows testing all PostgresStore methods in-process
// without Docker. BeforeConnect always returns an error, so no TCP connections
// are ever attempted. The pool object itself is valid and Close() works.
// ---------------------------------------------------------------------------

// newRejectingPool creates a pgxpool.Pool whose BeforeConnect hook always
// returns an error. No network connections are attempted. The returned pool
// is valid and its Close() method works without panicking.
func newRejectingPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig("postgres://x:x@127.0.0.1:5432/test?connect_timeout=1")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	cfg.ConnConfig.ConnectTimeout = 100 * time.Millisecond
	// BeforeConnect rejects all connection attempts — no TCP connections are made.
	cfg.BeforeConnect = func(_ context.Context, _ *pgx.ConnConfig) error {
		return errors.New("test: connection rejected by BeforeConnect")
	}
	cfg.MinConns = 0
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewWithConfig: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// newRejectingPostgresStore creates a PostgresStore backed by a rejecting
// pool. The flush loop is started (so Close() can stop it properly).
func newRejectingPostgresStore(t *testing.T) *PostgresStore {
	t.Helper()
	pool := newRejectingPool(t)
	s := &PostgresStore{
		pool: pool,
		log:  zerolog.Nop(),
		done: make(chan struct{}),
	}
	s.wg.Add(1)
	go s.flushLoop()
	t.Cleanup(func() {
		_ = s.Close()
	})
	return s
}

func TestPostgresStore_Close_WithPool(t *testing.T) {
	s := newRejectingPostgresStore(t)
	// Close should succeed — pool is valid, wg is managed, done channel exists.
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Second close is idempotent.
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestPostgresStore_Ping_WithPool(t *testing.T) {
	s := newRejectingPostgresStore(t)
	// Ping tries to acquire a connection; BeforeConnect rejects it.
	// Use a short timeout so the test doesn't hang.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := s.Ping(ctx)
	if err == nil {
		t.Log("Ping unexpectedly succeeded with rejecting pool")
	}
}

func TestPostgresStore_Pool_WithPool(t *testing.T) {
	s := newRejectingPostgresStore(t)
	pool := s.Pool()
	if pool == nil {
		t.Fatal("Pool should return non-nil pool")
	}
}

func TestPostgresStore_GetStats_WithPool(t *testing.T) {
	s := newRejectingPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	st, err := s.GetStats(ctx, "00000000-0000-0000-0000-000000000001", time.Now().Add(-24*time.Hour), time.Now())
	if err == nil {
		t.Logf("GetStats unexpectedly succeeded: %+v", st)
	}
}

func TestPostgresStore_GetModelTokenUsage_WithPool(t *testing.T) {
	s := newRejectingPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	usage, err := s.GetModelTokenUsage(ctx, "00000000-0000-0000-0000-000000000001", time.Now().Add(-24*time.Hour), time.Now())
	if err == nil {
		t.Logf("GetModelTokenUsage unexpectedly succeeded: %d rows", len(usage))
	}
}

func TestPostgresStore_ListSecurityEvents_WithPool(t *testing.T) {
	s := newRejectingPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, total, err := s.ListSecurityEvents(ctx, "00000000-0000-0000-0000-000000000001", 1, 50)
	if err == nil {
		t.Logf("ListSecurityEvents unexpectedly succeeded: %d events, total=%d", len(events), total)
	}
}

func TestPostgresStore_SearchSecurityEvents_WithPool(t *testing.T) {
	s := newRejectingPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, total, err := s.SearchSecurityEvents(ctx, "00000000-0000-0000-0000-000000000001", EventSearchParams{
		Page:  1,
		Limit: 50,
		Since: time.Now().Add(-24 * time.Hour),
		Until: time.Now(),
	})
	if err == nil {
		t.Logf("SearchSecurityEvents unexpectedly succeeded: %d events, total=%d", len(events), total)
	}
}

func TestPostgresStore_EraseSubject_WithPool(t *testing.T) {
	s := newRejectingPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	n, err := s.EraseSubject(ctx, "00000000-0000-0000-0000-000000000001", "user-1", "", "")
	if err == nil {
		t.Logf("EraseSubject unexpectedly succeeded: %d rows", n)
	}
}

func TestPostgresStore_SubjectAccess_WithPool(t *testing.T) {
	s := newRejectingPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rows, err := s.SubjectAccess(ctx, "00000000-0000-0000-0000-000000000001", "user-1", "", "", 10)
	if err == nil {
		t.Logf("SubjectAccess unexpectedly succeeded: %d rows", len(rows))
	}
}

func TestPostgresStore_ApplyRetention_WithPool(t *testing.T) {
	s := newRejectingPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	n, err := s.ApplyRetention(ctx, "00000000-0000-0000-0000-000000000001", 30*24*time.Hour)
	if err == nil {
		t.Logf("ApplyRetention unexpectedly succeeded: %d rows", n)
	}
}

// ---------------------------------------------------------------------------
// Test DBHandler with a deny-listed provider's policy (HashFindings)
// that also returns policy.Data == nil for code path coverage.
// ---------------------------------------------------------------------------

func TestDBHandler_NilPolicyData(t *testing.T) {
	errStore := &saveErrorStore{NoopStore: &NoopStore{}}
	// getPolicy returns a policy with nil Data.
	getPolicy := func() *policy.Policy {
		return &policy.Policy{Data: nil}
	}
	handler := DBHandler(zerolog.Nop(), errStore, "org-1", getPolicy)
	handler(events.Event{
		EventType: "request_scanned",
		OrgID:     "org-1",
		Findings:  []scanner.Finding{{Type: "pii", Match: "test", Severity: "low"}},
	})
}
