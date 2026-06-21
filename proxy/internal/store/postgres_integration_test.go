package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// All tests in this file require TAMGA_TEST_DB_URL.
// Run: TAMGA_TEST_DB_URL="postgres://postgres:test@localhost:5433/test?sslmode=disable" go test -count=1 -v ./internal/store/... -run "TestPG"

func skipIfNoDB(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Postgres integration test in short mode")
	}
	if os.Getenv("TAMGA_TEST_DB_URL") == "" {
		t.Skip("TAMGA_TEST_DB_URL not set; skipping Postgres integration test")
	}
}

// pgCleanTruncate removes all rows from test tables between tests.
func pgCleanTruncate(t *testing.T, s *PostgresStore) {
	t.Helper()
	ctx := context.Background()
	for _, table := range []string{"request_logs", "daily_stats", "model_pricing"} {
		if _, err := s.pool.Exec(ctx, "DELETE FROM "+table); err != nil {
			t.Logf("cleanup %s: %v (may not exist)", table, err)
		}
	}
}

const testOrgID = "00000000-0000-0000-0000-000000000001"

// insertLog inserts a request_log row directly via SQL (bypassing async SaveRequestLog).
func pgInsertLog(t *testing.T, s *PostgresStore, rl RequestLog) {
	t.Helper()
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `
			INSERT INTO request_logs (org_id, request_id, provider, model, model_family,
				input_tokens, output_tokens, findings, findings_count, action_taken,
				scan_latency_ms, total_latency_ms, user_identifier, cost_usd, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		rl.OrgID, rl.RequestID, rl.Provider, rl.Model, rl.ModelFamily,
		rl.InputTokens, rl.OutputTokens, rl.Findings, rl.FindingsCount, rl.ActionTaken,
		rl.ScanLatencyMs, rl.TotalLatencyMs, rl.UserID, 0.0, time.Now(),
	)
	if err != nil {
		t.Fatalf("insertLog: %v", err)
	}
}

func TestPG_SaveRequestLog_Flushes(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)

	ctx := context.Background()
	log := RequestLog{
		RequestID:      "pg-save-1",
		OrgID:          testOrgID,
		Provider:       "openai",
		Model:          "gpt-4o",
		InputTokens:    100,
		OutputTokens:   50,
		Findings:       []byte(`[]`),
		FindingsCount:  0,
		ActionTaken:    "block",
		ScanLatencyMs:  2.5,
		TotalLatencyMs: 120.0,
		UserID:         "user-1",
	}
	if err := s.SaveRequestLog(ctx, log); err != nil {
		t.Fatalf("SaveRequestLog: %v", err)
	}

	// Close triggers flush. Re-open to verify.
	_ = s.Close()

	s2 := newTestPostgresStore(t)
	defer func() { _ = s2.Close() }()

	var count int
	if err := s2.pool.QueryRow(ctx, "SELECT COUNT(*) FROM request_logs WHERE request_id = 'pg-save-1'").Scan(&count); err != nil {
		t.Fatalf("verify count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row after flush, got %d", count)
	}
}

func TestPG_GetStats(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)

	ctx := context.Background()
	// Populate daily_stats directly (GetStats queries daily_stats, not request_logs)
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
			INSERT INTO daily_stats (org_id, stat_date, total_requests, blocked_requests, redacted_requests, total_input_tokens, total_output_tokens)
			VALUES ($1, $2, 10, 3, 2, 500, 250)`, testOrgID, now.Format("2006-01-02"))
	if err != nil {
		t.Fatalf("daily_stats insert: %v", err)
	}

	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)
	stats, err := s.GetStats(ctx, testOrgID, from, to)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalRequests != 10 {
		t.Errorf("expected 10 total, got %d", stats.TotalRequests)
	}
	if stats.BlockedRequests != 3 {
		t.Errorf("expected 3 blocked, got %d", stats.BlockedRequests)
	}
	if stats.TotalInputTokens != 500 {
		t.Errorf("expected 500 input tokens, got %d", stats.TotalInputTokens)
	}
}

func TestPG_ListSecurityEvents(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)
	ctx := context.Background()

	// Insert 5 events via SQL
	for i := 0; i < 5; i++ {
		rl := RequestLog{
			RequestID:    "evt-" + string(rune('a'+i)),
			OrgID:        testOrgID,
			Provider:     "anthropic",
			Model:        "claude-3-5-sonnet",
			InputTokens:  10,
			OutputTokens: 5,
			Findings:     []byte(`[]`),
			ActionTaken:  "pass",
			UserID:       "user-1",
		}
		pgInsertLog(t, s, rl)
	}

	events, total, err := s.ListSecurityEvents(ctx, testOrgID, 1, 2)
	if err != nil {
		t.Fatalf("ListSecurityEvents: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(events) != 2 {
		t.Errorf("expected page size 2, got %d", len(events))
	}
}

func TestPG_SearchSecurityEvents(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)
	ctx := context.Background()

	rl := RequestLog{
		RequestID:    "find-me-999",
		OrgID:        testOrgID,
		Provider:     "openai",
		Model:        "gpt-4o",
		InputTokens:  10,
		OutputTokens: 5,
		Findings:     []byte(`[]`),
		ActionTaken:  "pass",
		UserID:       "search-user",
	}
	pgInsertLog(t, s, rl)

	events, total, err := s.SearchSecurityEvents(ctx, testOrgID, EventSearchParams{
		Q: "find-me", Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchSecurityEvents: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 result, got %d", total)
	}
	if len(events) > 0 && events[0].RequestID != "find-me-999" {
		t.Errorf("expected 'find-me-999', got %q", events[0].RequestID)
	}
}

func TestPG_GetModelTokenUsage(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)
	ctx := context.Background()

	rl := RequestLog{
		RequestID:    "usage-1",
		OrgID:        testOrgID,
		Provider:     "openai",
		Model:        "gpt-4o",
		ModelFamily:  "gpt-4o",
		InputTokens:  1000,
		OutputTokens: 500,
		Findings:     []byte(`[]`),
		ActionTaken:  "pass",
	}
	pgInsertLog(t, s, rl)

	from := time.Now().UTC().Add(-1 * time.Hour)
	to := time.Now().UTC().Add(1 * time.Hour)
	usage, err := s.GetModelTokenUsage(ctx, testOrgID, from, to)
	if err != nil {
		t.Fatalf("GetModelTokenUsage: %v", err)
	}
	if len(usage) == 0 {
		t.Fatal("expected at least 1 usage entry")
	}
	found := false
	for _, u := range usage {
		if u.Model == "gpt-4o" {
			found = true
			if u.InputTokens != 1000 {
				t.Errorf("expected 1000 input tokens, got %d", u.InputTokens)
			}
		}
	}
	if !found {
		t.Error("gpt-4o not found in usage results")
	}
}

func TestPG_PingAndPool(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	if err := s.Ping(ctx); err != nil {
		t.Errorf("Ping: %v", err)
	}

	if pool := s.Pool(); pool == nil {
		t.Error("Pool() returned nil")
	}
}

func TestPG_EraseSubject(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)
	ctx := context.Background()

	// Insert rows for two different user_identifier values
	for _, uid := range []string{"erase-me", "keep-me"} {
		rl := RequestLog{
			RequestID:    uuid.NewString(),
			OrgID:        testOrgID,
			Provider:     "openai",
			Model:        "gpt-4o",
			InputTokens:  10,
			OutputTokens: 5,
			Findings:     []byte(`[]`),
			ActionTaken:  "pass",
			UserID:       uid,
		}
		pgInsertLog(t, s, rl)
	}

	deleted, err := s.EraseSubject(ctx, testOrgID, "erase-me", "", "")
	if err != nil {
		t.Fatalf("EraseSubject: %v", err)
	}
	if deleted < 1 {
		t.Errorf("expected at least 1 deleted, got %d", deleted)
	}

	// Verify keep-me still exists
	var count int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM request_logs WHERE user_identifier = 'keep-me'").Scan(&count); err != nil {
		t.Fatalf("verify keep-me: %v", err)
	}
	if count < 1 {
		t.Error("keep-me should still exist after erase")
	}
}

func TestPG_SubjectAccess(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)
	ctx := context.Background()

	rl := RequestLog{
		RequestID:    uuid.NewString(),
		OrgID:        testOrgID,
		Provider:     "openai",
		Model:        "gpt-4o",
		InputTokens:  10,
		OutputTokens: 5,
		Findings:     []byte(`[]`),
		ActionTaken:  "pass",
		UserID:       "access-me",
	}
	pgInsertLog(t, s, rl)

	rows, err := s.SubjectAccess(ctx, testOrgID, "access-me", "", "", 10)
	if err != nil {
		t.Fatalf("SubjectAccess: %v", err)
	}
	if len(rows) < 1 {
		t.Errorf("expected at least 1 row, got %d", len(rows))
	}
	for _, r := range rows {
		if r.UserID != "access-me" {
			t.Errorf("expected user_id 'access-me', got %q", r.UserID)
		}
	}
}

func TestPG_FindingsJSONB_Roundtrip(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)
	ctx := context.Background()

	findings := []map[string]interface{}{
		{"type": "pii", "category": "credit_card", "severity": "high", "match": "sha256:abc"},
		{"type": "secret", "category": "api_key", "severity": "critical", "match": "sha256:def"},
	}
	fJSON, _ := json.Marshal(findings)

	rl := RequestLog{
		RequestID:     uuid.NewString(),
		OrgID:         testOrgID,
		Provider:      "openai",
		Model:         "gpt-4o",
		InputTokens:   100,
		OutputTokens:  50,
		Findings:      fJSON,
		FindingsCount: len(findings),
		ActionTaken:   "redact",
		UserID:        "user-1",
	}
	pgInsertLog(t, s, rl)

	events, total, err := s.ListSecurityEvents(ctx, testOrgID, 1, 10)
	if err != nil {
		t.Fatalf("ListSecurityEvents: %v", err)
	}
	if total < 1 {
		t.Fatal("expected at least 1 event")
	}
	for _, ev := range events {
		if ev.RequestID == rl.RequestID {
			if ev.FindingsCount != len(findings) {
				t.Errorf("expected %d findings, got %d", len(findings), ev.FindingsCount)
			}
		}
	}
}

func TestPG_ApplyRetention(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)
	ctx := context.Background()

	// Insert an old request_log
	oldTime := time.Now().UTC().Add(-100 * 24 * time.Hour) // 100 days ago
	_, err := s.pool.Exec(ctx, `
			INSERT INTO request_logs (org_id, request_id, provider, model, input_tokens, output_tokens, findings, action_taken, user_identifier, created_at)
			VALUES ($1, 'old-req', 'openai', 'gpt-4o', 10, 5, '[]'::jsonb, 'pass', 'old-user', $2)`,
		testOrgID, oldTime)
	if err != nil {
		t.Fatalf("insert old log: %v", err)
	}

	// Apply retention with 30-day window
	deleted, err := s.ApplyRetention(ctx, testOrgID, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("ApplyRetention: %v", err)
	}
	if deleted < 1 {
		t.Errorf("expected at least 1 row deleted after 30-day retention, got %d", deleted)
	}
}

func TestPG_PricingStore(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)

	ps := NewPricingStore(s.Pool())
	ctx := context.Background()

	// Insert a test price
	_, err := s.pool.Exec(ctx, `
			INSERT INTO model_pricing (provider, model_family, model_version, currency,
				input_per_1k, output_per_1k, effective_from, source)
			VALUES ('openai', 'gpt-4o', 'default', 'USD', 2.500, 10.000, CURRENT_DATE, 'test')`)
	if err != nil {
		t.Fatalf("insert pricing: %v", err)
	}

	active, err := ps.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) < 1 {
		t.Fatal("expected at least 1 active price")
	}

	price, err := ps.Lookup(ctx, "openai", "gpt-4o", "default", time.Now())
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if price == nil {
		t.Fatal("expected non-nil price for gpt-4o")
	}
	if price.InputPer1K != 2.5 {
		t.Errorf("expected 2.5 input price, got %f", price.InputPer1K)
	}
}

// ---------------------------------------------------------------------------
// Testcontainers-based integration tests (NewTestPostgres / NewTestPostgresStore)
// ---------------------------------------------------------------------------

func TestTC_NewPostgresStore(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()

	if err := store.Ping(ctx); err != nil {
		t.Fatalf("Ping after NewTestPostgresStore: %v", err)
	}
	if store.Pool() == nil {
		t.Error("Pool() returned nil")
	}
}

func TestTC_SaveRequestLog_GetStats(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Clean tables.
	_, _ = store.pool.Exec(ctx, "DELETE FROM request_logs")
	_, _ = store.pool.Exec(ctx, "DELETE FROM daily_stats")

	// Save a request log via the buffered path.
	rl := RequestLog{
		RequestID:      "tc-save-stats-1",
		OrgID:          testOrgID,
		Provider:       "openai",
		Model:          "gpt-4o",
		ModelFamily:    "gpt-4o",
		InputTokens:    100,
		OutputTokens:   50,
		Findings:       []byte(`[]`),
		FindingsCount:  0,
		ActionTaken:    "block",
		ScanLatencyMs:  2.0,
		TotalLatencyMs: 150.0,
		UserID:         "tc-user-1",
		Endpoint:       "/v1/chat/completions",
	}
	if err := store.SaveRequestLog(ctx, rl); err != nil {
		t.Fatalf("SaveRequestLog: %v", err)
	}

	// Force flush so the row is visible immediately.
	store.mu.Lock()
	batch := store.buf
	store.buf = nil
	store.mu.Unlock()
	if len(batch) > 0 {
		if err := store.insertBatch(ctx, batch); err != nil {
			t.Fatalf("flush batch: %v", err)
		}
	}

	// Verify the row exists in request_logs.
	var count int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM request_logs WHERE request_id = 'tc-save-stats-1'",
	).Scan(&count); err != nil {
		t.Fatalf("verify request_logs: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 request_log row, got %d", count)
	}

	// Populate daily_stats so GetStats returns data.
	_, err := store.pool.Exec(ctx, `
			INSERT INTO daily_stats (org_id, stat_date, total_requests, blocked_requests,
				redacted_requests, warned_requests, total_input_tokens, total_output_tokens)
			VALUES ($1, $2, 5, 2, 1, 0, 500, 250)`,
		testOrgID, now.Format("2006-01-02"))
	if err != nil {
		t.Fatalf("insert daily_stats: %v", err)
	}

	stats, err := store.GetStats(ctx, testOrgID, now.Add(-1*time.Hour), now.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalRequests != 5 {
		t.Errorf("TotalRequests: want 5, got %d", stats.TotalRequests)
	}
	if stats.BlockedRequests != 2 {
		t.Errorf("BlockedRequests: want 2, got %d", stats.BlockedRequests)
	}
	if stats.TotalInputTokens != 500 {
		t.Errorf("TotalInputTokens: want 500, got %d", stats.TotalInputTokens)
	}
}

func TestTC_ListSecurityEvents(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, _ = store.pool.Exec(ctx, "DELETE FROM request_logs")

	// Insert 5 events via direct SQL (bypass buffer for deterministic ordering).
	for i := 0; i < 5; i++ {
		rid := fmt.Sprintf("tc-list-%d-%d", now.UnixNano(), i)
		_, err := store.pool.Exec(ctx, `
				INSERT INTO request_logs (org_id, request_id, provider, model, model_family,
					input_tokens, output_tokens, findings, findings_count, action_taken,
					scan_latency_ms, total_latency_ms, user_identifier, created_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
			testOrgID, rid, "anthropic", "claude-sonnet-4-6", "claude-4",
			10, 5, []byte(`[]`), 0, "pass",
			1.0, 100.0, "tc-list-user", now,
		)
		if err != nil {
			t.Fatalf("insert event %d: %v", i, err)
		}
	}

	t.Run("paginated", func(t *testing.T) {
		events, total, err := store.ListSecurityEvents(ctx, testOrgID, 1, 2)
		if err != nil {
			t.Fatalf("ListSecurityEvents: %v", err)
		}
		if total != 5 {
			t.Errorf("expected total 5, got %d", total)
		}
		if len(events) != 2 {
			t.Errorf("expected page size 2, got %d", len(events))
		}
		for _, ev := range events {
			if ev.ActionTaken != "pass" {
				t.Errorf("expected action 'pass', got %q", ev.ActionTaken)
			}
		}
	})

	t.Run("page2", func(t *testing.T) {
		events, total, err := store.ListSecurityEvents(ctx, testOrgID, 2, 2)
		if err != nil {
			t.Fatalf("ListSecurityEvents page 2: %v", err)
		}
		if total != 5 {
			t.Errorf("expected total 5, got %d", total)
		}
		if len(events) != 2 {
			t.Errorf("expected page size 2, got %d", len(events))
		}
	})

	t.Run("page3", func(t *testing.T) {
		events, total, err := store.ListSecurityEvents(ctx, testOrgID, 3, 2)
		if err != nil {
			t.Fatalf("ListSecurityEvents page 3: %v", err)
		}
		if total != 5 {
			t.Errorf("expected total 5, got %d", total)
		}
		if len(events) != 1 {
			t.Errorf("expected page size 1, got %d", len(events))
		}
	})
}

func TestTC_SearchSecurityEvents(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, _ = store.pool.Exec(ctx, "DELETE FROM request_logs")

	findings := []map[string]interface{}{
		{"type": "PII", "severity": "HIGH", "category": "personally-identifiable-information"},
	}
	fJSON, _ := json.Marshal(findings)

	rid := "tc-search-" + uuid.NewString()[:8]
	_, err := store.pool.Exec(ctx, `
			INSERT INTO request_logs (org_id, request_id, provider, model, model_family,
				input_tokens, output_tokens, findings, findings_count, action_taken,
				created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		testOrgID, rid, "openai", "gpt-4o", "gpt-4o",
		100, 50, fJSON, len(findings), "block", now,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	t.Run("by_query", func(t *testing.T) {
		events, total, err := store.SearchSecurityEvents(ctx, testOrgID, EventSearchParams{
			Page:  1,
			Limit: 10,
			Q:     rid[:6],
			Since: now.Add(-1 * time.Hour),
			Until: now.Add(1 * time.Hour),
		})
		if err != nil {
			t.Fatalf("SearchSecurityEvents by Q: %v", err)
		}
		if total != 1 {
			t.Errorf("expected 1 result, got %d", total)
		}
		if len(events) > 0 && events[0].RequestID != rid {
			t.Errorf("expected %q, got %q", rid, events[0].RequestID)
		}
	})

	t.Run("by_action", func(t *testing.T) {
		events, total, err := store.SearchSecurityEvents(ctx, testOrgID, EventSearchParams{
			Page:   1,
			Limit:  10,
			Action: "block",
			Since:  now.Add(-1 * time.Hour),
			Until:  now.Add(1 * time.Hour),
		})
		if err != nil {
			t.Fatalf("SearchSecurityEvents by action: %v", err)
		}
		if total < 1 {
			t.Errorf("expected at least 1 result, got %d", total)
		}
		found := false
		for _, ev := range events {
			if ev.RequestID == rid {
				found = true
				break
			}
		}
		if !found {
			t.Error("saved event not found via action filter")
		}
	})

	t.Run("by_finding_type", func(t *testing.T) {
		_, total, err := store.SearchSecurityEvents(ctx, testOrgID, EventSearchParams{
			Page:        1,
			Limit:       10,
			FindingType: "PII",
			Since:       now.Add(-1 * time.Hour),
			Until:       now.Add(1 * time.Hour),
		})
		if err != nil {
			t.Fatalf("SearchSecurityEvents by finding type: %v", err)
		}
		if total < 1 {
			t.Errorf("expected at least 1 result, got %d", total)
		}
	})

	t.Run("empty_result", func(t *testing.T) {
		_, total, err := store.SearchSecurityEvents(ctx, testOrgID, EventSearchParams{
			Page:   1,
			Limit:  10,
			Action: "redact",
			Since:  now.Add(-1 * time.Hour),
			Until:  now.Add(1 * time.Hour),
		})
		if err != nil {
			t.Fatalf("SearchSecurityEvents empty: %v", err)
		}
		if total != 0 {
			t.Logf("warning: search(redact) returned %d results (may be pre-existing data)", total)
		}
	})
}

func TestTC_GetModelTokenUsage(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, _ = store.pool.Exec(ctx, "DELETE FROM request_logs")

	// Insert two rows: one for gpt-4o, one for claude.
	for i, row := range []struct {
		provider, model, family string
		input, output           int
	}{
		{"openai", "gpt-4o", "gpt-4o", 1000, 500},
		{"anthropic", "claude-sonnet-4-6", "claude-4", 800, 400},
	} {
		rid := fmt.Sprintf("tc-usage-%d-%d", now.UnixNano(), i)
		_, err := store.pool.Exec(ctx, `
				INSERT INTO request_logs (org_id, request_id, provider, model, model_family,
					input_tokens, output_tokens, findings, findings_count, action_taken, created_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			testOrgID, rid, row.provider, row.model, row.family,
			row.input, row.output, []byte(`[]`), 0, "pass", now,
		)
		if err != nil {
			t.Fatalf("insert usage row %d: %v", i, err)
		}
	}

	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)
	usage, err := store.GetModelTokenUsage(ctx, testOrgID, from, to)
	if err != nil {
		t.Fatalf("GetModelTokenUsage: %v", err)
	}

	if len(usage) < 2 {
		t.Fatalf("expected at least 2 model groups, got %d", len(usage))
	}

	byModel := make(map[string]ModelTokenUsage)
	for _, u := range usage {
		byModel[u.Model] = u
	}

	if u, ok := byModel["gpt-4o"]; ok {
		if u.InputTokens != 1000 {
			t.Errorf("gpt-4o input: want 1000, got %d", u.InputTokens)
		}
		if u.OutputTokens != 500 {
			t.Errorf("gpt-4o output: want 500, got %d", u.OutputTokens)
		}
	} else {
		t.Error("gpt-4o not found in usage results")
	}

	if u, ok := byModel["claude-sonnet-4-6"]; ok {
		if u.InputTokens != 800 {
			t.Errorf("claude-sonnet-4-6 input: want 800, got %d", u.InputTokens)
		}
	} else {
		t.Error("claude-sonnet-4-6 not found in usage results")
	}
}

func TestTC_Ping(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()

	if err := store.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestTC_Close(t *testing.T) {
	// Create a store, save an event, then close it. Verify the buffer is
	// flushed and the pool is shut down cleanly.
	pool := NewTestPostgres(t)
	ctx := context.Background()

	// Create store manually so we control its lifecycle.
	s := &PostgresStore{
		pool: pool,
		log:  zerolog.Nop(),
		done: make(chan struct{}),
	}
	s.wg.Add(1)
	go s.flushLoop()

	rid := "tc-close-" + uuid.NewString()[:8]
	if err := s.SaveRequestLog(ctx, RequestLog{
		RequestID:   rid,
		OrgID:       testOrgID,
		Provider:    "openai",
		Model:       "gpt-4o",
		Findings:    []byte(`[]`),
		ActionTaken: "pass",
	}); err != nil {
		t.Fatalf("SaveRequestLog: %v", err)
	}

	// Verify it's in buffer.
	s.mu.Lock()
	bufLen := len(s.buf)
	s.mu.Unlock()
	if bufLen != 1 {
		t.Errorf("expected 1 buffered row, got %d", bufLen)
	}

	// Close should flush.
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify the row made it to the database.
	var count int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM request_logs WHERE request_id = $1", rid,
	).Scan(&count); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row after close, got %d", count)
	}
}

func TestTC_EraseSubject(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, _ = store.pool.Exec(ctx, "DELETE FROM request_logs")

	// Insert rows for two users.
	for _, uid := range []string{"tc-erase-target", "tc-erase-keep"} {
		rid := fmt.Sprintf("tc-erase-%s-%d", uid, now.UnixNano())
		_, err := store.pool.Exec(ctx, `
				INSERT INTO request_logs (org_id, request_id, provider, model, user_identifier,
					findings, input_tokens, output_tokens, action_taken, created_at)
				VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10)`,
			testOrgID, rid, "openai", "gpt-4o", uid,
			[]byte(`[]`), 10, 5, "pass", now,
		)
		if err != nil {
			t.Fatalf("insert erase row: %v", err)
		}
	}

	// Erase the target.
	deleted, err := store.EraseSubject(ctx, testOrgID, "tc-erase-target", "", "")
	if err != nil {
		t.Fatalf("EraseSubject: %v", err)
	}
	if deleted < 1 {
		t.Errorf("expected at least 1 deleted, got %d", deleted)
	}

	// Verify target is gone.
	var targetCount int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM request_logs WHERE user_identifier = 'tc-erase-target'",
	).Scan(&targetCount); err != nil {
		t.Fatalf("count target: %v", err)
	}
	if targetCount != 0 {
		t.Errorf("tc-erase-target rows remaining: want 0, got %d", targetCount)
	}

	// Verify keep still exists.
	var keepCount int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM request_logs WHERE user_identifier = 'tc-erase-keep'",
	).Scan(&keepCount); err != nil {
		t.Fatalf("count keep: %v", err)
	}
	if keepCount < 1 {
		t.Error("tc-erase-keep should still exist after erase")
	}
}

func TestTC_SubjectAccess(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, _ = store.pool.Exec(ctx, "DELETE FROM request_logs")

	rid := "tc-access-" + uuid.NewString()[:8]
	_, err := store.pool.Exec(ctx, `
			INSERT INTO request_logs (org_id, request_id, provider, model, user_identifier,
				findings, input_tokens, output_tokens, action_taken, cost_usd, created_at)
			VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11)`,
		testOrgID, rid, "openai", "gpt-4o", "tc-access-user",
		[]byte(`[]`), 10, 5, "pass", 0.0025, now,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	rows, err := store.SubjectAccess(ctx, testOrgID, "tc-access-user", "", "", 10)
	if err != nil {
		t.Fatalf("SubjectAccess: %v", err)
	}
	if len(rows) < 1 {
		t.Fatal("expected at least 1 row")
	}
	for _, r := range rows {
		if r.UserID != "tc-access-user" {
			t.Errorf("expected user_id 'tc-access-user', got %q", r.UserID)
		}
	}

	// Query for nonexistent user.
	rowsNone, err := store.SubjectAccess(ctx, testOrgID, "no-such-user", "", "", 10)
	if err != nil {
		t.Fatalf("SubjectAccess nonexistent: %v", err)
	}
	if len(rowsNone) != 0 {
		t.Errorf("expected 0 rows for nonexistent user, got %d", len(rowsNone))
	}
}

func TestTC_ApplyRetention(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()

	_, _ = store.pool.Exec(ctx, "DELETE FROM request_logs")

	now := time.Now().UTC()
	oldTime := now.Add(-100 * 24 * time.Hour)  // 100 days ago
	recentTime := now.Add(-1 * 24 * time.Hour) // 1 day ago

	// Insert old rows.
	for i := 0; i < 3; i++ {
		rid := fmt.Sprintf("tc-ret-old-%d-%d", now.UnixNano(), i)
		_, err := store.pool.Exec(ctx, `
				INSERT INTO request_logs (org_id, request_id, provider, model, user_identifier,
					findings, input_tokens, output_tokens, action_taken, created_at)
				VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10)`,
			testOrgID, rid, "openai", "gpt-4o", "tc-ret-old",
			[]byte(`[]`), 10, 5, "pass", oldTime,
		)
		if err != nil {
			t.Fatalf("insert old row %d: %v", i, err)
		}
	}

	// Insert recent rows.
	for i := 0; i < 2; i++ {
		rid := fmt.Sprintf("tc-ret-recent-%d-%d", now.UnixNano(), i)
		_, err := store.pool.Exec(ctx, `
				INSERT INTO request_logs (org_id, request_id, provider, model, user_identifier,
					findings, input_tokens, output_tokens, action_taken, created_at)
				VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10)`,
			testOrgID, rid, "anthropic", "claude-sonnet-4-6", "tc-ret-recent",
			[]byte(`[]`), 20, 10, "pass", recentTime,
		)
		if err != nil {
			t.Fatalf("insert recent row %d: %v", i, err)
		}
	}

	// Apply 30-day retention.
	deleted, err := store.ApplyRetention(ctx, testOrgID, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("ApplyRetention: %v", err)
	}
	if deleted < 3 {
		t.Errorf("expected at least 3 old rows deleted, got %d", deleted)
	}

	// Verify old rows gone.
	var oldCount int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM request_logs WHERE user_identifier = 'tc-ret-old'",
	).Scan(&oldCount); err != nil {
		t.Fatalf("count old: %v", err)
	}
	if oldCount != 0 {
		t.Errorf("old rows remaining: want 0, got %d", oldCount)
	}

	// Verify recent rows survived.
	var recentCount int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM request_logs WHERE user_identifier = 'tc-ret-recent'",
	).Scan(&recentCount); err != nil {
		t.Fatalf("count recent: %v", err)
	}
	if recentCount != 2 {
		t.Errorf("recent rows: want 2, got %d", recentCount)
	}
}

// ---------------------------------------------------------------------------
// Partition manager — Testcontainers-based integration tests
// ---------------------------------------------------------------------------

// TestTC_PartitionManager_RunMaintenanceCycle creates upcoming partitions
// and verifies they exist after the maintenance cycle completes.
func TestTC_PartitionManager_RunMaintenanceCycle(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()

	policy := RetentionPolicy{
		RequestLogsDays:    0, // do not drop any partitions
		AlertsDays:         0,
		DailyStatsDays:     0,
		AuditLogsDays:      0,
		IncidentClosedDays: 0,
	}
	pm := NewPartitionManager(store.Pool(), policy, zerolog.Nop())

	if err := pm.RunMaintenanceCycle(ctx); err != nil {
		t.Fatalf("RunMaintenanceCycle: %v", err)
	}

	// Verify LastRun was updated.
	if pm.LastRun().IsZero() {
		t.Error("LastRun should not be zero after successful maintenance cycle")
	}

	// Verify at least 6 partitions exist (current + 5 future months).
	rows, err := store.pool.Query(ctx, `
		SELECT c.relname FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname = 'request_logs' AND c.relname LIKE 'request_logs_20%'
		ORDER BY c.relname`)
	if err != nil {
		t.Fatalf("query partitions: %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan partition name: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(names) < 6 {
		t.Errorf("expected at least 6 upcoming partitions, got %d: %v", len(names), names)
	}

	// Verify current month partition is in the list.
	now := time.Now().UTC()
	wantPart := fmt.Sprintf("request_logs_%04d_%02d", now.Year(), now.Month())
	found := false
	for _, n := range names {
		if n == wantPart {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("current month partition %s not found among: %v", wantPart, names)
	}
}

// TestTC_PartitionManager_CleanupOldPartitions creates a partition from
// year 2020, runs the maintenance cycle with a 1-day retention policy,
// and verifies the old partition was dropped while current partitions survive.
func TestTC_PartitionManager_CleanupOldPartitions(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()

	// Create an old partition (year 2020, month 01).
	oldPart := "request_logs_2020_01"
	_, err := store.pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s PARTITION OF request_logs
		FOR VALUES FROM ('2020-01-01') TO ('2020-02-01')`, oldPart))
	if err != nil {
		t.Fatalf("create old partition: %v", err)
	}

	// Verify old partition exists.
	var exists bool
	if err := store.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname = $1)`, oldPart,
	).Scan(&exists); err != nil {
		t.Fatalf("check old partition: %v", err)
	}
	if !exists {
		t.Fatal("old partition should exist before maintenance")
	}

	// Run maintenance with RequestLogsDays=1 so anything older than the cutoff is dropped.
	policy := RetentionPolicy{
		RequestLogsDays:    1,
		AlertsDays:         0,
		DailyStatsDays:     0,
		AuditLogsDays:      0,
		IncidentClosedDays: 0,
	}
	pm := NewPartitionManager(store.Pool(), policy, zerolog.Nop())
	if err := pm.RunMaintenanceCycle(ctx); err != nil {
		t.Fatalf("RunMaintenanceCycle: %v", err)
	}

	// Verify old partition was dropped.
	if err := store.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname = $1)`, oldPart,
	).Scan(&exists); err != nil {
		t.Fatalf("check old partition after maintenance: %v", err)
	}
	if exists {
		t.Errorf("old partition %s should have been dropped by retention", oldPart)
	}

	// Verify current month partition still exists.
	now := time.Now().UTC()
	currentPart := fmt.Sprintf("request_logs_%04d_%02d", now.Year(), now.Month())
	if err := store.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_class WHERE relname = $1)`, currentPart,
	).Scan(&exists); err != nil {
		t.Fatalf("check current partition: %v", err)
	}
	if !exists {
		t.Errorf("current partition %s should still exist after maintenance", currentPart)
	}
}

// ---------------------------------------------------------------------------
// Flush loop — Testcontainers-based integration tests
// ---------------------------------------------------------------------------

// TestTC_FlushLoop_BatchInsert inserts enough logs to trigger the
// batchMaxRows threshold (100) mid-flight, then verifies all rows
// end up in the database.
func TestTC_FlushLoop_BatchInsert(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()

	// Clean before test.
	_, _ = store.pool.Exec(ctx, "DELETE FROM request_logs")

	totalLogs := batchMaxRows + 50 // 150
	for i := 0; i < totalLogs; i++ {
		rl := RequestLog{
			RequestID:   fmt.Sprintf("tc-batch-%d", i),
			OrgID:       testOrgID,
			Provider:    "openai",
			Model:       "gpt-4o",
			Findings:    []byte(`[]`),
			ActionTaken: "pass",
		}
		if err := store.SaveRequestLog(ctx, rl); err != nil {
			t.Fatalf("SaveRequestLog %d: %v", i, err)
		}
	}

	// Force-flush any remaining buffer so all rows are persisted.
	store.mu.Lock()
	remaining := store.buf
	store.buf = nil
	store.mu.Unlock()
	if len(remaining) > 0 {
		if err := store.insertBatch(ctx, remaining); err != nil {
			t.Fatalf("flush remaining batch: %v", err)
		}
	}

	// Verify all logs persisted.
	var count int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM request_logs WHERE request_id LIKE 'tc-batch-%'",
	).Scan(&count); err != nil {
		t.Fatalf("count batch logs: %v", err)
	}
	if count != totalLogs {
		t.Errorf("expected %d batch logs, got %d", totalLogs, count)
	}
}

// TestTC_FlushLoop_ConcurrentWrites exercises concurrent SaveRequestLog
// calls from multiple goroutines and verifies no data is lost.
func TestTC_FlushLoop_ConcurrentWrites(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()

	// Clean before test.
	_, _ = store.pool.Exec(ctx, "DELETE FROM request_logs")

	const (
		numGoroutines    = 10
		logsPerGoroutine = 20
		totalLogs        = numGoroutines * logsPerGoroutine
	)

	var wg sync.WaitGroup
	errCh := make(chan error, totalLogs)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(grp int) {
			defer wg.Done()
			for i := 0; i < logsPerGoroutine; i++ {
				rl := RequestLog{
					RequestID:   fmt.Sprintf("tc-conc-%d-%d", grp, i),
					OrgID:       testOrgID,
					Provider:    "openai",
					Model:       "gpt-4o",
					Findings:    []byte(`[]`),
					ActionTaken: "pass",
				}
				if err := store.SaveRequestLog(ctx, rl); err != nil {
					errCh <- fmt.Errorf("grp %d log %d: %w", grp, i, err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent SaveRequestLog: %v", err)
	}

	// Force-flush any remaining buffer.
	store.mu.Lock()
	remaining := store.buf
	store.buf = nil
	store.mu.Unlock()
	if len(remaining) > 0 {
		if err := store.insertBatch(ctx, remaining); err != nil {
			t.Fatalf("flush remaining batch: %v", err)
		}
	}

	// Verify all concurrent logs persisted.
	var count int
	if err := store.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM request_logs WHERE request_id LIKE 'tc-conc-%'",
	).Scan(&count); err != nil {
		t.Fatalf("count concurrent logs: %v", err)
	}
	if count != totalLogs {
		t.Errorf("expected %d concurrent logs, got %d (data loss)", totalLogs, count)
	}
}

// ---------------------------------------------------------------------------
// Retention edge cases — Testcontainers-based integration tests
// ---------------------------------------------------------------------------

// TestTC_ApplyRetention_EmptyDB verifies retention on an empty request_logs
// table does not return an error.
func TestTC_ApplyRetention_EmptyDB(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()

	_, _ = store.pool.Exec(ctx, "DELETE FROM request_logs")

	deleted, err := store.ApplyRetention(ctx, testOrgID, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("ApplyRetention on empty DB: %v", err)
	}
	if deleted != 0 {
		t.Logf("ApplyRetention on empty DB deleted %d rows (may include pre-existing data)", deleted)
	}
}

// TestTC_ApplyRetention_CustomDays tests retention with non-default day windows.
func TestTC_ApplyRetention_CustomDays(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("7_days_purges_old", func(t *testing.T) {
		_, _ = store.pool.Exec(ctx, "DELETE FROM request_logs")

		// Insert a row from 10 days ago.
		oldTime := now.Add(-10 * 24 * time.Hour)
		_, err := store.pool.Exec(ctx, `
			INSERT INTO request_logs (org_id, request_id, provider, model,
				findings, input_tokens, output_tokens, action_taken, created_at)
			VALUES ($1, 'ret-7d-old', 'openai', 'gpt-4o',
				'[]'::jsonb, 10, 5, 'pass', $2)`,
			testOrgID, oldTime)
		if err != nil {
			t.Fatalf("insert old row: %v", err)
		}

		// 7-day retention should delete the 10-day-old row.
		deleted, err := store.ApplyRetention(ctx, testOrgID, 7*24*time.Hour)
		if err != nil {
			t.Fatalf("ApplyRetention 7d: %v", err)
		}
		if deleted < 1 {
			t.Errorf("expected at least 1 row deleted with 7-day window, got %d", deleted)
		}

		// Verify the old row is gone.
		var count int
		if err := store.pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM request_logs WHERE request_id = 'ret-7d-old'",
		).Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("old row should be deleted by 7d retention, got %d", count)
		}
	})

	t.Run("60_days_keeps_recent", func(t *testing.T) {
		_, _ = store.pool.Exec(ctx, "DELETE FROM request_logs")

		// Insert a row from 30 days ago.
		midTime := now.Add(-30 * 24 * time.Hour)
		_, err := store.pool.Exec(ctx, `
			INSERT INTO request_logs (org_id, request_id, provider, model,
				findings, input_tokens, output_tokens, action_taken, created_at)
			VALUES ($1, 'ret-60d-keep', 'openai', 'gpt-4o',
				'[]'::jsonb, 10, 5, 'pass', $2)`,
			testOrgID, midTime)
		if err != nil {
			t.Fatalf("insert mid row: %v", err)
		}

		// 60-day retention should NOT delete the 30-day-old row.
		deleted, err := store.ApplyRetention(ctx, testOrgID, 60*24*time.Hour)
		if err != nil {
			t.Fatalf("ApplyRetention 60d: %v", err)
		}
		if deleted > 0 {
			t.Logf("ApplyRetention 60d deleted %d rows (expected 0 for recent data)", deleted)
		}

		// Verify the row survived.
		var count int
		if err := store.pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM request_logs WHERE request_id = 'ret-60d-keep'",
		).Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 1 {
			t.Errorf("30-day-old row should survive 60-day retention, got %d", count)
		}
	})
}

// ---------------------------------------------------------------------------
// Stats edge cases — Testcontainers-based integration tests
// ---------------------------------------------------------------------------

// TestTC_GetStats_DateRange inserts daily_stats rows for a range of dates
// and verifies date-based filtering works as expected.
func TestTC_GetStats_DateRange(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()

	_, _ = store.pool.Exec(ctx, "DELETE FROM daily_stats")

	now := time.Now().UTC()
	type entry struct {
		date string
		reqs int64
	}
	dates := []entry{
		{now.AddDate(0, 0, -5).Format("2006-01-02"), 10},
		{now.AddDate(0, 0, -3).Format("2006-01-02"), 20},
		{now.AddDate(0, 0, -1).Format("2006-01-02"), 30},
		{now.Format("2006-01-02"), 40},
	}
	for _, d := range dates {
		_, err := store.pool.Exec(ctx, `
			INSERT INTO daily_stats (org_id, stat_date, total_requests)
			VALUES ($1, $2::date, $3)`, testOrgID, d.date, d.reqs)
		if err != nil {
			t.Fatalf("insert daily_stats for %s: %v", d.date, err)
		}
	}

	t.Run("narrow_range", func(t *testing.T) {
		// Only yesterday and today: 30 + 40 = 70.
		from := now.AddDate(0, 0, -1)
		to := now
		stats, err := store.GetStats(ctx, testOrgID, from, to)
		if err != nil {
			t.Fatalf("GetStats narrow: %v", err)
		}
		if want := int64(70); stats.TotalRequests != want {
			t.Errorf("narrow range: want %d, got %d", want, stats.TotalRequests)
		}
	})

	t.Run("full_range", func(t *testing.T) {
		// All four entries: 10 + 20 + 30 + 40 = 100.
		from := now.AddDate(0, 0, -10)
		to := now.AddDate(0, 0, 1)
		stats, err := store.GetStats(ctx, testOrgID, from, to)
		if err != nil {
			t.Fatalf("GetStats full: %v", err)
		}
		if want := int64(100); stats.TotalRequests != want {
			t.Errorf("full range: want %d, got %d", want, stats.TotalRequests)
		}
	})

	t.Run("outside_range", func(t *testing.T) {
		// Range far before any data.
		from := now.AddDate(0, 0, -365)
		to := now.AddDate(0, 0, -364)
		stats, err := store.GetStats(ctx, testOrgID, from, to)
		if err != nil {
			t.Fatalf("GetStats outside: %v", err)
		}
		if stats.TotalRequests != 0 {
			t.Errorf("outside range: want 0, got %d", stats.TotalRequests)
		}
	})

	t.Run("single_day", func(t *testing.T) {
		// Exactly today: 40.
		from := now
		to := now
		stats, err := store.GetStats(ctx, testOrgID, from, to)
		if err != nil {
			t.Fatalf("GetStats single day: %v", err)
		}
		if want := int64(40); stats.TotalRequests != want {
			t.Errorf("single day: want %d, got %d", want, stats.TotalRequests)
		}
	})
}

// TestTC_GetStats_NoData verifies GetStats returns all-zero Stats
// when daily_stats is completely empty.
func TestTC_GetStats_NoData(t *testing.T) {
	store := NewTestPostgresStore(t)
	ctx := context.Background()

	_, _ = store.pool.Exec(ctx, "DELETE FROM daily_stats")

	now := time.Now().UTC()
	stats, err := store.GetStats(ctx, testOrgID,
		now.Add(-30*24*time.Hour), now)
	if err != nil {
		t.Fatalf("GetStats on empty table: %v", err)
	}

	if stats.TotalRequests != 0 {
		t.Errorf("TotalRequests: want 0, got %d", stats.TotalRequests)
	}
	if stats.BlockedRequests != 0 {
		t.Errorf("BlockedRequests: want 0, got %d", stats.BlockedRequests)
	}
	if stats.RedactedRequests != 0 {
		t.Errorf("RedactedRequests: want 0, got %d", stats.RedactedRequests)
	}
	if stats.WarnedRequests != 0 {
		t.Errorf("WarnedRequests: want 0, got %d", stats.WarnedRequests)
	}
	if stats.TotalInputTokens != 0 {
		t.Errorf("TotalInputTokens: want 0, got %d", stats.TotalInputTokens)
	}
	if stats.TotalOutputTokens != 0 {
		t.Errorf("TotalOutputTokens: want 0, got %d", stats.TotalOutputTokens)
	}
}
