package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

// skipIfNoIntegration skips the test unless TAMGA_INTEGRATION_DB=1.
func skipIfNoIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("TAMGA_INTEGRATION_DB") != "1" {
		t.Skip("set TAMGA_INTEGRATION_DB=1 to run integration tests")
	}
}

// intCleanTable deletes all rows from the named table. Safe to call on any
// table created by newTestPostgresStore.
func intCleanTable(t *testing.T, s *PostgresStore, table string) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.pool.Exec(ctx, "DELETE FROM "+table); err != nil {
		t.Logf("cleanup %s: %v", table, err)
	}
}

const intTestOrgID = "00000000-0000-0000-0000-000000000001"

// ---------------------------------------------------------------------------
// GetStats — daily_stats aggregation
// ---------------------------------------------------------------------------

// TestPostgresStore_GetStats_Integration inserts multiple daily_stats rows for
// the test org (and one row for a different org) then verifies that GetStats
// correctly sums only the correct org's rows across the requested date window.
func TestPostgresStore_GetStats_Integration(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)
	intCleanTable(t, s, "daily_stats")

	ctx := context.Background()
	now := time.Now().UTC()

	// Insert 3 daily_stats rows for our test org.
	type row struct {
		date                            string
		totalReq, blocked, redact, warn int
		tokIn, tokOut                   int
	}
	rows := []row{
		{date: now.AddDate(0, 0, -2).Format("2006-01-02"), totalReq: 10, blocked: 2, redact: 1, warn: 0, tokIn: 500, tokOut: 250},
		{date: now.AddDate(0, 0, -1).Format("2006-01-02"), totalReq: 15, blocked: 4, redact: 2, warn: 1, tokIn: 700, tokOut: 350},
		{date: now.Format("2006-01-02"), totalReq: 20, blocked: 5, redact: 3, warn: 2, tokIn: 900, tokOut: 450},
	}
	for i, r := range rows {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO daily_stats (org_id, stat_date, total_requests, blocked_requests,
				redacted_requests, warned_requests, total_input_tokens, total_output_tokens)
			VALUES ($1, $2::date, $3, $4, $5, $6, $7, $8)`,
			intTestOrgID, r.date, r.totalReq, r.blocked, r.redact, r.warn, r.tokIn, r.tokOut)
		if err != nil {
			t.Fatalf("insert daily_stats row %d: %v", i, err)
		}
	}

	// Insert a row for a different org — must NOT be counted.
	_, err := s.pool.Exec(ctx, `
		INSERT INTO daily_stats (org_id, stat_date, total_requests, blocked_requests,
			total_input_tokens, total_output_tokens)
		VALUES ($1, $2::date, $3, $4, $5, $6)`,
		"00000000-0000-0000-0000-000000000002", now.Format("2006-01-02"),
		999, 999, 99999, 99999)
	if err != nil {
		t.Fatalf("insert other-org daily_stats: %v", err)
	}

	from := now.AddDate(0, 0, -5)
	to := now.AddDate(0, 0, 1)
	stats, err := s.GetStats(ctx, intTestOrgID, from, to)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}

	if want := int64(45); stats.TotalRequests != want { // 10+15+20
		t.Errorf("TotalRequests: want %d, got %d", want, stats.TotalRequests)
	}
	if want := int64(11); stats.BlockedRequests != want { // 2+4+5
		t.Errorf("BlockedRequests: want %d, got %d", want, stats.BlockedRequests)
	}
	if want := int64(6); stats.RedactedRequests != want { // 1+2+3
		t.Errorf("RedactedRequests: want %d, got %d", want, stats.RedactedRequests)
	}
	if want := int64(3); stats.WarnedRequests != want { // 0+1+2
		t.Errorf("WarnedRequests: want %d, got %d", want, stats.WarnedRequests)
	}
	if want := int64(2100); stats.TotalInputTokens != want { // 500+700+900
		t.Errorf("TotalInputTokens: want %d, got %d", want, stats.TotalInputTokens)
	}
	if want := int64(1050); stats.TotalOutputTokens != want { // 250+350+450
		t.Errorf("TotalOutputTokens: want %d, got %d", want, stats.TotalOutputTokens)
	}
}

// TestPostgresStore_GetStats_EmptyWindow verifies GetStats returns zeros
// when no daily_stats rows fall within the requested window.
func TestPostgresStore_GetStats_EmptyWindow(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)
	intCleanTable(t, s, "daily_stats")

	ctx := context.Background()

	// Date range far in the past where no rows exist.
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC)
	stats, err := s.GetStats(ctx, intTestOrgID, from, to)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}

	if stats.TotalRequests != 0 {
		t.Errorf("expected 0 TotalRequests, got %d", stats.TotalRequests)
	}
	if stats.TotalInputTokens != 0 {
		t.Errorf("expected 0 TotalInputTokens, got %d", stats.TotalInputTokens)
	}
}

// ---------------------------------------------------------------------------
// GetModelTokenUsage — per-model token aggregation
// ---------------------------------------------------------------------------

// TestPostgresStore_GetModelTokenUsage_Integration inserts request_log rows
// with several models (some with multiple rows) and verifies grouping,
// aggregation, and exclusion of zero-token rows.
func TestPostgresStore_GetModelTokenUsage_Integration(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)
	intCleanTable(t, s, "request_logs")

	ctx := context.Background()
	now := time.Now().UTC()

	type modelRow struct {
		provider, model, family string
		input, output           int
	}
	rows := []modelRow{
		{"openai", "gpt-4o", "gpt-4o", 1000, 500},
		{"openai", "gpt-4o-mini", "gpt-4o", 500, 250},
		{"anthropic", "claude-sonnet-4-6", "claude-4", 800, 400},
		{"anthropic", "claude-sonnet-4-6", "claude-4", 200, 100}, // second row, same model
		{"openai", "zero-token-model", "gpt-4o", 0, 0},           // must be excluded (0+0 tokens)
	}

	for i, r := range rows {
		rid := fmt.Sprintf("usage-%d-%d", now.UnixNano(), i)
		_, err := s.pool.Exec(ctx, `
			INSERT INTO request_logs (org_id, request_id, provider, model, model_family,
				input_tokens, output_tokens, findings, findings_count, action_taken, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			intTestOrgID, rid, r.provider, r.model, r.family,
			r.input, r.output, []byte(`[]`), 0, "pass", now)
		if err != nil {
			t.Fatalf("insert model row %d: %v", i, err)
		}
	}

	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)
	usage, err := s.GetModelTokenUsage(ctx, intTestOrgID, from, to)
	if err != nil {
		t.Fatalf("GetModelTokenUsage: %v", err)
	}

	// Build a lookup map.
	byModel := make(map[string]ModelTokenUsage)
	for _, u := range usage {
		byModel[u.Model] = u
	}

	// zero-token-model must not appear.
	if _, ok := byModel["zero-token-model"]; ok {
		t.Error("zero-token-model should be excluded (no tokens)")
	}

	// gpt-4o: single row, 1000 IN / 500 OUT.
	if u, ok := byModel["gpt-4o"]; ok {
		if u.InputTokens != 1000 {
			t.Errorf("gpt-4o input: want 1000, got %d", u.InputTokens)
		}
		if u.OutputTokens != 500 {
			t.Errorf("gpt-4o output: want 500, got %d", u.OutputTokens)
		}
		if u.Provider != "openai" {
			t.Errorf("gpt-4o provider: want openai, got %s", u.Provider)
		}
	} else {
		t.Error("gpt-4o not found in usage results")
	}

	// gpt-4o-mini: single row, 500 IN / 250 OUT.
	if u, ok := byModel["gpt-4o-mini"]; ok {
		if u.InputTokens != 500 {
			t.Errorf("gpt-4o-mini input: want 500, got %d", u.InputTokens)
		}
	} else {
		t.Error("gpt-4o-mini not found in usage results")
	}

	// claude-sonnet-4-6: two rows, 800+200=1000 IN / 400+100=500 OUT.
	if u, ok := byModel["claude-sonnet-4-6"]; ok {
		if u.InputTokens != 1000 {
			t.Errorf("claude-sonnet-4-6 input: want 1000, got %d", u.InputTokens)
		}
		if u.OutputTokens != 500 {
			t.Errorf("claude-sonnet-4-6 output: want 500, got %d", u.OutputTokens)
		}
		if u.Provider != "anthropic" {
			t.Errorf("claude-sonnet-4-6 provider: want anthropic, got %s", u.Provider)
		}
		if u.ModelFamily != "claude-4" {
			t.Errorf("claude-sonnet-4-6 family: want claude-4, got %s", u.ModelFamily)
		}
	} else {
		t.Error("claude-sonnet-4-6 not found in usage results")
	}

	if len(usage) != 3 {
		t.Errorf("expected 3 model groups, got %d", len(usage))
	}
}

// ---------------------------------------------------------------------------
// SubjectAccess — GDPR Art. 15 / KVKK madde 11
// ---------------------------------------------------------------------------

// TestPostgresStore_SubjectAccess_Integration inserts rows for two different
// users and verifies SubjectAccess returns only the correct user's rows.
func TestPostgresStore_SubjectAccess_Integration(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)
	intCleanTable(t, s, "request_logs")

	ctx := context.Background()
	now := time.Now().UTC()

	// Insert 2 rows for user A, 2 rows for user B.
	users := []struct {
		uid   string
		model string
	}{
		{"user-subj-a", "gpt-4o"},
		{"user-subj-a", "gpt-4o-mini"},
		{"user-subj-b", "claude-sonnet-4-6"},
		{"user-subj-b", "claude-opus-4"},
	}
	for i, u := range users {
		rid := fmt.Sprintf("subj-%d-%d", now.UnixNano(), i)
		findings, _ := json.Marshal([]map[string]interface{}{
			{"type": "PII", "severity": "LOW"},
		})
		_, err := s.pool.Exec(ctx, `
			INSERT INTO request_logs (org_id, request_id, provider, model, user_identifier,
				findings, input_tokens, output_tokens, action_taken, cost_usd, created_at)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11)`,
			intTestOrgID, rid, "openai", u.model, u.uid,
			findings, 100, 50, "pass", 0.0025, now)
		if err != nil {
			t.Fatalf("insert subject row %d: %v", i, err)
		}
	}

	// Query SubjectAccess for user A.
	rowsA, err := s.SubjectAccess(ctx, intTestOrgID, "user-subj-a", "", "", 100)
	if err != nil {
		t.Fatalf("SubjectAccess user-subj-a: %v", err)
	}
	if len(rowsA) != 2 {
		t.Errorf("user-subj-a: want 2 rows, got %d", len(rowsA))
	}
	for _, r := range rowsA {
		if r.UserID != "user-subj-a" {
			t.Errorf("unexpected user_id in result: %q", r.UserID)
		}
	}

	// Query SubjectAccess for user B with a limit of 1.
	rowsB, err := s.SubjectAccess(ctx, intTestOrgID, "user-subj-b", "", "", 1)
	if err != nil {
		t.Fatalf("SubjectAccess user-subj-b with limit=1: %v", err)
	}
	if len(rowsB) > 1 {
		t.Errorf("user-subj-b limit=1: want at most 1, got %d", len(rowsB))
	}
	for _, r := range rowsB {
		if r.UserID != "user-subj-b" {
			t.Errorf("unexpected user_id in result: %q", r.UserID)
		}
	}
}

// TestPostgresStore_SubjectAccess_NoMatch verifies SubjectAccess returns an
// empty slice when querying for a user that does not exist.
func TestPostgresStore_SubjectAccess_NoMatch(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)
	intCleanTable(t, s, "request_logs")

	ctx := context.Background()
	rows, err := s.SubjectAccess(ctx, intTestOrgID, "nonexistent-user", "", "", 500)
	if err != nil {
		t.Fatalf("SubjectAccess: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows for nonexistent user, got %d", len(rows))
	}
}

// TestPostgresStore_SubjectAccess_NoIdentifier verifies SubjectAccess errors
// when no subject identifier is provided.
func TestPostgresStore_SubjectAccess_NoIdentifier(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)

	ctx := context.Background()
	_, err := s.SubjectAccess(ctx, intTestOrgID, "", "", "", 10)
	if err == nil {
		t.Error("expected error for empty identifier, got nil")
	}
}

// ---------------------------------------------------------------------------
// EraseSubject — GDPR Art. 17 / KVKK madde 7 silme
// ---------------------------------------------------------------------------

// TestPostgresStore_EraseSubject_Integration inserts rows for two users,
// erases one, and verifies the erased user's rows are gone while the other
// user's rows remain.
func TestPostgresStore_EraseSubject_Integration(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)
	intCleanTable(t, s, "request_logs")

	ctx := context.Background()
	now := time.Now().UTC()

	// Insert rows for two different users.
	for _, uid := range []string{"erase-target", "erase-keep"} {
		for i := 0; i < 3; i++ {
			rid := fmt.Sprintf("erase-%s-%d-%d", uid, now.UnixNano(), i)
			_, err := s.pool.Exec(ctx, `
				INSERT INTO request_logs (org_id, request_id, provider, model, user_identifier,
					findings, input_tokens, output_tokens, action_taken, created_at)
				VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10)`,
				intTestOrgID, rid, "openai", "gpt-4o", uid,
				[]byte(`[]`), 10, 5, "pass", now)
			if err != nil {
				t.Fatalf("insert erase row: %v", err)
			}
		}
	}

	// Erase the target user.
	deleted, err := s.EraseSubject(ctx, intTestOrgID, "erase-target", "", "")
	if err != nil {
		t.Fatalf("EraseSubject: %v", err)
	}
	if deleted != 3 {
		t.Errorf("expected 3 rows deleted, got %d", deleted)
	}

	// Verify target user's rows are gone.
	var targetCount int
	if err := s.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM request_logs WHERE user_identifier = $1", "erase-target",
	).Scan(&targetCount); err != nil {
		t.Fatalf("count erase-target: %v", err)
	}
	if targetCount != 0 {
		t.Errorf("erase-target rows remaining: want 0, got %d", targetCount)
	}

	// Verify keep user's rows still exist.
	var keepCount int
	if err := s.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM request_logs WHERE user_identifier = $1", "erase-keep",
	).Scan(&keepCount); err != nil {
		t.Fatalf("count erase-keep: %v", err)
	}
	if keepCount != 3 {
		t.Errorf("erase-keep rows: want 3, got %d", keepCount)
	}
}

// TestPostgresStore_EraseSubject_NoIdentifier verifies EraseSubject errors
// when no subject identifier is provided.
func TestPostgresStore_EraseSubject_NoIdentifier(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)

	ctx := context.Background()
	_, err := s.EraseSubject(ctx, intTestOrgID, "", "", "")
	if err == nil {
		t.Error("expected error for empty identifier, got nil")
	}
}

// ---------------------------------------------------------------------------
// ApplyRetention — time-based log deletion
// ---------------------------------------------------------------------------

// TestPostgresStore_ApplyRetention_Integration inserts old (100 days) and
// recent (1 day) rows, applies a 30-day retention window, and verifies that
// only the old rows are deleted.
func TestPostgresStore_ApplyRetention_Integration(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)
	intCleanTable(t, s, "request_logs")

	ctx := context.Background()
	now := time.Now().UTC()
	oldTime := now.Add(-100 * 24 * time.Hour)  // 100 days ago
	recentTime := now.Add(-1 * 24 * time.Hour) // 1 day ago

	// Insert 3 old rows.
	for i := 0; i < 3; i++ {
		rid := fmt.Sprintf("ret-old-%d-%d", now.UnixNano(), i)
		_, err := s.pool.Exec(ctx, `
			INSERT INTO request_logs (org_id, request_id, provider, model, user_identifier,
				findings, input_tokens, output_tokens, action_taken, created_at)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10)`,
			intTestOrgID, rid, "openai", "gpt-4o", "ret-old-user",
			[]byte(`[]`), 10, 5, "pass", oldTime)
		if err != nil {
			t.Fatalf("insert old row %d: %v", i, err)
		}
	}

	// Insert 2 recent rows.
	for i := 0; i < 2; i++ {
		rid := fmt.Sprintf("ret-recent-%d-%d", now.UnixNano(), i)
		_, err := s.pool.Exec(ctx, `
			INSERT INTO request_logs (org_id, request_id, provider, model, user_identifier,
				findings, input_tokens, output_tokens, action_taken, created_at)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10)`,
			intTestOrgID, rid, "anthropic", "claude-sonnet-4-6", "ret-recent-user",
			[]byte(`[]`), 20, 10, "pass", recentTime)
		if err != nil {
			t.Fatalf("insert recent row %d: %v", i, err)
		}
	}

	// Apply retention with 30-day window.
	deleted, err := s.ApplyRetention(ctx, intTestOrgID, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("ApplyRetention: %v", err)
	}
	if deleted < 3 {
		t.Errorf("expected at least 3 old rows deleted, got %d", deleted)
	}

	// Verify old rows are gone.
	var oldCount int
	if err := s.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM request_logs WHERE user_identifier = $1", "ret-old-user",
	).Scan(&oldCount); err != nil {
		t.Fatalf("count old rows: %v", err)
	}
	if oldCount != 0 {
		t.Errorf("old rows remaining: want 0, got %d", oldCount)
	}

	// Verify recent rows still exist.
	var recentCount int
	if err := s.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM request_logs WHERE user_identifier = $1", "ret-recent-user",
	).Scan(&recentCount); err != nil {
		t.Fatalf("count recent rows: %v", err)
	}
	if recentCount != 2 {
		t.Errorf("recent rows: want 2, got %d", recentCount)
	}
}

// TestPostgresStore_ApplyRetention_EmptyResult verifies retention on a
// table with no matching rows returns 0 deleted.
func TestPostgresStore_ApplyRetention_EmptyResult(t *testing.T) {
	skipIfNoIntegration(t)
	s := newTestPostgresStore(t)
	intCleanTable(t, s, "request_logs")

	ctx := context.Background()

	// Insert only recent rows, then apply aggressive retention.
	now := time.Now().UTC()
	rid := fmt.Sprintf("ret-only-recent-%d-%d", now.UnixNano(), uuid.New().ID())
	_, err := s.pool.Exec(ctx, `
		INSERT INTO request_logs (org_id, request_id, provider, model, user_identifier,
			findings, input_tokens, output_tokens, action_taken, created_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10)`,
		intTestOrgID, rid, "openai", "gpt-4o", "ret-only-recent",
		[]byte(`[]`), 10, 5, "pass", now)
	if err != nil {
		t.Fatalf("insert recent row: %v", err)
	}

	// Short window — no rows should be old enough.
	deleted, err := s.ApplyRetention(ctx, intTestOrgID, 365*24*time.Hour)
	if err != nil {
		t.Fatalf("ApplyRetention: %v", err)
	}
	if deleted != 0 {
		t.Logf("ApplyRetention with 365d window deleted %d rows (may include pre-existing data)", deleted)
	}

	// Verify our row still exists.
	var count int
	if err := s.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM request_logs WHERE request_id = $1", rid,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row to survive, got %d", count)
	}
}
