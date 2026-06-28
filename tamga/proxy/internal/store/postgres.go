package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

const (
	batchMaxRows    = 100
	flushInterval   = 5 * time.Second
	insertTimeout   = 30 * time.Second
	getStatsTimeout = 15 * time.Second
)

// PostgresStore batches request_logs inserts for throughput.
type PostgresStore struct {
	pool   *pgxpool.Pool
	log    zerolog.Logger
	mu     sync.Mutex
	buf    []RequestLog
	done   chan struct{}
	wg     sync.WaitGroup
	closed bool
}

// NewPostgresStore opens a pool and starts periodic flushing.
func NewPostgresStore(ctx context.Context, dsn string, log zerolog.Logger) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}
	s := &PostgresStore{
		pool: pool,
		log:  log.With().Str("component", "postgres_store").Logger(),
		done: make(chan struct{}),
	}
	s.wg.Add(1)
	go s.flushLoop()
	return s, nil
}

func (s *PostgresStore) flushLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			s.flush(context.Background())
			return
		case <-ticker.C:
			s.flush(context.Background())
		}
	}
}

func (s *PostgresStore) flush(ctx context.Context) {
	s.mu.Lock()
	batch := s.buf
	s.buf = nil
	s.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	ctx2, cancel := context.WithTimeout(ctx, insertTimeout)
	defer cancel()
	if err := s.insertBatch(ctx2, batch); err != nil {
		s.log.Warn().Err(err).Str("component", "store").Int("rows", len(batch)).Msg("request_logs batch insert failed")
	}
}

func (s *PostgresStore) insertBatch(ctx context.Context, rows []RequestLog) error {
	if len(rows) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	var n int
	for _, r := range rows {
		org, err := uuid.Parse(r.OrgID)
		if err != nil {
			continue
		}
		batch.Queue(`
INSERT INTO request_logs (
  org_id, request_id, provider, model, model_family, endpoint,
  input_tokens, output_tokens, findings, findings_count,
  action_taken, scan_latency_ms, total_latency_ms, user_identifier
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
			org, r.RequestID, r.Provider, nullIfEmpty(r.Model), nullIfEmpty(r.ModelFamily), nullIfEmpty(r.Endpoint),
			r.InputTokens, r.OutputTokens, r.Findings, r.FindingsCount,
			r.ActionTaken, r.ScanLatencyMs, r.TotalLatencyMs, nullIfEmpty(r.UserID),
		)
		n++
	}
	if n == 0 {
		return nil
	}
	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	for i := 0; i < n; i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("batch exec %d: %w", i, err)
		}
	}
	return nil
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// SaveRequestLog enqueues a row; may flush when buffer reaches batchMaxRows.
func (s *PostgresStore) SaveRequestLog(ctx context.Context, logRow RequestLog) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("store closed")
	}
	s.buf = append(s.buf, logRow)
	n := len(s.buf)
	var flush []RequestLog
	if n >= batchMaxRows {
		flush = s.buf
		s.buf = nil
	}
	s.mu.Unlock()
	if len(flush) > 0 {
		ctx2, cancel := context.WithTimeout(ctx, insertTimeout)
		defer cancel()
		return s.insertBatch(ctx2, flush)
	}
	return nil
}

// GetStats aggregates daily_stats for an org in [from, to] (inclusive, date portion).
func (s *PostgresStore) GetStats(ctx context.Context, orgID string, from, to time.Time) (*Stats, error) {
	org, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("org id: %w", err)
	}
	ctx2, cancel := context.WithTimeout(ctx, getStatsTimeout)
	defer cancel()
	var st Stats
	err = s.pool.QueryRow(ctx2, `
SELECT
  COALESCE(SUM(total_requests), 0),
  COALESCE(SUM(blocked_requests), 0),
  COALESCE(SUM(redacted_requests), 0),
  COALESCE(SUM(warned_requests), 0),
  COALESCE(SUM(total_input_tokens), 0),
  COALESCE(SUM(total_output_tokens), 0)
FROM daily_stats
WHERE org_id = $1 AND stat_date >= $2::date AND stat_date <= $3::date`,
		org, from.UTC(), to.UTC(),
	).Scan(
		&st.TotalRequests,
		&st.BlockedRequests,
		&st.RedactedRequests,
		&st.WarnedRequests,
		&st.TotalInputTokens,
		&st.TotalOutputTokens,
	)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// Ping checks database connectivity.
func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Pool exposes the underlying pgx pool so sibling stores (audit,
// policy history) can share a single connection pool without opening a
// second one against the same database.
func (s *PostgresStore) Pool() *pgxpool.Pool {
	return s.pool
}

// GetModelTokenUsage aggregates input/output tokens per provider+model from
// request_logs for the given organisation and time window. Used by the
// billing costs/breakdown endpoint.
func (s *PostgresStore) GetModelTokenUsage(ctx context.Context, orgID string, from, to time.Time) ([]ModelTokenUsage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			COALESCE(provider, '')   AS provider,
			COALESCE(model, '')       AS model,
			COALESCE(model_family, '') AS model_family,
			COALESCE(SUM(input_tokens), 0)  AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens
		FROM request_logs
		WHERE org_id = $1
		  AND created_at >= $2
		  AND created_at < $3
		  AND (input_tokens > 0 OR output_tokens > 0)
		GROUP BY provider, model, model_family
		ORDER BY SUM(input_tokens + output_tokens) DESC
	`, orgID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ModelTokenUsage
	for rows.Next() {
		var r ModelTokenUsage
		if err := rows.Scan(&r.Provider, &r.Model, &r.ModelFamily, &r.InputTokens, &r.OutputTokens); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetDailyTokenUsage aggregates input/output tokens per date+provider+model
// from request_logs for the given organisation and time window. Used by the
// billing costs/breakdown endpoint for daily breakdown and MTD projections.
func (s *PostgresStore) GetDailyTokenUsage(ctx context.Context, orgID string, from, to time.Time) ([]DailyTokenUsage, error) {
	rows, err := s.pool.Query(ctx, `
			SELECT
				created_at::date              AS date,
				COALESCE(provider, '')         AS provider,
				COALESCE(model, '')            AS model,
				COALESCE(model_family, '')     AS model_family,
				COALESCE(SUM(input_tokens), 0)  AS input_tokens,
				COALESCE(SUM(output_tokens), 0) AS output_tokens
			FROM request_logs
			WHERE org_id = $1
			  AND created_at >= $2
			  AND created_at < $3
			  AND (input_tokens > 0 OR output_tokens > 0)
			GROUP BY created_at::date, provider, model, model_family
			ORDER BY created_at::date DESC, SUM(input_tokens + output_tokens) DESC
		`, orgID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DailyTokenUsage
	for rows.Next() {
		var r DailyTokenUsage
		if err := rows.Scan(&r.Date, &r.Provider, &r.Model, &r.ModelFamily, &r.InputTokens, &r.OutputTokens); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ListSecurityEvents returns paginated request_logs for the last 7 days.
func (s *PostgresStore) ListSecurityEvents(ctx context.Context, orgID string, page, limit int) ([]SecurityEvent, int, error) {
	until := time.Now().UTC()
	return s.SearchSecurityEvents(ctx, orgID, EventSearchParams{
		Page:  page,
		Limit: limit,
		Since: until.AddDate(0, 0, -7),
		Until: until,
	})
}

// SearchSecurityEvents returns filtered, paginated request_logs.
func (s *PostgresStore) SearchSecurityEvents(ctx context.Context, orgID string, p EventSearchParams) ([]SecurityEvent, int, error) {
	org, err := uuid.Parse(orgID)
	if err != nil {
		return nil, 0, fmt.Errorf("org id: %w", err)
	}
	page := p.Page
	limit := p.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := (page - 1) * limit

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

	if a := strings.TrimSpace(p.Action); a != "" {
		i := next()
		fmt.Fprintf(&where, " AND upper(action_taken) = upper($%d)", i)
		args = append(args, a)
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

	ctx2, cancel := context.WithTimeout(ctx, getStatsTimeout)
	defer cancel()

	w := where.String()
	countSQL := "SELECT COUNT(*) FROM request_logs " + w
	var total int
	if err := s.pool.QueryRow(ctx2, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limPos := len(args) + 1
	offPos := len(args) + 2
	args2 := append(append([]interface{}{}, args...), limit, offset)
	dataSQL := fmt.Sprintf(`
SELECT request_id, COALESCE(provider,''), COALESCE(model,''), action_taken, findings, findings_count, created_at
FROM request_logs
%s
ORDER BY created_at DESC
LIMIT $%d OFFSET $%d`, w, limPos, offPos)

	rows, err := s.pool.Query(ctx2, dataSQL, args2...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []SecurityEvent
	for rows.Next() {
		var ev SecurityEvent
		var prov, mod string
		var findings []byte
		if err := rows.Scan(&ev.RequestID, &prov, &mod, &ev.ActionTaken, &findings, &ev.FindingsCount, &ev.CreatedAt); err != nil {
			return nil, 0, err
		}
		ev.Provider = prov
		ev.Model = mod
		if len(findings) > 0 {
			ev.Findings = json.RawMessage(findings)
		} else {
			ev.Findings = json.RawMessage([]byte("[]"))
		}
		out = append(out, ev)
	}
	return out, total, rows.Err()
}

// EraseSubject deletes (or soft-redacts) every request_logs row that
// references the given data subject. Use user_id for Clerk IDs, email for
// legacy exports, and tckn_hash for SHA-256'ed TCKN lookups.
func (s *PostgresStore) EraseSubject(ctx context.Context, orgID, userID, email, tcknHash string) (int, error) {
	ctx2, cancel := context.WithTimeout(ctx, getStatsTimeout)
	defer cancel()
	var cmd pgconn.CommandTag
	var err error
	switch {
	case userID != "":
		cmd, err = s.pool.Exec(ctx2,
			`DELETE FROM request_logs WHERE org_id = $1 AND user_identifier = $2`, orgID, userID)
	case email != "":
		cmd, err = s.pool.Exec(ctx2,
			`DELETE FROM request_logs WHERE org_id = $1 AND user_identifier = $2`, orgID, email)
	case tcknHash != "":
		// TCKN hashes are stored inside the findings JSON; match any occurrence.
		cmd, err = s.pool.Exec(ctx2,
			`DELETE FROM request_logs WHERE org_id = $1 AND findings::text LIKE '%' || $2 || '%'`,
			orgID, tcknHash)
	default:
		return 0, fmt.Errorf("no subject identifier provided")
	}
	if err != nil {
		return 0, err
	}
	return int(cmd.RowsAffected()), nil
}

// SubjectAccess returns request_logs rows for a data subject (GDPR Art. 15 / KVKK madde 11).
// Returns up to limit rows; pass 0 for default (500).
func (s *PostgresStore) SubjectAccess(ctx context.Context, orgID, userID, email, tcknHash string, limit int) ([]RequestLogRow, error) {
	if limit <= 0 {
		limit = 500
	}
	ctx2, cancel := context.WithTimeout(ctx, getStatsTimeout)
	defer cancel()
	var rows pgx.Rows
	var err error
	switch {
	case userID != "":
		rows, err = s.pool.Query(ctx2,
			`SELECT request_id, org_id, provider, model, user_identifier, created_at, findings, input_tokens, output_tokens, COALESCE(cost_usd,0)
			 FROM request_logs WHERE org_id = $1 AND user_identifier = $2
			 ORDER BY created_at DESC LIMIT $3`, orgID, userID, limit)
	case email != "":
		rows, err = s.pool.Query(ctx2,
			`SELECT request_id, org_id, provider, model, user_identifier, created_at, findings, input_tokens, output_tokens, COALESCE(cost_usd,0)
			 FROM request_logs WHERE org_id = $1 AND user_identifier = $2
			 ORDER BY created_at DESC LIMIT $3`, orgID, email, limit)
	case tcknHash != "":
		rows, err = s.pool.Query(ctx2,
			`SELECT request_id, org_id, provider, model, user_identifier, created_at, findings, input_tokens, output_tokens, COALESCE(cost_usd,0)
			 FROM request_logs WHERE org_id = $1 AND findings::text LIKE '%' || $2 || '%'
			 ORDER BY created_at DESC LIMIT $3`, orgID, tcknHash, limit)
	default:
		return nil, fmt.Errorf("no subject identifier provided")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RequestLogRow
	for rows.Next() {
		var r RequestLogRow
		var findings []byte
		if err := rows.Scan(&r.RequestID, &r.OrgID, &r.Provider, &r.Model, &r.UserID, &r.CreatedAt, &findings, &r.TokensIn, &r.TokensOut, &r.CostUSD); err != nil {
			return out, err
		}
		r.Findings = findings
		out = append(out, r)
	}
	return out, rows.Err()
}

// RequestLogRow is a single row returned by SubjectAccess.
type RequestLogRow struct {
	RequestID string
	OrgID     string
	Provider  string
	Model     string
	UserID    string
	CreatedAt time.Time
	Findings  json.RawMessage
	TokensIn  int64
	TokensOut int64
	CostUSD   float64
}

// ApplyRetention deletes rows older than the provided window (KVKK madde 7).
func (s *PostgresStore) ApplyRetention(ctx context.Context, orgID string, window time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-window)
	ctx2, cancel := context.WithTimeout(ctx, getStatsTimeout)
	defer cancel()
	cmd, err := s.pool.Exec(ctx2,
		`DELETE FROM request_logs WHERE org_id = $1 AND created_at < $2`,
		orgID, cutoff)
	if err != nil {
		return 0, err
	}
	return int(cmd.RowsAffected()), nil
}

// Close stops the flush ticker, drains the buffer, and closes the pool.
func (s *PostgresStore) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	close(s.done)
	s.wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), insertTimeout)
	defer cancel()
	s.mu.Lock()
	last := s.buf
	s.buf = nil
	s.mu.Unlock()
	if len(last) > 0 {
		if err := s.insertBatch(ctx, last); err != nil {
			s.log.Warn().Err(err).Str("component", "store").Int("rows", len(last)).Msg("final request_logs flush failed")
		}
	}
	s.pool.Close()
	return nil
}
