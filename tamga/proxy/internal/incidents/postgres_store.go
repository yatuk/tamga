package incidents

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgxPool is a subset of *pgxpool.Pool used by PostgresStore to allow
// unit testing with mocks.
type pgxPool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PostgresStore persists incident lifecycle state in the incident_lifecycle
// table. It implements both Store and LifecycleStore.
type PostgresStore struct {
	pool pgxPool
}

// NewPostgresStore creates a Postgres-backed incident store using the given pool.
// Pass nil to create a store that short-circuits (useful for testing).
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	ps := &PostgresStore{}
	if pool != nil {
		ps.pool = pool
	}
	return ps
}

// ensureTable creates the incident_lifecycle table if it does not exist.
func (s *PostgresStore) ensureTable(ctx context.Context) error {
	if s.pool == nil {
		return fmt.Errorf("postgres pool not available")
	}
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx2, `
	CREATE TABLE IF NOT EXISTS incident_lifecycle (
		request_id      TEXT PRIMARY KEY,
		org_id          TEXT NOT NULL DEFAULT '',
		status          TEXT NOT NULL DEFAULT 'Open',
		assignee        TEXT NOT NULL DEFAULT '',
		reason          TEXT NOT NULL DEFAULT '',
		tags            TEXT[] NOT NULL DEFAULT '{}',
		comments        JSONB NOT NULL DEFAULT '[]'::jsonb,
		triaged_at      TIMESTAMPTZ,
		triaged_by      TEXT NOT NULL DEFAULT '',
		resolved_at     TIMESTAMPTZ,
		resolved_by     TEXT NOT NULL DEFAULT '',
		resolution      TEXT NOT NULL DEFAULT '',
		resolution_notes TEXT NOT NULL DEFAULT '',
		created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`)
	return err
}

func (s *PostgresStore) EnsureTable(ctx context.Context) error {
	return s.ensureTable(ctx)
}

// Get returns a single incident state by request_id.
func (s *PostgresStore) Get(requestID string) (State, error) {
	if s.pool == nil || requestID == "" {
		return State{}, ErrNotFound
	}
	row, err := s.pool.Query(context.Background(), `
	SELECT request_id, org_id, status, assignee, reason, tags, comments,
	       triaged_at, triaged_by, resolved_at, resolved_by,
	       resolution, resolution_notes, created_at, updated_at
	FROM incident_lifecycle WHERE request_id = $1`, requestID)
	if err != nil {
		return State{}, fmt.Errorf("query incident: %w", err)
	}
	defer row.Close()
	if !row.Next() {
		return State{}, ErrNotFound
	}
	st, err := scanState(row)
	if err != nil {
		return State{}, err
	}
	if err := row.Err(); err != nil {
		return State{}, fmt.Errorf("scan incident: %w", err)
	}
	return st, nil
}

// List returns the most recently updated incidents (capped at limit).
func (s *PostgresStore) List(limit int) []State {
	if s.pool == nil {
		return nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(context.Background(), `
	SELECT request_id, org_id, status, assignee, reason, tags, comments,
	       triaged_at, triaged_by, resolved_at, resolved_by,
	       resolution, resolution_notes, created_at, updated_at
	FROM incident_lifecycle
	ORDER BY updated_at DESC
	LIMIT $1`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []State
	for rows.Next() {
		st, err := scanState(rows)
		if err != nil {
			continue
		}
		out = append(out, st)
	}
	return out
}

// Apply creates or patches an incident record.
func (s *PostgresStore) Apply(requestID string, p Patch) (State, error) {
	if s.pool == nil || requestID == "" {
		return State{}, fmt.Errorf("missing request_id")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fetch existing state.
	cur, err := s.Get(requestID)
	now := time.Now().UTC()
	if err != nil {
		if err == ErrNotFound {
			cur = State{RequestID: requestID, Status: StatusOpen, CreatedAt: now}
		} else {
			return State{}, err
		}
	}

	if p.Status != nil {
		cur.Status = *p.Status
	}
	if p.Assignee != nil {
		cur.Assignee = *p.Assignee
	}
	if p.Reason != nil {
		cur.Reason = *p.Reason
	}
	if p.Tags != nil {
		cur.Tags = dedup(p.Tags)
	}
	if p.AddComment != nil {
		c := *p.AddComment
		if c.CreatedAt.IsZero() {
			c.CreatedAt = now
		}
		cur.Comments = append(cur.Comments, c)
	}
	if p.Resolution != nil {
		cur.Resolution = *p.Resolution
	}
	if p.ResolutionNotes != nil {
		cur.ResolutionNotes = *p.ResolutionNotes
	}
	cur.UpdatedAt = now

	if err := s.upsertState(ctx, cur); err != nil {
		return State{}, err
	}
	return cur, nil
}

// Triage sets the incident status to In Progress and records the assignee.
func (s *PostgresStore) Triage(ctx context.Context, requestID, assignee string) error {
	if s.pool == nil || requestID == "" {
		return fmt.Errorf("missing request_id")
	}
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx2, `
	INSERT INTO incident_lifecycle (request_id, org_id, status, assignee, triaged_at, triaged_by, updated_at)
	VALUES ($1, '', $2, $3, $4, $5, $4)
	ON CONFLICT (request_id) DO UPDATE SET
		status      = EXCLUDED.status,
		assignee    = COALESCE(NULLIF(EXCLUDED.assignee, ''), incident_lifecycle.assignee),
		triaged_at  = EXCLUDED.triaged_at,
		triaged_by  = EXCLUDED.triaged_by,
		updated_at  = EXCLUDED.updated_at`,
		requestID, StatusInProgress, assignee, now)
	if err != nil {
		return fmt.Errorf("triage: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("triage: no rows affected for %s", requestID)
	}
	return nil
}

// Resolve closes an incident with a resolution type and notes.
func (s *PostgresStore) Resolve(ctx context.Context, requestID, resolution, notes, resolvedBy string) error {
	if s.pool == nil || requestID == "" {
		return fmt.Errorf("missing request_id")
	}
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx2, `
	INSERT INTO incident_lifecycle (request_id, org_id, status, resolution, resolution_notes, resolved_at, resolved_by, updated_at)
	VALUES ($1, '', $2, $3, $4, $5, $6, $5)
	ON CONFLICT (request_id) DO UPDATE SET
		status           = EXCLUDED.status,
		resolution       = EXCLUDED.resolution,
		resolution_notes = EXCLUDED.resolution_notes,
		resolved_at      = EXCLUDED.resolved_at,
		resolved_by      = EXCLUDED.resolved_by,
		updated_at       = EXCLUDED.updated_at`,
		requestID, StatusClosed, resolution, notes, now, resolvedBy)
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("resolve: no rows affected for %s", requestID)
	}
	return nil
}

// Reopen reopens a resolved/closed incident.
func (s *PostgresStore) Reopen(ctx context.Context, requestID string) error {
	if s.pool == nil || requestID == "" {
		return fmt.Errorf("missing request_id")
	}
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx2, `
	UPDATE incident_lifecycle SET
		status           = $2,
		resolution       = '',
		resolution_notes = '',
		resolved_at      = NULL,
		resolved_by      = '',
		updated_at       = $3
	WHERE request_id = $1 AND status = $4`,
		requestID, StatusOpen, now, StatusClosed)
	if err != nil {
		return fmt.Errorf("reopen: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("reopen: incident %s not found or not in Closed status", requestID)
	}
	return nil
}

// CalculateMTTR computes mean time to resolve for resolved incidents in the given window.
func (s *PostgresStore) CalculateMTTR(ctx context.Context, orgID string, from, to time.Time) (*MTTRStats, error) {
	if s.pool == nil {
		return &MTTRStats{}, nil
	}
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Overall MTTR for the current period.
	var overall *float64
	err := s.pool.QueryRow(ctx2, `
	SELECT AVG(EXTRACT(EPOCH FROM (resolved_at - created_at)) / 60)
	FROM incident_lifecycle
	WHERE resolved_at IS NOT NULL
	  AND resolved_at >= $2
	  AND resolved_at <= $3
	  AND ($1 = '' OR org_id = $1)`,
		orgID, from, to).Scan(&overall)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &MTTRStats{}, nil
		}
		return nil, fmt.Errorf("mttr query: %w", err)
	}
	if overall == nil {
		return &MTTRStats{}, nil
	}

	// By severity (from tags or we infer from finding data; fallback to status groupings).
	// Here we group by resolution type as a proxy for severity analysis.
	type row struct {
		Resolution string  `json:"-"`
		AvgMin     float64 `json:"avg_min"`
	}
	sevRows, err := s.pool.Query(ctx2, `
	SELECT COALESCE(NULLIF(resolution, ''), 'unspecified'),
	       AVG(EXTRACT(EPOCH FROM (resolved_at - created_at)) / 60)
	FROM incident_lifecycle
	WHERE resolved_at IS NOT NULL
	  AND resolved_at >= $2
	  AND resolved_at <= $3
	  AND ($1 = '' OR org_id = $1)
	GROUP BY resolution`,
		orgID, from, to)
	if err != nil {
		return nil, fmt.Errorf("mttr by severity: %w", err)
	}
	defer sevRows.Close()
	bySeverity := make(map[string]float64)
	for sevRows.Next() {
		var r row
		if err := sevRows.Scan(&r.Resolution, &r.AvgMin); err != nil {
			continue
		}
		bySeverity[r.Resolution] = r.AvgMin
	}

	// Trend: compare current period MTTR to previous equal-length period.
	periodLen := to.Sub(from)
	prevFrom := from.Add(-periodLen)
	prevTo := from.Add(-1 * time.Second)
	var prevMTTR *float64
	err = s.pool.QueryRow(ctx2, `
	SELECT AVG(EXTRACT(EPOCH FROM (resolved_at - created_at)) / 60)
	FROM incident_lifecycle
	WHERE resolved_at IS NOT NULL
	  AND resolved_at >= $2
	  AND resolved_at <= $3
	  AND ($1 = '' OR org_id = $1)`,
		orgID, prevFrom, prevTo).Scan(&prevMTTR)
	trend := "stable"
	if err == nil && prevMTTR != nil {
		diff := *overall - *prevMTTR
		if diff < -5 { // improved by more than 5 minutes
			trend = "improving"
		} else if diff > 5 { // worsened by more than 5 minutes
			trend = "worsening"
		}
	}

	// SLA compliance: percentage resolved within 60 minutes.
	var totalResolved int64
	var withinSLA int64
	err = s.pool.QueryRow(ctx2, `
	SELECT COUNT(*),
	       COUNT(*) FILTER (WHERE EXTRACT(EPOCH FROM (resolved_at - created_at)) / 60 <= 60)
	FROM incident_lifecycle
	WHERE resolved_at IS NOT NULL
	  AND resolved_at >= $2
	  AND resolved_at <= $3
	  AND ($1 = '' OR org_id = $1)`,
		orgID, from, to).Scan(&totalResolved, &withinSLA)
	if err != nil {
		return nil, fmt.Errorf("sla compliance: %w", err)
	}
	slaCompliance := float64(0)
	if totalResolved > 0 {
		slaCompliance = float64(withinSLA) / float64(totalResolved) * 100
	}

	return &MTTRStats{
		OverallMinutes: *overall,
		BySeverity:     bySeverity,
		Trend:          trend,
		SLACompliance:  slaCompliance,
	}, nil
}

// ListIncidents returns filtered incidents with pagination from Postgres.
func (s *PostgresStore) ListIncidents(ctx context.Context, opts ListIncidentsOptions) ([]State, int, error) {
	if s.pool == nil {
		return nil, 0, nil
	}
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	// Build WHERE clause.
	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if opts.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, opts.Status)
		argIdx++
	}
	if opts.Resolution != "" {
		where += fmt.Sprintf(" AND resolution = $%d", argIdx)
		args = append(args, opts.Resolution)
		argIdx++
	}
	if opts.Assignee != "" {
		where += fmt.Sprintf(" AND assignee = $%d", argIdx)
		args = append(args, opts.Assignee)
		argIdx++
	}

	// Get total count.
	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM incident_lifecycle %s", where)
	if err := s.pool.QueryRow(ctx2, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count incidents: %w", err)
	}

	// Get page.
	listArgs := make([]interface{}, len(args)+2)
	copy(listArgs, args)
	listArgs[len(args)] = limit
	listArgs[len(args)+1] = offset
	listQuery := fmt.Sprintf(`
	SELECT request_id, org_id, status, assignee, reason, tags, comments,
	       triaged_at, triaged_by, resolved_at, resolved_by,
	       resolution, resolution_notes, created_at, updated_at
	FROM incident_lifecycle
	%s
	ORDER BY updated_at DESC
	LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	rows, err := s.pool.Query(ctx2, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list incidents: %w", err)
	}
	defer rows.Close()

	var out []State
	for rows.Next() {
		st, err := scanState(rows)
		if err != nil {
			continue
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("scan incidents: %w", err)
	}

	return out, total, nil
}

// Save persists a full State record (upsert).
func (s *PostgresStore) Save(ctx context.Context, st State) error {
	if s.pool == nil {
		return fmt.Errorf("postgres pool not available")
	}
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.upsertState(ctx2, st)
}

// upsertState inserts or updates a full incident_lifecycle row.
func (s *PostgresStore) upsertState(ctx context.Context, st State) error {
	commentsJSON, _ := json.Marshal(st.Comments)
	tags := st.Tags
	if tags == nil {
		tags = []string{}
	}
	_, err := s.pool.Exec(ctx, `
	INSERT INTO incident_lifecycle (
		request_id, org_id, status, assignee, reason, tags, comments,
		triaged_at, triaged_by, resolved_at, resolved_by,
		resolution, resolution_notes, created_at, updated_at
	) VALUES (
		$1,  $2,  $3,  $4,  $5,  $6,  $7::jsonb,
		$8,  $9,  $10, $11, $12, $13, $14, $15
	)
	ON CONFLICT (request_id) DO UPDATE SET
		org_id           = EXCLUDED.org_id,
		status           = EXCLUDED.status,
		assignee         = EXCLUDED.assignee,
		reason           = EXCLUDED.reason,
		tags             = EXCLUDED.tags,
		comments         = EXCLUDED.comments,
		triaged_at       = EXCLUDED.triaged_at,
		triaged_by       = EXCLUDED.triaged_by,
		resolved_at      = EXCLUDED.resolved_at,
		resolved_by      = EXCLUDED.resolved_by,
		resolution       = EXCLUDED.resolution,
		resolution_notes = EXCLUDED.resolution_notes,
		updated_at       = EXCLUDED.updated_at`,
		st.RequestID, st.OrgID, st.Status, st.Assignee, st.Reason, tags, string(commentsJSON),
		st.TriagedAt, st.TriagedBy, st.ResolvedAt, st.ResolvedBy,
		st.Resolution, st.ResolutionNotes, st.CreatedAt, st.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert incident %s: %w", st.RequestID, err)
	}
	return nil
}

// scanState reads a State from a pgx.Rows result.
func scanState(row pgx.Row) (State, error) {
	var st State
	var commentsRaw []byte
	var tags []string
	if err := row.Scan(
		&st.RequestID, &st.OrgID, &st.Status, &st.Assignee, &st.Reason, &tags, &commentsRaw,
		&st.TriagedAt, &st.TriagedBy, &st.ResolvedAt, &st.ResolvedBy,
		&st.Resolution, &st.ResolutionNotes, &st.CreatedAt, &st.UpdatedAt,
	); err != nil {
		return State{}, fmt.Errorf("scan state row: %w", err)
	}
	st.Tags = tags
	if len(commentsRaw) > 0 {
		_ = json.Unmarshal(commentsRaw, &st.Comments)
	}
	return st, nil
}

// Compile-time interface checks.
var (
	_ Store          = (*PostgresStore)(nil)
	_ LifecycleStore = (*PostgresStore)(nil)
	_ LifecycleStore = (*MemoryStore)(nil)
)

// Lifecycle sort helper for by-severity map keys.
func sortIncidentStates(ss []State) {
	sort.Slice(ss, func(i, j int) bool {
		return ss[i].UpdatedAt.After(ss[j].UpdatedAt)
	})
}
