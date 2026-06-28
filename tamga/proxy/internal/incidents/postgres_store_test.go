package incidents

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ---------------------------------------------------------------------------
// Mock implementations for pgxPool, pgx.Rows, and pgx.Row
// ---------------------------------------------------------------------------

// mockPgxPool implements pgxPool for testing PostgresStore.
type mockPgxPool struct {
	execFn     func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row

	execCalls  int
	queryCalls int
	lastSQL    string
	lastArgs   []any
}

func (m *mockPgxPool) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	m.execCalls++
	m.lastSQL = sql
	m.lastArgs = arguments
	if m.execFn != nil {
		return m.execFn(ctx, sql, arguments...)
	}
	return pgconn.NewCommandTag("OK"), nil
}

func (m *mockPgxPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	m.queryCalls++
	if m.queryFn != nil {
		return m.queryFn(ctx, sql, args...)
	}
	return nil, nil
}

func (m *mockPgxPool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, sql, args...)
	}
	return &mockRow{}
}

// mockRow implements pgx.Row.
type mockRow struct {
	scanFn func(dest ...any) error
}

func (m *mockRow) Scan(dest ...any) error {
	if m.scanFn != nil {
		return m.scanFn(dest...)
	}
	return nil
}

// mockIncidentRows implements pgx.Rows for incident_lifecycle queries.
type mockIncidentRows struct {
	states []State
	idx    int
	err    error
}

func (m *mockIncidentRows) Close()                                       {}
func (m *mockIncidentRows) Err() error                                   { return m.err }
func (m *mockIncidentRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT") }
func (m *mockIncidentRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (m *mockIncidentRows) Values() ([]any, error)                       { return nil, nil }
func (m *mockIncidentRows) RawValues() [][]byte                          { return nil }
func (m *mockIncidentRows) Conn() *pgx.Conn                              { return nil }

func (m *mockIncidentRows) Next() bool {
	m.idx++
	return m.idx < len(m.states)
}

func (m *mockIncidentRows) Scan(dest ...any) error {
	if m.idx < 0 || m.idx >= len(m.states) {
		return errors.New("mockIncidentRows: index out of range")
	}
	s := m.states[m.idx]
	// Order: request_id, org_id, status, assignee, reason, tags, comments,
	//        triaged_at, triaged_by, resolved_at, resolved_by,
	//        resolution, resolution_notes, created_at, updated_at
	if len(dest) > 0 {
		if dp, ok := dest[0].(*string); ok {
			*dp = s.RequestID
		}
	}
	if len(dest) > 1 {
		if dp, ok := dest[1].(*string); ok {
			*dp = s.OrgID
		}
	}
	if len(dest) > 2 {
		if dp, ok := dest[2].(*string); ok {
			*dp = s.Status
		}
	}
	if len(dest) > 3 {
		if dp, ok := dest[3].(*string); ok {
			*dp = s.Assignee
		}
	}
	if len(dest) > 4 {
		if dp, ok := dest[4].(*string); ok {
			*dp = s.Reason
		}
	}
	if len(dest) > 5 {
		if dp, ok := dest[5].(*[]string); ok {
			*dp = s.Tags
		}
	}
	if len(dest) > 6 {
		if dp, ok := dest[6].(*[]byte); ok {
			if len(s.Comments) > 0 {
				*dp = []byte(`[{"author":"tester","text":"hello"}]`)
			} else {
				*dp = []byte(`[]`)
			}
		}
	}
	if len(dest) > 7 {
		if dp, ok := dest[7].(**time.Time); ok {
			*dp = s.TriagedAt
		}
	}
	if len(dest) > 8 {
		if dp, ok := dest[8].(*string); ok {
			*dp = s.TriagedBy
		}
	}
	if len(dest) > 9 {
		if dp, ok := dest[9].(**time.Time); ok {
			*dp = s.ResolvedAt
		}
	}
	if len(dest) > 10 {
		if dp, ok := dest[10].(*string); ok {
			*dp = s.ResolvedBy
		}
	}
	if len(dest) > 11 {
		if dp, ok := dest[11].(*string); ok {
			*dp = s.Resolution
		}
	}
	if len(dest) > 12 {
		if dp, ok := dest[12].(*string); ok {
			*dp = s.ResolutionNotes
		}
	}
	if len(dest) > 13 {
		if dp, ok := dest[13].(*time.Time); ok {
			*dp = s.CreatedAt
		}
	}
	if len(dest) > 14 {
		if dp, ok := dest[14].(*time.Time); ok {
			*dp = s.UpdatedAt
		}
	}
	return nil
}

func newMockPostgresStore(mock *mockPgxPool) *PostgresStore {
	return &PostgresStore{pool: mock}
}

// ---------------------------------------------------------------------------
// NewPostgresStore
// ---------------------------------------------------------------------------

func TestPostgresStore_New_NilPool(t *testing.T) {
	ps := NewPostgresStore(nil)
	if ps == nil {
		t.Fatal("expected non-nil PostgresStore")
	}
	if ps.pool != nil {
		t.Error("expected nil pool")
	}
	// Methods should safely return zero values.
	st, err := ps.Get("any")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if st.RequestID != "" {
		t.Errorf("expected empty State, got %v", st)
	}
	list := ps.List(10)
	if list != nil {
		t.Errorf("expected nil list, got %v", list)
	}
}

// ---------------------------------------------------------------------------
// ensureTable / EnsureTable
// ---------------------------------------------------------------------------

func TestPostgresStore_EnsureTable_Success(t *testing.T) {
	mock := &mockPgxPool{}
	ps := newMockPostgresStore(mock)

	err := ps.EnsureTable(context.Background())
	if err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	if mock.execCalls != 1 {
		t.Errorf("expected 1 Exec call, got %d", mock.execCalls)
	}
}

func TestPostgresStore_EnsureTable_Error(t *testing.T) {
	wantErr := errors.New("connection lost")
	mock := &mockPgxPool{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, wantErr
		},
	}
	ps := newMockPostgresStore(mock)

	err := ps.EnsureTable(context.Background())
	if err != wantErr {
		t.Errorf("expected %v, got %v", wantErr, err)
	}
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

func TestPostgresStore_Get_Found(t *testing.T) {
	now := time.Now().UTC()
	ts := timePtr(now.Add(-10 * time.Minute))
	st := State{
		RequestID: "req-get",
		OrgID:     "org1",
		Status:    StatusInProgress,
		Assignee:  "analyst",
		TriagedAt: ts,
		TriagedBy: "analyst",
		CreatedAt: now.Add(-1 * time.Hour),
		UpdatedAt: now,
	}
	rows := &mockIncidentRows{states: []State{st}, idx: -1}
	mock := &mockPgxPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return rows, nil
		},
	}
	ps := newMockPostgresStore(mock)

	got, err := ps.Get("req-get")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.RequestID != "req-get" {
		t.Errorf("RequestID = %q, want req-get", got.RequestID)
	}
	if got.Status != StatusInProgress {
		t.Errorf("Status = %q, want %q", got.Status, StatusInProgress)
	}
	if got.Assignee != "analyst" {
		t.Errorf("Assignee = %q, want analyst", got.Assignee)
	}
}

func TestPostgresStore_Get_NotFound(t *testing.T) {
	rows := &mockIncidentRows{idx: -1} // no rows
	mock := &mockPgxPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return rows, nil
		},
	}
	ps := newMockPostgresStore(mock)

	_, err := ps.Get("missing")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresStore_Get_QueryError(t *testing.T) {
	wantErr := errors.New("query failed")
	mock := &mockPgxPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return nil, wantErr
		},
	}
	ps := newMockPostgresStore(mock)

	_, err := ps.Get("req")
	if err == nil {
		t.Error("expected error")
	}
}

func TestPostgresStore_Get_EmptyRequestID(t *testing.T) {
	mock := &mockPgxPool{}
	ps := newMockPostgresStore(mock)

	_, err := ps.Get("")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestPostgresStore_List_WithRows(t *testing.T) {
	now := time.Now().UTC()
	states := []State{
		{RequestID: "a", Status: StatusOpen, UpdatedAt: now},
		{RequestID: "b", Status: StatusClosed, UpdatedAt: now.Add(time.Second)},
	}
	rows := &mockIncidentRows{states: states, idx: -1}
	mock := &mockPgxPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return rows, nil
		},
	}
	ps := newMockPostgresStore(mock)

	list := ps.List(10)
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	if list[0].RequestID != "a" {
		t.Errorf("list[0] = %q", list[0].RequestID)
	}
}

func TestPostgresStore_List_Empty(t *testing.T) {
	rows := &mockIncidentRows{idx: -1}
	mock := &mockPgxPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return rows, nil
		},
	}
	ps := newMockPostgresStore(mock)

	list := ps.List(10)
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestPostgresStore_List_NilPool(t *testing.T) {
	ps := &PostgresStore{pool: nil}
	list := ps.List(10)
	if list != nil {
		t.Errorf("expected nil, got %v", list)
	}
}

// ---------------------------------------------------------------------------
// Triage
// ---------------------------------------------------------------------------

func TestPostgresStore_Triage_Success(t *testing.T) {
	mock := &mockPgxPool{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	ps := newMockPostgresStore(mock)

	err := ps.Triage(context.Background(), "req-t", "analyst1")
	if err != nil {
		t.Fatalf("Triage: %v", err)
	}
	if mock.execCalls != 1 {
		t.Errorf("expected 1 Exec call, got %d", mock.execCalls)
	}
}

func TestPostgresStore_Triage_NoRowsAffected(t *testing.T) {
	mock := &mockPgxPool{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 0"), nil
		},
	}
	ps := newMockPostgresStore(mock)

	err := ps.Triage(context.Background(), "req-t", "analyst1")
	if err == nil {
		t.Error("expected error for 0 rows affected")
	}
}

func TestPostgresStore_Triage_NilPool(t *testing.T) {
	ps := &PostgresStore{pool: nil}
	err := ps.Triage(context.Background(), "req", "a")
	if err == nil {
		t.Error("expected error from nil pool")
	}
}

func TestPostgresStore_Triage_EmptyRequestID(t *testing.T) {
	mock := &mockPgxPool{}
	ps := newMockPostgresStore(mock)

	err := ps.Triage(context.Background(), "", "a")
	if err == nil {
		t.Error("expected error for empty request ID")
	}
}

// ---------------------------------------------------------------------------
// Resolve
// ---------------------------------------------------------------------------

func TestPostgresStore_Resolve_Success(t *testing.T) {
	mock := &mockPgxPool{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	ps := newMockPostgresStore(mock)

	err := ps.Resolve(context.Background(), "req-r", "tp", "notes", "alice")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}

func TestPostgresStore_Resolve_NoRowsAffected(t *testing.T) {
	mock := &mockPgxPool{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 0"), nil
		},
	}
	ps := newMockPostgresStore(mock)

	err := ps.Resolve(context.Background(), "req-r", "tp", "notes", "a")
	if err == nil {
		t.Error("expected error for 0 rows affected")
	}
}

func TestPostgresStore_Resolve_NilPool(t *testing.T) {
	ps := &PostgresStore{pool: nil}
	err := ps.Resolve(context.Background(), "req", "tp", "n", "a")
	if err == nil {
		t.Error("expected error from nil pool")
	}
}

// ---------------------------------------------------------------------------
// Reopen
// ---------------------------------------------------------------------------

func TestPostgresStore_Reopen_Success(t *testing.T) {
	mock := &mockPgxPool{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	ps := newMockPostgresStore(mock)

	err := ps.Reopen(context.Background(), "req-ro")
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
}

func TestPostgresStore_Reopen_NotFound(t *testing.T) {
	mock := &mockPgxPool{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
	}
	ps := newMockPostgresStore(mock)

	err := ps.Reopen(context.Background(), "missing")
	if err == nil {
		t.Error("expected error when no rows updated")
	}
}

func TestPostgresStore_Reopen_NilPool(t *testing.T) {
	ps := &PostgresStore{pool: nil}
	err := ps.Reopen(context.Background(), "req")
	if err == nil {
		t.Error("expected error from nil pool")
	}
}

// ---------------------------------------------------------------------------
// Save / upsertState
// ---------------------------------------------------------------------------

func TestPostgresStore_Save_Success(t *testing.T) {
	mock := &mockPgxPool{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	ps := newMockPostgresStore(mock)

	st := State{
		RequestID: "req-save",
		Status:    StatusOpen,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	err := ps.Save(context.Background(), st)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func TestPostgresStore_Save_NilPool(t *testing.T) {
	ps := &PostgresStore{pool: nil}
	err := ps.Save(context.Background(), State{RequestID: "req"})
	if err == nil {
		t.Error("expected error from nil pool")
	}
}

// ---------------------------------------------------------------------------
// CalculateMTTR
// ---------------------------------------------------------------------------

func TestPostgresStore_CalculateMTTR_NilPool(t *testing.T) {
	ps := &PostgresStore{pool: nil}
	now := time.Now().UTC()
	stats, err := ps.CalculateMTTR(context.Background(), "", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CalculateMTTR: %v", err)
	}
	if stats.OverallMinutes != 0 {
		t.Errorf("expected 0, got %f", stats.OverallMinutes)
	}
}

func TestPostgresStore_CalculateMTTR_NoRows(t *testing.T) {
	mock := &mockPgxPool{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			// Return ErrNoRows.
			return &mockRow{
				scanFn: func(dest ...any) error {
					return pgx.ErrNoRows
				},
			}
		},
	}
	ps := newMockPostgresStore(mock)

	now := time.Now().UTC()
	stats, err := ps.CalculateMTTR(context.Background(), "", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CalculateMTTR: %v", err)
	}
	if stats.OverallMinutes != 0 {
		t.Errorf("expected 0 (no rows), got %f", stats.OverallMinutes)
	}
}

// ---------------------------------------------------------------------------
// ListIncidents
// ---------------------------------------------------------------------------

func TestPostgresStore_ListIncidents_NilPool(t *testing.T) {
	ps := &PostgresStore{pool: nil}
	results, total, err := ps.ListIncidents(context.Background(), ListIncidentsOptions{Limit: 10})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil, got %v", results)
	}
	if total != 0 {
		t.Errorf("expected 0, got %d", total)
	}
}

func TestPostgresStore_ListIncidents_WithFilters(t *testing.T) {
	now := time.Now().UTC()
	items := []State{
		{RequestID: "inc-a", Status: StatusOpen, UpdatedAt: now},
		{RequestID: "inc-b", Status: StatusInProgress, UpdatedAt: now.Add(time.Second)},
	}
	rows := &mockIncidentRows{states: items, idx: -1}
	mock := &mockPgxPool{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &mockRow{
				scanFn: func(dest ...any) error {
					if dp, ok := dest[0].(*int); ok {
						*dp = 2
					}
					return nil
				},
			}
		},
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return rows, nil
		},
	}
	ps := newMockPostgresStore(mock)

	results, total, err := ps.ListIncidents(context.Background(), ListIncidentsOptions{
		Status: StatusOpen,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("ListIncidents: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
}

// ---------------------------------------------------------------------------
// Apply (delegates to Get + upsertState)
// ---------------------------------------------------------------------------

func TestPostgresStore_Apply_NilPool(t *testing.T) {
	ps := &PostgresStore{pool: nil}
	_, err := ps.Apply("req", Patch{})
	if err == nil {
		t.Error("expected error from nil pool")
	}
}

func TestPostgresStore_Apply_EmptyRequestID(t *testing.T) {
	mock := &mockPgxPool{}
	ps := newMockPostgresStore(mock)

	_, err := ps.Apply("", Patch{})
	if err == nil {
		t.Error("expected error for empty request ID")
	}
}

func TestPostgresStore_Apply_CreateNew(t *testing.T) {
	// Get returns ErrNotFound (no existing row), upsertState succeeds.
	mock := &mockPgxPool{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return &mockIncidentRows{idx: -1}, nil // no rows = not found
		},
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	ps := newMockPostgresStore(mock)

	status := StatusOpen
	st, err := ps.Apply("req-new", Patch{Status: &status})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if st.RequestID != "req-new" {
		t.Errorf("RequestID = %q", st.RequestID)
	}
	if st.Status != StatusOpen {
		t.Errorf("Status = %q", st.Status)
	}
}

// ---------------------------------------------------------------------------
// sortIncidentStates
// ---------------------------------------------------------------------------

func TestSortIncidentStates(t *testing.T) {
	now := time.Now().UTC()
	states := []State{
		{RequestID: "old", UpdatedAt: now.Add(-1 * time.Hour)},
		{RequestID: "new", UpdatedAt: now},
		{RequestID: "mid", UpdatedAt: now.Add(-30 * time.Minute)},
	}
	sortIncidentStates(states)
	if states[0].RequestID != "new" {
		t.Errorf("states[0] = %q, want new", states[0].RequestID)
	}
	if states[1].RequestID != "mid" {
		t.Errorf("states[1] = %q, want mid", states[1].RequestID)
	}
	if states[2].RequestID != "old" {
		t.Errorf("states[2] = %q, want old", states[2].RequestID)
	}
}
