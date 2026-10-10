package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/scanner"
)

// The queries in this file answer the console from request_logs, the record
// of what the proxy scanned. Every one of them names an organisation and a
// time window, so it runs on idx_request_logs_org_time and prunes the monthly
// partitions outside the window, and every one is bounded by a LIMIT.
//
// daily_stats is not read here: nothing writes it.

const (
	// historyGroupLimit bounds the rows an aggregate returns. An aggregate
	// that reaches it fails with ErrTooManyGroups: part of a total is not
	// handed back as the total.
	historyGroupLimit = 5000
	// riskCountCap is the count above which more findings of one type no
	// longer move a risk score (scanner.CalculateRisk adds for at most five
	// extra findings). Capping there keeps the distinct profiles few.
	riskCountCap = 6
	// timeseriesLimit covers 30 days of minute buckets.
	timeseriesLimit = 50000
	// ExportMaxRows is the most rows one export returns.
	ExportMaxRows = 50000
	exportTimeout = 60 * time.Second
)

// ErrTooManyGroups means an aggregate over request_logs had more groups than
// it may return, so its answer would have been partial.
var ErrTooManyGroups = errors.New("store: aggregate exceeds the group limit")

// RequestRecord is one request_logs row, with what the table holds about it.
// A value the row does not hold is nil or empty here, not zero: rows written
// by older versions lack most of what follows the latencies. The input risk
// is not stored; it is a function of the findings.
type RequestRecord struct {
	RequestID   string
	Provider    string
	Model       string
	ModelFamily string
	Endpoint    string
	ActionTaken string
	// InputTokens and OutputTokens are nil when the counts are not known.
	InputTokens   *int
	OutputTokens  *int
	Findings      json.RawMessage
	FindingsCount int
	// ScanLatencyMs and TotalLatencyMs are zero when they were not recorded.
	ScanLatencyMs  float64
	TotalLatencyMs float64
	CreatedAt      time.Time
	// CostUSD is nil when no cost is known.
	CostUSD     *float64
	CacheStatus string
	// OutputFindings and OutputRisk are nil when the response was not scanned.
	OutputFindings json.RawMessage
	OutputAction   string
	OutputRisk     json.RawMessage
	PolicyName     string
	PolicyVersion  string
}

// RequestCount is the number of requests for one provider and action, with
// the sum of their scan latencies.
type RequestCount struct {
	Provider         string
	Action           string // upper case
	Requests         int64
	ScanLatencySumMs float64
	// ScanSamples is the number of those requests that have a scan latency.
	ScanSamples int64
}

// RequestBucket is one bucket of a request timeseries.
type RequestBucket struct {
	Start       time.Time // UTC
	Total       int64
	Blocked     int64
	Redacted    int64
	Warned      int64
	ScanP95Ms   float64
	ScanSamples int64
}

// FindingCount is the number of findings with one type, category and severity.
type FindingCount struct {
	Type     string
	Category string
	Severity string // lower case
	Count    int64
}

// ModelCount is the number of requests one provider and model served.
type ModelCount struct {
	Provider    string
	Model       string
	ModelFamily string
	Requests    int64
}

// RiskProfile is a set of findings, reduced to what a risk score depends on
// (type and severity), and the number of requests that had it.
type RiskProfile struct {
	Findings []scanner.Finding
	Requests int64
}

func scanRequestRecord(row pgx.Row) (RequestRecord, error) {
	var rec RequestRecord
	var findings, outputFindings, outputRisk []byte
	err := row.Scan(&rec.RequestID, &rec.Provider, &rec.Model, &rec.ModelFamily,
		&rec.Endpoint, &rec.ActionTaken, &rec.InputTokens, &rec.OutputTokens,
		&findings, &rec.FindingsCount, &rec.ScanLatencyMs, &rec.TotalLatencyMs, &rec.CreatedAt,
		&rec.CostUSD, &rec.CacheStatus, &outputFindings, &rec.OutputAction, &outputRisk,
		&rec.PolicyName, &rec.PolicyVersion)
	if err != nil {
		return rec, err
	}
	rec.Findings = json.RawMessage(findingsOrEmpty(findings))
	if len(outputFindings) > 0 {
		rec.OutputFindings = json.RawMessage(outputFindings)
	}
	if len(outputRisk) > 0 {
		rec.OutputRisk = json.RawMessage(outputRisk)
	}
	return rec, nil
}

// requestRecordByID is the lookup of GetRequestRecord, after its column list.
// Equality on the organisation and the request id, a range and the order on
// the time: the shape of idx_request_logs_org_request.
const requestRecordByID = `
FROM request_logs
WHERE org_id = $1 AND created_at >= $2 AND request_id = $3
ORDER BY created_at DESC
LIMIT 1`

// GetRequestRecord returns the newest row with the given request id in the
// organisation, no older than since, or nil when there is none. The lookup is
// answered by idx_request_logs_org_request (migration 017), one probe for each
// monthly partition in the window; on a table without that index it walks the
// organisation's rows in the window.
func (s *PostgresStore) GetRequestRecord(ctx context.Context, orgID, requestID string, since time.Time) (*RequestRecord, error) {
	org, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("org id: %w", err)
	}
	if requestID == "" {
		return nil, nil
	}
	ctx2, cancel := context.WithTimeout(ctx, getStatsTimeout)
	defer cancel()
	rec, err := scanRequestRecord(s.pool.QueryRow(ctx2,
		"SELECT "+s.requestLogSchema(ctx2).selectSQL+requestRecordByID, org, since.UTC(), requestID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// RequestCounts counts the organisation's requests in [from, to] by provider
// and action. A scan latency of zero is one that was not recorded (older
// versions stored whole milliseconds, or nothing), so it is in neither the
// sum nor the samples.
func (s *PostgresStore) RequestCounts(ctx context.Context, orgID string, from, to time.Time) ([]RequestCount, error) {
	org, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("org id: %w", err)
	}
	ctx2, cancel := context.WithTimeout(ctx, getStatsTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx2, `
SELECT COALESCE(provider,''), upper(btrim(action_taken)), COUNT(*),
  COALESCE(SUM(scan_latency_ms::float8) FILTER (WHERE scan_latency_ms > 0), 0)::float8,
  COUNT(*) FILTER (WHERE scan_latency_ms > 0)
FROM request_logs
WHERE org_id = $1 AND created_at >= $2 AND created_at <= $3
GROUP BY 1, 2
ORDER BY 3 DESC
LIMIT $4`, org, from.UTC(), to.UTC(), historyGroupLimit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RequestCount
	for rows.Next() {
		var c RequestCount
		if err := rows.Scan(&c.Provider, &c.Action, &c.Requests, &c.ScanLatencySumMs, &c.ScanSamples); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > historyGroupLimit {
		return nil, ErrTooManyGroups
	}
	return out, nil
}

// RequestTimeseries counts the organisation's requests in [from, to] per
// bucket ("minute", "hour" or "day"), ascending. Buckets start on UTC
// boundaries; a bucket with no requests has no row. The p95 is over the
// requests that have a scan latency (see RequestCounts).
func (s *PostgresStore) RequestTimeseries(ctx context.Context, orgID string, from, to time.Time, bucket string) ([]RequestBucket, error) {
	org, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("org id: %w", err)
	}
	switch bucket {
	case "minute", "hour", "day":
	default:
		return nil, fmt.Errorf("bucket %q: want minute, hour or day", bucket)
	}
	ctx2, cancel := context.WithTimeout(ctx, getStatsTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx2, `
SELECT date_trunc($4::text, created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS bucket,
  COUNT(*),
  COUNT(*) FILTER (WHERE upper(btrim(action_taken)) = 'BLOCK'),
  COUNT(*) FILTER (WHERE upper(btrim(action_taken)) = 'REDACT'),
  COUNT(*) FILTER (WHERE upper(btrim(action_taken)) = 'WARN'),
  COALESCE(round((percentile_disc(0.95) WITHIN GROUP (ORDER BY scan_latency_ms)
    FILTER (WHERE scan_latency_ms > 0))::numeric, 3), 0)::float8,
  COUNT(*) FILTER (WHERE scan_latency_ms > 0)
FROM request_logs
WHERE org_id = $1 AND created_at >= $2 AND created_at <= $3
GROUP BY 1
ORDER BY 1 ASC
LIMIT $5`, org, from.UTC(), to.UTC(), bucket, timeseriesLimit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RequestBucket
	for rows.Next() {
		var b RequestBucket
		if err := rows.Scan(&b.Start, &b.Total, &b.Blocked, &b.Redacted, &b.Warned, &b.ScanP95Ms, &b.ScanSamples); err != nil {
			return nil, err
		}
		b.Start = b.Start.UTC()
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > timeseriesLimit {
		return nil, ErrTooManyGroups
	}
	return out, nil
}

// FindingCounts counts the findings of the organisation's requests in
// [from, to] by type, category and severity.
func (s *PostgresStore) FindingCounts(ctx context.Context, orgID string, from, to time.Time) ([]FindingCount, error) {
	org, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("org id: %w", err)
	}
	ctx2, cancel := context.WithTimeout(ctx, getStatsTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx2, `
SELECT COALESCE(f->>'type',''), COALESCE(f->>'category',''), lower(COALESCE(f->>'severity','')), COUNT(*)
FROM request_logs r
CROSS JOIN LATERAL jsonb_array_elements(r.findings) AS f
WHERE r.org_id = $1 AND r.created_at >= $2 AND r.created_at <= $3
  AND r.findings_count > 0 AND jsonb_typeof(r.findings) = 'array'
GROUP BY 1, 2, 3
ORDER BY 4 DESC
LIMIT $4`, org, from.UTC(), to.UTC(), historyGroupLimit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FindingCount
	for rows.Next() {
		var c FindingCount
		if err := rows.Scan(&c.Type, &c.Category, &c.Severity, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > historyGroupLimit {
		return nil, ErrTooManyGroups
	}
	return out, nil
}

// ModelCounts counts the organisation's requests in [from, to] by provider,
// model and model family.
func (s *PostgresStore) ModelCounts(ctx context.Context, orgID string, from, to time.Time) ([]ModelCount, error) {
	org, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("org id: %w", err)
	}
	ctx2, cancel := context.WithTimeout(ctx, getStatsTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx2, `
SELECT COALESCE(provider,''), COALESCE(model,''), COALESCE(model_family,''), COUNT(*)
FROM request_logs
WHERE org_id = $1 AND created_at >= $2 AND created_at <= $3
GROUP BY 1, 2, 3
ORDER BY 4 DESC
LIMIT $4`, org, from.UTC(), to.UTC(), historyGroupLimit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelCount
	for rows.Next() {
		var c ModelCount
		if err := rows.Scan(&c.Provider, &c.Model, &c.ModelFamily, &c.Requests); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > historyGroupLimit {
		return nil, ErrTooManyGroups
	}
	return out, nil
}

// RiskProfiles groups the organisation's requests in [from, to] that had
// findings by what their risk score depends on: the type and severity of
// each finding and how many there were, counted up to riskCountCap. The
// score itself stays in scanner.CalculateRisk; a request with no findings
// has no profile and a risk of zero.
func (s *PostgresStore) RiskProfiles(ctx context.Context, orgID string, from, to time.Time) ([]RiskProfile, error) {
	org, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("org id: %w", err)
	}
	ctx2, cancel := context.WithTimeout(ctx, getStatsTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx2, `
SELECT p.profile::text, COUNT(*)
FROM (
  SELECT (
    SELECT jsonb_agg(jsonb_build_object('type', g.t, 'severity', g.sev, 'n', g.n) ORDER BY g.t, g.sev)
    FROM (
      SELECT COALESCE(f->>'type','') AS t, lower(btrim(COALESCE(f->>'severity',''))) AS sev,
        LEAST(COUNT(*), $5::bigint) AS n
      FROM jsonb_array_elements(r.findings) AS f
      GROUP BY 1, 2
    ) g
  ) AS profile
  FROM request_logs r
  WHERE r.org_id = $1 AND r.created_at >= $2 AND r.created_at <= $3
    AND r.findings_count > 0 AND jsonb_typeof(r.findings) = 'array'
) p
WHERE p.profile IS NOT NULL
GROUP BY p.profile
ORDER BY 2 DESC
LIMIT $4`, org, from.UTC(), to.UTC(), historyGroupLimit+1, riskCountCap)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RiskProfile
	for rows.Next() {
		var raw string
		var p RiskProfile
		if err := rows.Scan(&raw, &p.Requests); err != nil {
			return nil, err
		}
		var parts []struct {
			Type     string `json:"type"`
			Severity string `json:"severity"`
			N        int    `json:"n"`
		}
		if err := json.Unmarshal([]byte(raw), &parts); err != nil {
			return nil, fmt.Errorf("risk profile: %w", err)
		}
		for _, part := range parts {
			for i := 0; i < part.N; i++ {
				p.Findings = append(p.Findings, scanner.Finding{Type: part.Type, Severity: part.Severity})
			}
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > historyGroupLimit {
		return nil, ErrTooManyGroups
	}
	return out, nil
}

// ExportRequestRecords returns the organisation's rows that match p, newest
// first, up to max (at most ExportMaxRows), and whether more rows matched
// than were returned. Page and Limit in p are ignored.
func (s *PostgresStore) ExportRequestRecords(ctx context.Context, orgID string, p EventSearchParams, max int) ([]RequestRecord, bool, error) {
	org, err := uuid.Parse(orgID)
	if err != nil {
		return nil, false, fmt.Errorf("org id: %w", err)
	}
	if max < 1 || max > ExportMaxRows {
		max = ExportMaxRows
	}
	where, args := eventSearchWhere(org, p)
	ctx2, cancel := context.WithTimeout(ctx, exportTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx2, fmt.Sprintf(`
SELECT `+s.requestLogSchema(ctx2).selectSQL+`
FROM request_logs
%s
ORDER BY created_at DESC
LIMIT $%d`, where, len(args)+1), append(args, max+1)...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []RequestRecord
	for rows.Next() {
		rec, err := scanRequestRecord(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(out) > max {
		return out[:max], true, nil
	}
	return out, false, nil
}

// eventSearchWhere builds the WHERE clause and its arguments for a search of
// one organisation's request_logs. The window defaults to the last 7 days.
func eventSearchWhere(org uuid.UUID, p EventSearchParams) (string, []interface{}) {
	until := p.Until
	if until.IsZero() {
		until = time.Now().UTC()
	}
	since := p.Since
	if since.IsZero() {
		since = until.AddDate(0, 0, -7)
	}

	args := []interface{}{org, since, until}
	var where strings.Builder
	where.WriteString(`WHERE org_id = $1 AND created_at >= $2 AND created_at <= $3`)
	next := func() int {
		return len(args) + 1
	}

	if wanted := events.ActionSet(p.Action); len(wanted) > 0 {
		i := next()
		fmt.Fprintf(&where, " AND upper(action_taken) = ANY($%d)", i)
		args = append(args, wanted)
	}
	prov := strings.ToLower(strings.TrimSpace(p.Provider))
	if p.ShadowOnly || prov == "shadow" {
		where.WriteString(` AND lower(provider) NOT IN ('openai','anthropic','google','azure','azure_openai','google_vertex')`)
	} else if prov != "" && prov != "all" {
		i := next()
		fmt.Fprintf(&where, " AND lower(provider) = $%d", i)
		args = append(args, prov)
	}
	if ft := strings.TrimSpace(p.FindingType); ft != "" {
		i := next()
		fmt.Fprintf(&where, ` AND EXISTS (SELECT 1 FROM jsonb_array_elements(findings) elem WHERE lower(elem->>'type') = lower($%d))`, i)
		args = append(args, ft)
	}
	if sev := strings.TrimSpace(p.Severity); sev != "" {
		i := next()
		fmt.Fprintf(&where, ` AND EXISTS (SELECT 1 FROM jsonb_array_elements(findings) elem WHERE lower(elem->>'severity') = lower($%d))`, i)
		args = append(args, sev)
	}
	if cat := strings.TrimSpace(p.Category); cat != "" {
		i := next()
		fmt.Fprintf(&where, ` AND EXISTS (SELECT 1 FROM jsonb_array_elements(findings) elem WHERE elem->>'category' ILIKE $%d)`, i)
		args = append(args, "%"+cat+"%")
	}
	if tech := strings.TrimSpace(p.Technique); tech != "" {
		i := next()
		fmt.Fprintf(&where, " AND findings::text ILIKE $%d", i)
		args = append(args, "%"+tech+"%")
	}
	if q := strings.TrimSpace(p.Q); q != "" {
		pat := "%" + q + "%"
		i := next()
		fmt.Fprintf(&where, " AND (request_id ILIKE $%d OR findings::text ILIKE $%d)", i, i)
		args = append(args, pat)
	}
	if rid := strings.TrimSpace(p.RequestID); rid != "" {
		// position() and not ILIKE: the text is a literal, not a pattern.
		i := next()
		fmt.Fprintf(&where, " AND position(lower($%d::text) in lower(request_id)) > 0", i)
		args = append(args, rid)
	}
	return where.String(), args
}
