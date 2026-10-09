package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/scanner"
)

// These tests run the request_logs queries of history.go against a real
// PostgreSQL (TAMGA_TEST_DB_URL, or the container TestMain starts).

const histOtherOrgID = "00000000-0000-0000-0000-0000000000ff"

func pgInsertAt(t *testing.T, s *PostgresStore, org, id, provider, model, action string, at time.Time, scanMs float64, findings string, count int) {
	t.Helper()
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO request_logs (org_id, request_id, provider, model, model_family, endpoint,
			findings, findings_count, action_taken, scan_latency_ms, total_latency_ms, created_at)
		VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($4,'') || '-family','/v1/chat/completions',$5::jsonb,$6,$7,$8,$9,$10)`,
		org, id, provider, model, findings, count, action, scanMs, scanMs*10, at)
	if err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

// seedHistory stores, for the test organisation: 4 passed and 2 blocked
// requests one hour ago, 1 redacted two days ago, 1 blocked 60 days ago;
// and one blocked request of another organisation.
func seedHistory(t *testing.T, s *PostgresStore, now time.Time) {
	t.Helper()
	const pii = `[{"type":"pii","category":"email","severity":"high","match":"a***@b.c"},{"type":"pii","category":"phone_tr","severity":"High","match":"05**"}]`
	const secret = `[{"type":"secret","category":"aws_key","severity":"critical","match":"AKIA****"}]`
	hourAgo := now.Add(-time.Hour)
	for i := 0; i < 4; i++ {
		pgInsertAt(t, s, testOrgID, fmt.Sprintf("hist-pass-%d", i), "openai", "gpt-6-luna", "pass", hourAgo, float64(i+1), `[]`, 0)
	}
	// Both spellings of an action are in real tables: the bus writer stores
	// lower case, the NATS consumer upper case.
	pgInsertAt(t, s, testOrgID, "hist-block-0", "anthropic", "claude-haiku-4-5", "block", hourAgo, 5, secret, 1)
	pgInsertAt(t, s, testOrgID, "hist-block-1", "anthropic", "claude-haiku-4-5", "BLOCK", hourAgo, 100, secret, 1)
	pgInsertAt(t, s, testOrgID, "hist-redact-0", "gemini", "", "REDACT", now.Add(-48*time.Hour), 7, pii, 2)
	pgInsertAt(t, s, testOrgID, "hist-old-0", "openai", "gpt-6-luna", "block", now.Add(-60*24*time.Hour), 9, secret, 1)
	pgInsertAt(t, s, histOtherOrgID, "hist-theirs-0", "openai", "gpt-6-luna", "block", hourAgo, 50, secret, 1)
}

func TestPG_RequestHistory(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)
	ctx := context.Background()
	// Whole seconds in the middle of an hour, so no row sits on a bucket edge.
	now := time.Now().UTC().Truncate(time.Hour).Add(-30 * time.Minute)
	seedHistory(t, s, now)
	from, to := now.Add(-7*24*time.Hour), now

	t.Run("counts", func(t *testing.T) {
		rows, err := s.RequestCounts(ctx, testOrgID, from, to)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int64{}
		var total int64
		var latency float64
		for _, r := range rows {
			got[r.Provider+"/"+r.Action] += r.Requests
			total += r.Requests
			latency += r.ScanLatencySumMs
		}
		if total != 7 || got["openai/PASS"] != 4 || got["anthropic/BLOCK"] != 2 || got["gemini/REDACT"] != 1 {
			t.Errorf("counts = %v (total %d), want 7 rows of this organisation in the window", got, total)
		}
		if latency != 1+2+3+4+5+100+7 {
			t.Errorf("latency sum = %v", latency)
		}
	})

	t.Run("timeseries", func(t *testing.T) {
		for _, bucket := range []string{"hour", "day"} {
			rows, err := s.RequestTimeseries(ctx, testOrgID, from, to, bucket)
			if err != nil {
				t.Fatalf("%s: %v", bucket, err)
			}
			step := time.Hour
			if bucket == "day" {
				step = 24 * time.Hour
			}
			var total, blocked, redacted int64
			for i, r := range rows {
				if !r.Start.Equal(r.Start.UTC().Truncate(step)) {
					t.Errorf("%s bucket %v is not on a UTC boundary", bucket, r.Start)
				}
				if i > 0 && !rows[i-1].Start.Before(r.Start) {
					t.Errorf("%s buckets are not ascending", bucket)
				}
				total, blocked, redacted = total+r.Total, blocked+r.Blocked, redacted+r.Redacted
			}
			// The same requests RequestCounts counted.
			if total != 7 || blocked != 2 || redacted != 1 {
				t.Errorf("%s: total/blocked/redacted = %d/%d/%d, want 7/2/1", bucket, total, blocked, redacted)
			}
		}
		rows, _ := s.RequestTimeseries(ctx, testOrgID, now.Add(-2*time.Hour), to, "hour")
		want := now.Add(-time.Hour).Truncate(time.Hour)
		if len(rows) != 1 || !rows[0].Start.Equal(want) || rows[0].Total != 6 || rows[0].ScanP95Ms != 100 || rows[0].ScanSamples != 6 {
			t.Errorf("last two hours = %+v, want one bucket at %v with 6 requests and p95 100", rows, want)
		}
		if _, err := s.RequestTimeseries(ctx, testOrgID, from, to, "week'; DROP TABLE request_logs; --"); err == nil {
			t.Error("an unknown bucket was accepted")
		}
	})

	t.Run("findings", func(t *testing.T) {
		rows, err := s.FindingCounts(ctx, testOrgID, from, to)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int64{}
		for _, r := range rows {
			got[r.Type+"/"+r.Category+"/"+r.Severity] = r.Count
		}
		if len(got) != 3 || got["secret/aws_key/critical"] != 2 || got["pii/email/high"] != 1 || got["pii/phone_tr/high"] != 1 {
			t.Errorf("finding counts = %v", got)
		}
	})

	t.Run("models", func(t *testing.T) {
		rows, err := s.ModelCounts(ctx, testOrgID, from, to)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int64{}
		for _, r := range rows {
			got[r.Provider+"/"+r.Model+"/"+r.ModelFamily] = r.Requests
		}
		if len(got) != 3 || got["openai/gpt-6-luna/gpt-6-luna-family"] != 4 || got["anthropic/claude-haiku-4-5/claude-haiku-4-5-family"] != 2 || got["gemini//"] != 1 {
			t.Errorf("model counts = %v", got)
		}
	})

	t.Run("risk profiles", func(t *testing.T) {
		rows, err := s.RiskProfiles(ctx, testOrgID, from, to)
		if err != nil {
			t.Fatal(err)
		}
		// Two profiles: one critical secret (2 requests, 100%), two high pii
		// (1 request, 80%). The requests without findings have none.
		var requests, weighted int64
		for _, r := range rows {
			requests += r.Requests
			weighted += int64(scanner.CalculateRisk(r.Findings).Percentage) * r.Requests
		}
		if len(rows) != 2 || requests != 3 || weighted != 2*100+80 {
			t.Errorf("profiles = %+v (requests %d, weighted %d)", rows, requests, weighted)
		}
	})

	t.Run("detail", func(t *testing.T) {
		since := now.Add(-400 * 24 * time.Hour)
		rec, err := s.GetRequestRecord(ctx, testOrgID, "hist-redact-0", since)
		if err != nil || rec == nil {
			t.Fatalf("stored request: rec=%v err=%v", rec, err)
		}
		if rec.Provider != "gemini" || rec.Model != "" || rec.ActionTaken != "REDACT" || rec.FindingsCount != 2 ||
			rec.ScanLatencyMs != 7 || rec.TotalLatencyMs != 70 || rec.Endpoint != "/v1/chat/completions" ||
			!rec.CreatedAt.Equal(now.Add(-48*time.Hour)) || len(rec.Findings) == 0 {
			t.Errorf("record = %+v", rec)
		}
		if rec, err := s.GetRequestRecord(ctx, testOrgID, "no-such-request", since); err != nil || rec != nil {
			t.Errorf("unknown id: rec=%v err=%v, want nil, nil", rec, err)
		}
		// Another organisation's request is not found, by either of them.
		if rec, err := s.GetRequestRecord(ctx, testOrgID, "hist-theirs-0", since); err != nil || rec != nil {
			t.Errorf("another organisation's id: rec=%v err=%v, want nil, nil", rec, err)
		}
		if rec, _ := s.GetRequestRecord(ctx, histOtherOrgID, "hist-theirs-0", since); rec == nil {
			t.Error("the owning organisation does not find its own request")
		}
		// Older than the window asked for.
		if rec, _ := s.GetRequestRecord(ctx, testOrgID, "hist-old-0", now.Add(-24*time.Hour)); rec != nil {
			t.Error("a request older than since was returned")
		}
		if _, err := s.GetRequestRecord(ctx, "not-a-uuid", "x", since); err == nil {
			t.Error("a malformed organisation id was accepted")
		}
	})

	t.Run("export", func(t *testing.T) {
		all := EventSearchParams{Since: from, Until: to}
		rows, more, err := s.ExportRequestRecords(ctx, testOrgID, all, 0)
		if err != nil || more || len(rows) != 7 {
			t.Fatalf("export: %d rows, more=%v, err=%v; want 7 complete", len(rows), more, err)
		}
		for i := 1; i < len(rows); i++ {
			if rows[i].CreatedAt.After(rows[i-1].CreatedAt) {
				t.Error("export is not newest first")
			}
		}
		rows, more, _ = s.ExportRequestRecords(ctx, testOrgID, all, 3)
		if len(rows) != 3 || !more {
			t.Errorf("export limited to 3: %d rows, more=%v", len(rows), more)
		}
		blocked := all
		blocked.Action = "block"
		if rows, _, _ := s.ExportRequestRecords(ctx, testOrgID, blocked, 0); len(rows) != 2 {
			t.Errorf("export of blocked requests: %d rows, want 2 (both spellings)", len(rows))
		}
		byID := all
		byID.RequestID = "REDACT-"
		if rows, _, _ := s.ExportRequestRecords(ctx, testOrgID, byID, 0); len(rows) != 1 || rows[0].RequestID != "hist-redact-0" {
			t.Errorf("export by request id: %+v", rows)
		}
		// The text is a literal: a wildcard in it matches nothing here.
		byID.RequestID = "hist-%"
		if rows, _, _ := s.ExportRequestRecords(ctx, testOrgID, byID, 0); len(rows) != 0 {
			t.Errorf("a %% in the request id was read as a pattern: %d rows", len(rows))
		}
		// The list and the export agree on a filter.
		pii := all
		pii.FindingType = "pii"
		exported, _, _ := s.ExportRequestRecords(ctx, testOrgID, pii, 0)
		_, listed, err := s.SearchSecurityEvents(ctx, testOrgID, pii)
		if err != nil || len(exported) != 1 || listed != 1 {
			t.Errorf("finding_type=pii: exported %d, listed %d, err %v; want 1 and 1", len(exported), listed, err)
		}
	})
}

func TestRequestHistory_RejectsMalformedOrg(t *testing.T) {
	// The organisation id is parsed before the pool is touched.
	s := &PostgresStore{}
	ctx := context.Background()
	now := time.Now()
	if _, err := s.RequestCounts(ctx, "x", now, now); err == nil {
		t.Error("RequestCounts")
	}
	if _, err := s.RequestTimeseries(ctx, "x", now, now, "day"); err == nil {
		t.Error("RequestTimeseries")
	}
	if _, err := s.RequestTimeseries(ctx, uuid.NewString(), now, now, "century"); err == nil {
		t.Error("RequestTimeseries with an unknown bucket")
	}
	if _, err := s.FindingCounts(ctx, "x", now, now); err == nil {
		t.Error("FindingCounts")
	}
	if _, err := s.ModelCounts(ctx, "x", now, now); err == nil {
		t.Error("ModelCounts")
	}
	if _, err := s.RiskProfiles(ctx, "x", now, now); err == nil {
		t.Error("RiskProfiles")
	}
	if _, _, err := s.ExportRequestRecords(ctx, "x", EventSearchParams{}, 0); err == nil {
		t.Error("ExportRequestRecords")
	}
}

// A row is dated by its request, not by the flush that wrote it.
func TestDBHandler_DatesRowByEvent(t *testing.T) {
	spy := &spyStore{}
	at := time.Date(2026, 10, 8, 19, 51, 35, 0, time.UTC)
	DBHandler(zerolog.Nop(), spy, testOrgID, nil)(events.Event{EventType: "request_scanned", Timestamp: at})
	if !spy.lastAt.Equal(at) {
		t.Errorf("row dated %v, want the event's %v", spy.lastAt, at)
	}
}

// RiskProfiles counts findings of one type and severity up to riskCountCap.
// That is only sound while more findings than that leave the score unchanged.
func TestRiskCountCapLeavesScoreUnchanged(t *testing.T) {
	many := func(n int, sev string) []scanner.Finding {
		out := make([]scanner.Finding, n)
		for i := range out {
			out[i] = scanner.Finding{Type: "pii", Severity: sev}
		}
		return out
	}
	for _, sev := range []string{"low", "medium", "high", "critical", ""} {
		capped := scanner.CalculateRisk(many(riskCountCap, sev))
		for _, n := range []int{riskCountCap + 1, 50} {
			if got := scanner.CalculateRisk(many(n, sev)); got.Percentage != capped.Percentage || got.Level != capped.Level {
				t.Errorf("%d %q findings score %d%%, %d score %d%%: the cap in RiskProfiles changes the result", n, sev, got.Percentage, riskCountCap, capped.Percentage)
			}
		}
	}
	// Split across severities, the capped counts still add up to the cap.
	mixed := append(many(3, "low"), many(40, "high")...)
	cappedMixed := append(many(3, "low"), many(riskCountCap, "high")...)
	if a, b := scanner.CalculateRisk(mixed), scanner.CalculateRisk(cappedMixed); a.Percentage != b.Percentage {
		t.Errorf("mixed severities: %d%% uncapped, %d%% capped", a.Percentage, b.Percentage)
	}
}
