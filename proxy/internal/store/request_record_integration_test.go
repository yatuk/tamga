package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/events"
)

// These tests run against a PostgreSQL that has every migration of
// deploy/migrations applied (NewTestPostgresStore), so they see the table,
// the partitions and the indexes an installation has.

func migrationSQL(t *testing.T, name string) []string {
	t.Helper()
	dir, err := findMigrationsDir()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return splitPlainStatements(string(b))
}

func execAll(t *testing.T, s *PostgresStore, stmts []string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := s.pool.Exec(context.Background(), stmt); err != nil {
			t.Fatalf("%v\nstatement: %s", err, stmt)
		}
	}
}

func recColumns(t *testing.T, s *PostgresStore) map[string]bool {
	t.Helper()
	rows, err := s.pool.Query(context.Background(),
		`SELECT attname::text FROM pg_attribute WHERE attrelid = 'request_logs'::regclass AND attnum > 0 AND NOT attisdropped`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		out[n] = true
	}
	return out
}

// rowText is everything a stored request says, as text, without the two
// values that differ between any two rows.
func rowText(t *testing.T, s *PostgresStore, requestID string) string {
	t.Helper()
	var out string
	err := s.pool.QueryRow(context.Background(),
		`SELECT (to_jsonb(r) - 'id' - 'request_id')::text FROM request_logs r WHERE request_id = $1`, requestID).Scan(&out)
	if err != nil {
		t.Fatalf("row %s: %v", requestID, err)
	}
	return out
}

func countRows(t *testing.T, s *PostgresStore, where string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM request_logs WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTC_RequestRecord(t *testing.T) {
	s := NewTestPostgresStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Hour).Add(-30 * time.Minute)
	since := now.Add(-24 * time.Hour)

	// A monthly partition made after the migrations, as the partition
	// manager makes them, beside the default one.
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	month := fmt.Sprintf("request_logs_%04d_%02d", now.Year(), int(now.Month()))
	if _, err := s.pool.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s PARTITION OF request_logs FOR VALUES FROM ('%s') TO ('%s')`,
		month, monthStart.Format("2006-01-02"), monthStart.AddDate(0, 1, 0).Format("2006-01-02"))); err != nil {
		t.Fatal(err)
	}

	full := fullRequestEvent()
	full.OrgID, full.Timestamp = testOrgID, now

	t.Run("migration 017 is in place and can be run again", func(t *testing.T) {
		cols := recColumns(t, s)
		for _, c := range []string{"policy_name", "policy_version", "output_risk"} {
			if !cols[c] {
				t.Errorf("column %s is missing after the migrations", c)
			}
		}
		execAll(t, s, migrationSQL(t, "017_request_record.up.sql"))
		// The parent's index, and one on each partition: the default, and
		// the monthly one that was created after it.
		for _, table := range []string{"request_logs", "request_logs_default", month} {
			n := 0
			if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM pg_indexes WHERE tablename = $1
				AND indexdef LIKE '%(org_id, request_id, created_at DESC)%'`, table).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 1 {
				t.Errorf("%s has %d request-id indexes, want 1", table, n)
			}
		}
	})

	t.Run("both writers store the same row, with every column", func(t *testing.T) {
		viaBus, viaNATS := full, full
		viaBus.RequestID, viaNATS.RequestID = "rec-bus", "rec-nats"
		busRow, _ := RequestLogFromEvent(viaBus, testOrgID, nil)
		natsRow, ok := RequestLogFromV2(overNATS(t, viaNATS), testOrgID, nil)
		if !ok {
			t.Fatal("the NATS consumer did not take the event")
		}
		if err := s.insertBatch(ctx, []RequestLog{busRow, natsRow}); err != nil {
			t.Fatal(err)
		}
		bus, nats := rowText(t, s, "rec-bus"), rowText(t, s, "rec-nats")
		if bus != nats {
			t.Errorf("the rows differ:\n bus  %s\n nats %s", bus, nats)
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(bus), &raw); err != nil {
			t.Fatal(err)
		}
		for col, want := range map[string]interface{}{
			"action_taken": "redact", "output_action": "redact", "cache_status": "miss", "policy_name": "bank-policy",
			"policy_version": "7", "model_family": "gpt-4o", "endpoint": "/v1/chat/completions",
			"input_tokens": 1200.0, "output_tokens": 300.0, "cost_usd": 0.00036, "scan_latency_ms": 0.347, "total_latency_ms": 812.5,
		} {
			if raw[col] != want {
				t.Errorf("column %s = %v, want %v", col, raw[col], want)
			}
		}
		// Nothing in the proxy knows a policy channel: the column stays empty.
		if raw["policy_channel"] != nil {
			t.Errorf("policy_channel = %v", raw["policy_channel"])
		}

		rec, err := s.GetRequestRecord(ctx, testOrgID, "rec-nats", since)
		if err != nil || rec == nil {
			t.Fatalf("GetRequestRecord: %v, %v", rec, err)
		}
		if rec.InputTokens == nil || *rec.InputTokens != 1200 || rec.OutputTokens == nil || *rec.OutputTokens != 300 {
			t.Errorf("tokens = %v/%v", rec.InputTokens, rec.OutputTokens)
		}
		if rec.CostUSD == nil || *rec.CostUSD != 0.00036 {
			t.Errorf("cost = %v", rec.CostUSD)
		}
		// Sub-millisecond precision survives the REAL column, without the
		// noise of its conversion (0.347 and not 0.3470000028...).
		if rec.ScanLatencyMs != 0.347 || rec.TotalLatencyMs != 812.5 {
			t.Errorf("latency = %v / %v", rec.ScanLatencyMs, rec.TotalLatencyMs)
		}
		if rec.CacheStatus != "miss" || rec.OutputAction != "redact" || rec.PolicyName != "bank-policy" || rec.PolicyVersion != "7" {
			t.Errorf("record = %+v", rec)
		}
		// The response's findings are stored as the request's are: masked.
		if !strings.Contains(string(rec.OutputFindings), `"match": "05** *** ** 23"`) && !strings.Contains(string(rec.OutputFindings), `"match":"05** *** ** 23"`) {
			t.Errorf("output findings = %s", rec.OutputFindings)
		}
		if !strings.Contains(string(rec.OutputRisk), `"level"`) {
			t.Errorf("output risk = %s", rec.OutputRisk)
		}
	})

	t.Run("unknown is stored as NULL and read as unknown", func(t *testing.T) {
		e := full
		e.RequestID = "rec-unknown"
		e.InputTokens, e.OutputTokens, e.CostKnown, e.CostUSD = 0, 0, false, 0
		e.OutputFindings, e.OutputAction, e.OutputScanned = nil, "", false
		e.CacheStatus, e.PolicyName, e.PolicyVersion = "", "", ""
		row, _ := RequestLogFromEvent(e, testOrgID, nil)
		if err := s.insertBatch(ctx, []RequestLog{row}); err != nil {
			t.Fatal(err)
		}
		if n := countRows(t, s, `request_id = 'rec-unknown' AND input_tokens IS NULL AND output_tokens IS NULL AND cost_usd IS NULL
			AND output_findings IS NULL AND output_action IS NULL AND output_risk IS NULL AND cache_status IS NULL
			AND policy_name IS NULL AND policy_version IS NULL`); n != 1 {
			t.Errorf("the unknown values are not NULL: %s", rowText(t, s, "rec-unknown"))
		}
		rec, err := s.GetRequestRecord(ctx, testOrgID, "rec-unknown", since)
		if err != nil || rec == nil {
			t.Fatal(rec, err)
		}
		if rec.InputTokens != nil || rec.OutputTokens != nil || rec.CostUSD != nil || rec.OutputFindings != nil || rec.OutputRisk != nil {
			t.Errorf("unknown values were read as known: %+v", rec)
		}

		// A row of an older version: 0 and 0 where the counts were never
		// written. Not a request of no tokens.
		if _, err := s.pool.Exec(ctx, `INSERT INTO request_logs (org_id, request_id, provider, action_taken, created_at)
			VALUES ($1, 'rec-legacy', 'openai', 'PASS', $2)`, testOrgID, now); err != nil {
			t.Fatal(err)
		}
		rec, err = s.GetRequestRecord(ctx, testOrgID, "rec-legacy", since)
		if err != nil || rec == nil {
			t.Fatal(rec, err)
		}
		if rec.InputTokens != nil || rec.OutputTokens != nil || rec.PolicyName != "" || rec.OutputRisk != nil {
			t.Errorf("an older row was read as knowing its tokens: %+v", rec)
		}

		// A cache hit: a cost that is known to be zero, tokens unknown.
		hit := e
		hit.RequestID, hit.CacheStatus, hit.CostKnown = "rec-hit", "hit", true
		row, _ = RequestLogFromEvent(hit, testOrgID, nil)
		if err := s.insertBatch(ctx, []RequestLog{row}); err != nil {
			t.Fatal(err)
		}
		rec, _ = s.GetRequestRecord(ctx, testOrgID, "rec-hit", since)
		if rec == nil || rec.CostUSD == nil || *rec.CostUSD != 0 || rec.CacheStatus != "hit" || rec.InputTokens != nil {
			t.Errorf("the hit's record = %+v", rec)
		}
	})

	t.Run("an event delivered again does not add a row", func(t *testing.T) {
		e := full
		e.RequestID = "rec-once"
		row, _ := RequestLogFromEvent(e, testOrgID, nil)
		redelivered, _ := RequestLogFromV2(overNATS(t, e), testOrgID, nil)
		// Twice in one batch, then again in a later one, through each writer.
		if err := s.insertBatch(ctx, []RequestLog{row, redelivered}); err != nil {
			t.Fatal(err)
		}
		if err := s.insertBatch(ctx, []RequestLog{redelivered, row}); err != nil {
			t.Fatal(err)
		}
		if n := countRows(t, s, "request_id = 'rec-once'"); n != 1 {
			t.Fatalf("%d rows for one request, want 1", n)
		}
		// So the billing figures count its tokens once: with rec-bus and
		// rec-nats that is three requests of 1200 and 300.
		usage, err := s.GetDailyTokenUsage(ctx, testOrgID, since, now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		var in, out int64
		for _, u := range usage {
			if u.Model != "gpt-4o-mini" || u.Provider != "openai" {
				t.Errorf("usage row %+v", u)
			}
			in += u.InputTokens
			out += u.OutputTokens
		}
		if in != 3*1200 || out != 3*300 {
			t.Errorf("billing usage = %d in, %d out; want %d and %d", in, out, 3*1200, 3*300)
		}
		models, err := s.GetModelTokenUsage(ctx, testOrgID, since, now.Add(time.Hour))
		if err != nil || len(models) != 1 || models[0].InputTokens != 3*1200 {
			t.Errorf("model usage = %+v (%v)", models, err)
		}
		// The same request id at another time is another event, and the
		// same event of another organisation is that organisation's.
		later := row
		later.At = row.At.Add(time.Second)
		theirs := row
		theirs.OrgID = histOtherOrgID
		if err := s.insertBatch(ctx, []RequestLog{later, theirs}); err != nil {
			t.Fatal(err)
		}
		if n := countRows(t, s, "request_id = 'rec-once'"); n != 3 {
			t.Errorf("%d rows, want 3", n)
		}
		if _, err := s.pool.Exec(ctx, `DELETE FROM request_logs WHERE request_id = 'rec-once' AND (created_at <> $1 OR org_id <> $2)`, now, testOrgID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("latency figures use the fractions and skip what was not recorded", func(t *testing.T) {
		// Stored so far for the organisation: five rows at 0.347 ms
		// (rec-bus, rec-nats, rec-unknown, rec-hit, rec-once) and
		// rec-legacy, which has no latency at all.
		counts, err := s.RequestCounts(ctx, testOrgID, since, now.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		var requests, samples int64
		var sum float64
		for _, c := range counts {
			requests += c.Requests
			samples += c.ScanSamples
			sum += c.ScanLatencySumMs
		}
		if requests != 6 || samples != 5 {
			t.Fatalf("%d requests, %d with a scan latency; want 6 and 5", requests, samples)
		}
		if avg := sum / float64(samples); avg < 0.3469 || avg > 0.3471 {
			t.Errorf("average scan latency = %v ms, want 0.347", avg)
		}
		buckets, err := s.RequestTimeseries(ctx, testOrgID, since, now.Add(time.Hour), "hour")
		if err != nil || len(buckets) != 1 {
			t.Fatalf("timeseries = %+v (%v)", buckets, err)
		}
		if b := buckets[0]; b.Total != 6 || b.ScanSamples != 5 || b.ScanP95Ms != 0.347 {
			t.Errorf("bucket = %+v, want p95 0.347 over 5 samples of 6 requests", b)
		}
		recs, _, err := s.ExportRequestRecords(ctx, testOrgID, EventSearchParams{Since: since, Until: now.Add(time.Hour)}, 0)
		if err != nil || len(recs) != 6 {
			t.Fatalf("export = %d rows (%v)", len(recs), err)
		}
		for _, r := range recs {
			if r.RequestID != "rec-legacy" && r.ScanLatencyMs != 0.347 {
				t.Errorf("export %s: scan latency %v", r.RequestID, r.ScanLatencyMs)
			}
		}
	})

	t.Run("a lookup by request id uses the index", func(t *testing.T) {
		// Enough rows of the organisation that walking them costs more
		// than a probe, in the monthly partition and in the default one.
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO request_logs (org_id, request_id, provider, action_taken, created_at)
			SELECT $1, 'bulk-' || g, 'openai', 'pass', ts
			FROM generate_series(1, 20000) g,
			     LATERAL (SELECT CASE WHEN g % 2 = 0 THEN $2::timestamptz ELSE $2::timestamptz - interval '200 days' END + g * interval '1 millisecond' AS ts) x`,
			testOrgID, now.Add(-2*time.Hour)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.Exec(ctx, "ANALYZE request_logs"); err != nil {
			t.Fatal(err)
		}
		explain := func(id string) string {
			rows, err := s.pool.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) SELECT request_id"+requestRecordByID,
				testOrgID, now.Add(-400*24*time.Hour), id)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var plan strings.Builder
			for rows.Next() {
				var line string
				_ = rows.Scan(&line)
				plan.WriteString(line + "\n")
			}
			return plan.String()
		}
		for _, id := range []string{"bulk-10", "bulk-11", "no-such-request"} {
			plan := explain(id)
			t.Logf("lookup of %s:\n%s", id, plan)
			if strings.Contains(plan, "Seq Scan") || strings.Contains(plan, "org_id_created_at_idx") {
				t.Errorf("lookup of %s walks the organisation's rows:\n%s", id, plan)
			}
			if !strings.Contains(plan, "org_id_request_id_created_at_idx") {
				t.Errorf("lookup of %s does not use the request-id index:\n%s", id, plan)
			}
		}
		// And it answers: a row in each partition, and a miss.
		for id, want := range map[string]bool{"bulk-10": true, "bulk-11": true, "no-such-request": false} {
			rec, err := s.GetRequestRecord(ctx, testOrgID, id, now.Add(-400*24*time.Hour))
			if err != nil || (rec != nil) != want {
				t.Errorf("GetRequestRecord(%s) = %v, %v", id, rec, err)
			}
		}
		if _, err := s.pool.Exec(ctx, `DELETE FROM request_logs WHERE request_id LIKE 'bulk-%'`); err != nil {
			t.Fatal(err)
		}
	})

	// From here the table is taken back to what it was before migration
	// 017, which is what a proxy upgraded before its database finds.
	t.Run("the proxy works on the table as it was before migration 017", func(t *testing.T) {
		execAll(t, s, migrationSQL(t, "017_request_record.down.sql"))
		if cols := recColumns(t, s); cols["policy_name"] || cols["policy_version"] || cols["output_risk"] {
			t.Fatalf("the down migration left its columns: %v", cols)
		}
		// The store still believes in the columns it saw. The first write
		// finds out and is not lost.
		e := full
		e.RequestID = "rec-premigration"
		row, _ := RequestLogFromEvent(e, testOrgID, nil)
		if err := s.insertBatch(ctx, []RequestLog{row}); err != nil {
			t.Fatalf("write on the older table: %v", err)
		}
		// Everything the older table has a column for is stored.
		if n := countRows(t, s, `request_id = 'rec-premigration' AND input_tokens = 1200 AND cost_usd = 0.00036
			AND cache_status = 'miss' AND output_action = 'redact' AND output_findings IS NOT NULL`); n != 1 {
			t.Errorf("the row on the older table: %s", rowText(t, s, "rec-premigration"))
		}
		// A store that starts on it (nothing known yet) reads and writes too.
		s.schema.Store(nil)
		rec, err := s.GetRequestRecord(ctx, testOrgID, "rec-premigration", since)
		if err != nil || rec == nil {
			t.Fatalf("read on the older table: %v, %v", rec, err)
		}
		if rec.InputTokens == nil || *rec.InputTokens != 1200 || rec.PolicyName != "" || rec.OutputRisk != nil {
			t.Errorf("record on the older table = %+v", rec)
		}
		if _, _, err := s.ExportRequestRecords(ctx, testOrgID, EventSearchParams{Since: since, Until: now.Add(time.Hour)}, 0); err != nil {
			t.Errorf("export on the older table: %v", err)
		}
		if err := s.insertBatch(ctx, []RequestLog{row}); err != nil || countRows(t, s, "request_id = 'rec-premigration'") != 1 {
			t.Errorf("a redelivered event on the older table (no request-id index): %v", err)
		}

		// At startup the proxy adds the columns itself; the index is left
		// to the migration.
		if err := s.EnsureRequestLogColumns(ctx); err != nil {
			t.Fatal(err)
		}
		if cols := recColumns(t, s); !cols["policy_name"] || !cols["policy_version"] || !cols["output_risk"] {
			t.Fatalf("EnsureRequestLogColumns did not add the columns: %v", cols)
		}
		if err := s.EnsureRequestLogColumns(ctx); err != nil {
			t.Errorf("EnsureRequestLogColumns a second time: %v", err)
		}
		e.RequestID = "rec-after-ensure"
		row, _ = RequestLogFromEvent(e, testOrgID, nil)
		if err := s.insertBatch(ctx, []RequestLog{row}); err != nil {
			t.Fatal(err)
		}
		rec, _ = s.GetRequestRecord(ctx, testOrgID, "rec-after-ensure", since)
		if rec == nil || rec.PolicyName != "bank-policy" || rec.OutputRisk == nil {
			t.Errorf("record after the columns were added = %+v", rec)
		}
		// The old row keeps saying nothing about its policy.
		rec, _ = s.GetRequestRecord(ctx, testOrgID, "rec-premigration", since)
		if rec == nil || rec.PolicyName != "" {
			t.Errorf("the row written before the columns existed = %+v", rec)
		}
		// The migration then finds the columns and builds the index.
		execAll(t, s, migrationSQL(t, "017_request_record.up.sql"))
		if n := countRows(t, s, "request_id IN ('rec-premigration', 'rec-after-ensure', 'rec-bus')"); n != 3 {
			t.Errorf("%d of 3 rows survived the migration", n)
		}
	})
}

// The row a NATS consumer would write for an event that is not a request's
// never reaches the table; the request itself does, once.
func TestTC_RequestRecord_OneRowPerForwardedRequest(t *testing.T) {
	s := NewTestPostgresStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute)
	req := fullRequestEvent()
	req.OrgID, req.RequestID, req.Timestamp = testOrgID, "rec-forwarded", now
	// What one forwarded request puts on the bus.
	onBus := []events.Event{
		{EventType: "output_scan_hint", RequestID: "rec-forwarded", Provider: "openai", ContentType: "application/json", Timestamp: now},
		{EventType: "output_scanned", RequestID: "rec-forwarded", OrgID: testOrgID, OutputAction: "REDACT", OutputFindings: req.OutputFindings, Timestamp: now},
		req,
	}
	var rows []RequestLog
	for _, e := range onBus {
		if rl, ok := RequestLogFromV2(overNATS(t, e), testOrgID, nil); ok {
			rows = append(rows, rl)
		}
	}
	if err := s.insertBatch(ctx, rows); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, s, "org_id = $1", testOrgID); n != 1 {
		t.Errorf("%d rows for one forwarded request, want 1", n)
	}
}
