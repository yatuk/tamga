package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// The binary is upgraded before the migrations are run, so the request_logs
// table it finds may be older than the one it was written for. Two things
// keep it working:
//
//   - EnsureRequestLogColumns, called at startup, adds the columns of the
//     newest migration, the way the other stores bring their tables in line
//     (incidents.EnsureTable, docstore.Init).
//   - If that could not be done (no right to alter the table, the lock was
//     not free in time), the writer and the readers look at the columns the
//     table has and leave the others out. The row is still written; it is
//     poorer until the migration is applied, which is noticed within
//     schemaRecheck without a restart.

// requestLogOptionalColumns are the columns later migrations added to
// request_logs (003, 006, 017), with the type a value is written as.
var requestLogOptionalColumns = []struct{ name, sqlType string }{
	{"model_family", "text"},
	{"output_findings", "jsonb"},
	{"output_action", "text"},
	{"cost_usd", "numeric"},
	{"cache_status", "text"},
	{"policy_name", "text"},
	{"policy_version", "text"},
	{"output_risk", "jsonb"},
}

// schemaRecheck is how often a table found incomplete is looked at again.
const schemaRecheck = time.Minute

// requestLogSchema is which optional columns request_logs has, and the
// statements that follow from it.
type requestLogSchema struct {
	has       map[string]bool
	missing   []string
	checkedAt time.Time
	// optional lists the optional columns the table has, as indexes into
	// requestLogOptionalColumns, in the order insertSQL names them.
	optional  []int
	insertSQL string
	selectSQL string
}

// requestLogBaseArgs is the number of arguments of the insert before the
// optional columns: the columns of the first migration.
const requestLogBaseArgs = 14

func newRequestLogSchema(has map[string]bool, at time.Time) *requestLogSchema {
	sc := &requestLogSchema{has: has, checkedAt: at}
	cols := []string{"org_id", "request_id", "provider", "model", "endpoint", "input_tokens", "output_tokens",
		"findings", "findings_count", "action_taken", "scan_latency_ms", "total_latency_ms", "user_identifier", "created_at"}
	vals := []string{"$1::uuid", "$2::text", "$3::text", "$4::text", "$5::text", "$6::int", "$7::int",
		"$8::jsonb", "$9::int", "$10::text", "$11::real", "$12::real", "$13::text", "COALESCE($14::timestamptz, now())"}
	for i, c := range requestLogOptionalColumns {
		if !has[c.name] {
			sc.missing = append(sc.missing, c.name)
			continue
		}
		cols = append(cols, c.name)
		sc.optional = append(sc.optional, i)
		vals = append(vals, fmt.Sprintf("$%d::%s", requestLogBaseArgs+len(sc.optional), c.sqlType))
	}
	// One row per event: an event is named by its organisation, request id
	// and time, and one that is delivered again (NATS redelivers what was
	// not acknowledged in time) finds its row and adds nothing. The time is
	// compared exactly, so the check reads one partition and is answered by
	// idx_request_logs_org_request, or by idx_request_logs_org_time on a
	// table that does not have it yet. A row with no time of its own is
	// dated now and cannot be recognised.
	sc.insertSQL = "INSERT INTO request_logs (" + strings.Join(cols, ", ") + ")\nSELECT " + strings.Join(vals, ", ") + `
WHERE $14::timestamptz IS NULL OR NOT EXISTS (
  SELECT 1 FROM request_logs
  WHERE org_id = $1::uuid AND request_id = $2::text AND created_at = $14::timestamptz)`

	opt := func(name, expr, absent string) string {
		if has[name] {
			return expr
		}
		return absent
	}
	// Tokens are known when either count is above zero: rows written before
	// the counts were recorded hold 0 and 0, newer rows hold NULL.
	sc.selectSQL = `request_id, COALESCE(provider,''), COALESCE(model,''), ` + opt("model_family", "COALESCE(model_family,'')", "''") + `,
  COALESCE(endpoint,''), action_taken,
  CASE WHEN COALESCE(input_tokens,0) + COALESCE(output_tokens,0) > 0 THEN COALESCE(input_tokens,0) END,
  CASE WHEN COALESCE(input_tokens,0) + COALESCE(output_tokens,0) > 0 THEN COALESCE(output_tokens,0) END,
  findings, findings_count,
  round(COALESCE(scan_latency_ms,0)::numeric, 3)::float8, round(COALESCE(total_latency_ms,0)::numeric, 3)::float8,
  created_at,
  ` + opt("cost_usd", "cost_usd::float8", "NULL::float8") + `,
  ` + opt("cache_status", "COALESCE(cache_status,'')", "''") + `,
  ` + opt("output_findings", "output_findings", "NULL::jsonb") + `,
  ` + opt("output_action", "COALESCE(output_action,'')", "''") + `,
  ` + opt("output_risk", "output_risk", "NULL::jsonb") + `,
  ` + opt("policy_name", "COALESCE(policy_name,'')", "''") + `,
  ` + opt("policy_version", "COALESCE(policy_version,'')", "''")
	return sc
}

// insertArgs are the arguments of insertSQL for one row.
func (sc *requestLogSchema) insertArgs(org interface{}, r RequestLog) []interface{} {
	var inTok, outTok interface{}
	if r.InputTokens > 0 || r.OutputTokens > 0 {
		inTok, outTok = r.InputTokens, r.OutputTokens
	}
	var cost interface{}
	if r.CostUSD != nil {
		cost = *r.CostUSD
	}
	args := []interface{}{
		org, r.RequestID, r.Provider, nullIfEmpty(r.Model), nullIfEmpty(r.Endpoint), inTok, outTok,
		findingsOrEmpty(r.Findings), r.FindingsCount, r.ActionTaken, r.ScanLatencyMs, r.TotalLatencyMs,
		nullIfEmpty(r.UserID), happenedAt(r.At),
	}
	// In the order of requestLogOptionalColumns.
	optional := [...]interface{}{
		nullIfEmpty(r.ModelFamily), jsonOrNull(r.OutputFindings), nullIfEmpty(r.OutputAction), cost,
		nullIfEmpty(r.CacheStatus), nullIfEmpty(r.PolicyName), nullIfEmpty(r.PolicyVersion), jsonOrNull(r.OutputRisk),
	}
	for _, i := range sc.optional {
		args = append(args, optional[i])
	}
	return args
}

func jsonOrNull(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}

// requestLogSchema returns what is known about the table's columns, looking
// when nothing is known yet, or when the table was found incomplete and
// schemaRecheck has passed. When the table cannot be looked at it returns the
// last answer, or, without one, the columns of the first migration only.
func (s *PostgresStore) requestLogSchema(ctx context.Context) *requestLogSchema {
	cur := s.schema.Load()
	if cur != nil && (len(cur.missing) == 0 || time.Since(cur.checkedAt) < schemaRecheck) {
		return cur
	}
	rows, err := s.pool.Query(ctx, `
SELECT attname::text FROM pg_attribute
WHERE attrelid = to_regclass('request_logs') AND attnum > 0 AND NOT attisdropped`)
	if err == nil {
		has := map[string]bool{}
		for rows.Next() {
			var name string
			if err = rows.Scan(&name); err != nil {
				break
			}
			has[name] = true
		}
		rows.Close()
		if err == nil {
			err = rows.Err()
		}
		if err == nil && len(has) > 0 {
			next := newRequestLogSchema(has, time.Now())
			if len(next.missing) > 0 && (cur == nil || strings.Join(cur.missing, ",") != strings.Join(next.missing, ",")) {
				s.log.Warn().Strs("missing_columns", next.missing).
					Msg("request_logs is older than this version: these columns are not stored until the migrations in deploy/migrations are applied")
			}
			s.schema.Store(next)
			return next
		}
	}
	if cur != nil {
		return cur
	}
	return newRequestLogSchema(map[string]bool{}, time.Now())
}

// ensureLockTimeout bounds the wait for the table lock at startup, so a proxy
// that starts behind a long query does not hold up the writers queued behind it.
const ensureLockTimeout = "5s"

// EnsureRequestLogColumns adds the columns of migration 017 to request_logs
// when they are not there. Adding a nullable column with no default changes
// the catalog only: the table is not rewritten, and the ACCESS EXCLUSIVE lock
// on it and its partitions is held for the moment the change takes. The
// index of that migration is not built here: that reads every row.
//
// On error the table is as it was, and the store works with the columns it has.
func (s *PostgresStore) EnsureRequestLogColumns(ctx context.Context) error {
	has := s.requestLogSchema(ctx).has
	if has["policy_name"] && has["policy_version"] && has["output_risk"] {
		return nil
	}
	ctx2, cancel := context.WithTimeout(ctx, insertTimeout)
	defer cancel()
	tx, err := s.pool.Begin(ctx2)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx2) }()
	if _, err := tx.Exec(ctx2, "SET LOCAL lock_timeout = '"+ensureLockTimeout+"'"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx2, `
ALTER TABLE request_logs
    ADD COLUMN IF NOT EXISTS policy_name TEXT,
    ADD COLUMN IF NOT EXISTS policy_version TEXT,
    ADD COLUMN IF NOT EXISTS output_risk JSONB`); err != nil {
		return err
	}
	if err := tx.Commit(ctx2); err != nil {
		return err
	}
	s.schema.Store(nil)
	return nil
}

// undefinedColumn reports whether err is PostgreSQL saying a statement named
// a column the table does not have (the table changed under a cached answer).
func undefinedColumn(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42703"
}

// Requests that were scanned and could not be stored. A request must not
// leave the record without a trace: each is counted here by reason (exported
// as tamga_request_log_unrecorded_total) and logged, at most once per
// unrecordedLogEvery for each reason.
const (
	// UnrecordedOrgNotUUID: the request's organisation is not a UUID, and
	// request_logs.org_id holds nothing else.
	UnrecordedOrgNotUUID = "org_not_uuid"
	// UnrecordedInsertFailed: the batch the request was in was refused by
	// the database.
	UnrecordedInsertFailed = "insert_failed"

	unrecordedLogEvery = time.Minute
)

var unrecorded = struct {
	mu       sync.Mutex
	counts   map[string]*atomic.Int64
	loggedAt map[string]time.Time
}{counts: map[string]*atomic.Int64{}, loggedAt: map[string]time.Time{}}

// countUnrecorded adds n requests to a reason's count and reports whether
// the reason is due a log line.
func countUnrecorded(reason string, n int) (total int64, logNow bool) {
	unrecorded.mu.Lock()
	defer unrecorded.mu.Unlock()
	c := unrecorded.counts[reason]
	if c == nil {
		c = &atomic.Int64{}
		unrecorded.counts[reason] = c
	}
	total = c.Add(int64(n))
	now := time.Now()
	if last, ok := unrecorded.loggedAt[reason]; !ok || now.Sub(last) >= unrecordedLogEvery {
		unrecorded.loggedAt[reason] = now
		logNow = true
	}
	return total, logNow
}

// UnrecordedRequest is how many scanned requests were not stored, for one reason.
type UnrecordedRequest struct {
	Reason string
	Count  int64
}

// UnrecordedRequests returns the counts of scanned requests that were not
// stored since the process started, by reason, in a stable order. Both
// reasons are always listed, so a metric exists before its first increment.
func UnrecordedRequests() []UnrecordedRequest {
	unrecorded.mu.Lock()
	defer unrecorded.mu.Unlock()
	out := []UnrecordedRequest{{Reason: UnrecordedInsertFailed}, {Reason: UnrecordedOrgNotUUID}}
	for reason, c := range unrecorded.counts {
		found := false
		for i := range out {
			if out[i].Reason == reason {
				out[i].Count = c.Load()
				found = true
			}
		}
		if !found {
			out = append(out, UnrecordedRequest{Reason: reason, Count: c.Load()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Reason < out[j].Reason })
	return out
}
