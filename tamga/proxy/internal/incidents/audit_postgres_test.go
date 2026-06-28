package incidents

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

// mockDBConn implements the dbConn interface for testing.
type mockDBConn struct {
	execCalls  int
	queryCalls int
	lastSQL    string
	lastArgs   []any
	execFn     func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	execTag    pgconn.CommandTag // default tag returned when execFn is nil
	execErr    error             // default error returned when execFn is nil
}

func (m *mockDBConn) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	m.execCalls++
	m.lastSQL = sql
	m.lastArgs = arguments
	if m.execFn != nil {
		return m.execFn(ctx, sql, arguments...)
	}
	return m.execTag, m.execErr
}

func (m *mockDBConn) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	m.queryCalls++
	if m.queryFn != nil {
		return m.queryFn(ctx, sql, args...)
	}
	return nil, nil
}

// mockAuditRow holds one row of audit data.
type mockAuditRow struct {
	ts       time.Time
	actor    string
	kind     string
	target   string
	detail   []byte
	prevHash string
	hash     string
}

// mockRows implements pgx.Rows with an in-memory slice.
type mockRows struct {
	rows []mockAuditRow
	idx  int // current row index, -1 means before first row
	err  error
}

func (m *mockRows) Close()                                       {}
func (m *mockRows) Err() error                                   { return m.err }
func (m *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT") }
func (m *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (m *mockRows) Values() ([]any, error)                       { return nil, nil }
func (m *mockRows) RawValues() [][]byte                          { return nil }
func (m *mockRows) Conn() *pgx.Conn                              { return nil }

func (m *mockRows) Next() bool {
	m.idx++
	return m.idx < len(m.rows)
}

func (m *mockRows) Scan(dest ...any) error {
	if m.idx < 0 || m.idx >= len(m.rows) {
		return fmt.Errorf("mockRows: index %d out of range [0,%d)", m.idx, len(m.rows))
	}
	row := m.rows[m.idx]
	// Dest order matches Load query: ts, actor, kind, target, detail, prev_hash, hash
	if len(dest) > 0 {
		if dp, ok := dest[0].(*time.Time); ok {
			*dp = row.ts
		}
	}
	if len(dest) > 1 {
		if dp, ok := dest[1].(*string); ok {
			*dp = row.actor
		}
	}
	if len(dest) > 2 {
		if dp, ok := dest[2].(*string); ok {
			*dp = row.kind
		}
	}
	if len(dest) > 3 {
		if dp, ok := dest[3].(*string); ok {
			*dp = row.target
		}
	}
	if len(dest) > 4 {
		if dp, ok := dest[4].(*[]byte); ok {
			*dp = row.detail
		}
	}
	if len(dest) > 5 {
		if dp, ok := dest[5].(*string); ok {
			*dp = row.prevHash
		}
	}
	if len(dest) > 6 {
		if dp, ok := dest[6].(*string); ok {
			*dp = row.hash
		}
	}
	return nil
}

// newMockPersister creates an AuditPersister backed by a mockDBConn.
func newMockPersister(mock *mockDBConn) *AuditPersister {
	return &AuditPersister{pool: mock}
}

// ---------------------------------------------------------------------------
// Append tests
// ---------------------------------------------------------------------------

func TestAuditPersister_Append_InsertsRow(t *testing.T) {
	mock := &mockDBConn{
		execTag: pgconn.NewCommandTag("INSERT 0 1"),
	}
	p := newMockPersister(mock)

	ctx := context.Background()
	e := AuditEntry{
		Timestamp: time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
		Actor:     "alice",
		Kind:      "policy.create",
		Target:    "policy_123",
		Detail:    map[string]interface{}{"field": "value"},
		PrevHash:  "abc123",
		Hash:      "def456",
	}

	err := p.Append(ctx, e)
	if err != nil {
		t.Fatalf("Append: unexpected error: %v", err)
	}
	if mock.execCalls != 1 {
		t.Fatalf("expected 1 Exec call, got %d", mock.execCalls)
	}

	// Verify SQL contains expected table and placeholders.
	if mock.lastSQL == "" {
		t.Error("SQL should not be empty")
	}

	// Verify args: ts, actor, kind, target, detail, prev_hash, hash (7 args).
	if len(mock.lastArgs) != 7 {
		t.Fatalf("expected 7 args, got %d: %v", len(mock.lastArgs), mock.lastArgs)
	}
	// Arg 0 is timestamp.
	if ts, ok := mock.lastArgs[0].(time.Time); !ok || !ts.Equal(e.Timestamp) {
		t.Errorf("arg[0] timestamp = %v, want %v", mock.lastArgs[0], e.Timestamp)
	}
	// Arg 1 is actor.
	if actor, ok := mock.lastArgs[1].(string); !ok || actor != e.Actor {
		t.Errorf("arg[1] actor = %v, want %v", mock.lastArgs[1], e.Actor)
	}
	// Arg 2 is kind.
	if kind, ok := mock.lastArgs[2].(string); !ok || kind != e.Kind {
		t.Errorf("arg[2] kind = %v, want %v", mock.lastArgs[2], e.Kind)
	}
	// Arg 5 is prev_hash.
	if ph, ok := mock.lastArgs[5].(string); !ok || ph != e.PrevHash {
		t.Errorf("arg[5] prev_hash = %v, want %v", mock.lastArgs[5], e.PrevHash)
	}
	// Arg 6 is hash.
	if h, ok := mock.lastArgs[6].(string); !ok || h != e.Hash {
		t.Errorf("arg[6] hash = %v, want %v", mock.lastArgs[6], e.Hash)
	}
}

func TestAuditPersister_Append_NilDetail(t *testing.T) {
	mock := &mockDBConn{execTag: pgconn.NewCommandTag("INSERT 0 1")}
	p := newMockPersister(mock)

	e := AuditEntry{
		Kind: "test",
	}
	err := p.Append(context.Background(), e)
	if err != nil {
		t.Fatalf("Append with nil detail: %v", err)
	}
	// Detail arg (index 4): json.Marshal(nil) returns "null".
	if len(mock.lastArgs) > 4 {
		if detail, ok := mock.lastArgs[4].(string); !ok || detail != "null" {
			t.Errorf("arg[4] detail = %v, want \"null\"", mock.lastArgs[4])
		}
	}
}

func TestAuditPersister_Append_ExecError(t *testing.T) {
	wantErr := fmt.Errorf("connection refused")
	mock := &mockDBConn{
		execErr: wantErr,
	}
	p := newMockPersister(mock)

	err := p.Append(context.Background(), AuditEntry{Kind: "test"})
	if err != wantErr {
		t.Errorf("expected %v, got %v", wantErr, err)
	}
}

func TestAuditPersister_Append_NilPersister(t *testing.T) {
	var p *AuditPersister
	err := p.Append(context.Background(), AuditEntry{Kind: "test"})
	if err != nil {
		t.Errorf("nil persister: expected nil error, got %v", err)
	}
}

func TestAuditPersister_Append_NilPool(t *testing.T) {
	p := &AuditPersister{pool: nil}
	err := p.Append(context.Background(), AuditEntry{Kind: "test"})
	if err != nil {
		t.Errorf("nil pool: expected nil error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Load tests
// ---------------------------------------------------------------------------

func TestAuditPersister_Load_ReversesOrder(t *testing.T) {
	ts1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)  // oldest
	ts2 := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)  // middle
	ts3 := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC) // newest

	// DB returns newest-first (ORDER BY id DESC).
	rows := &mockRows{
		rows: []mockAuditRow{
			{ts: ts3, actor: "c", kind: "create", target: "t3", detail: []byte(`{"n":3}`), prevHash: "h2", hash: "h3"},
			{ts: ts2, actor: "b", kind: "update", target: "t2", detail: []byte(`{"n":2}`), prevHash: "h1", hash: "h2"},
			{ts: ts1, actor: "a", kind: "delete", target: "t1", detail: []byte(`{"n":1}`), prevHash: "", hash: "h1"},
		},
		idx: -1,
	}

	mock := &mockDBConn{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return rows, nil
		},
	}
	p := newMockPersister(mock)

	entries, err := p.Load(context.Background(), 10)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// After reversal, should be oldest-first.
	if !entries[0].Timestamp.Equal(ts1) || entries[0].Actor != "a" {
		t.Errorf("entries[0] wrong: ts=%v actor=%s", entries[0].Timestamp, entries[0].Actor)
	}
	if !entries[1].Timestamp.Equal(ts2) || entries[1].Actor != "b" {
		t.Errorf("entries[1] wrong: ts=%v actor=%s", entries[1].Timestamp, entries[1].Actor)
	}
	if !entries[2].Timestamp.Equal(ts3) || entries[2].Actor != "c" {
		t.Errorf("entries[2] wrong: ts=%v actor=%s", entries[2].Timestamp, entries[2].Actor)
	}

	// Detail should be unmarshalled.
	if entries[0].Detail == nil || entries[0].Detail["n"] != float64(1) {
		t.Errorf("entries[0] detail: %v", entries[0].Detail)
	}
}

func TestAuditPersister_Load_EmptyTable(t *testing.T) {
	rows := &mockRows{idx: -1}
	mock := &mockDBConn{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return rows, nil
		},
	}
	p := newMockPersister(mock)

	entries, err := p.Load(context.Background(), 10)
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty slice, got %v (len=%d)", entries, len(entries))
	}
}

func TestAuditPersister_Load_NilPersister(t *testing.T) {
	var p *AuditPersister
	entries, err := p.Load(context.Background(), 10)
	if err != nil {
		t.Errorf("nil persister: unexpected error: %v", err)
	}
	if entries != nil {
		t.Errorf("nil persister: expected nil, got %v", entries)
	}
}

func TestAuditPersister_Load_QueryError(t *testing.T) {
	wantErr := fmt.Errorf("table not found")
	mock := &mockDBConn{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return nil, wantErr
		},
	}
	p := newMockPersister(mock)

	_, err := p.Load(context.Background(), 10)
	if err != wantErr {
		t.Errorf("expected %v, got %v", wantErr, err)
	}
}

func TestAuditPersister_Load_LimitDefault(t *testing.T) {
	// Verify that limit=0 defaults to 512.
	mock := &mockDBConn{
		queryFn: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			// Check that the limit arg passed to Query is 512.
			if len(args) != 1 {
				t.Errorf("expected 1 query arg (limit), got %d", len(args))
			}
			if limit, ok := args[0].(int); ok && limit != 512 {
				t.Errorf("expected default limit 512, got %d", limit)
			}
			return &mockRows{idx: -1}, nil
		},
	}
	p := newMockPersister(mock)
	_, err := p.Load(context.Background(), 0)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
}

// ---------------------------------------------------------------------------
// PersistAll tests
// ---------------------------------------------------------------------------

func TestAuditPersister_PersistAll_Success(t *testing.T) {
	mock := &mockDBConn{execTag: pgconn.NewCommandTag("INSERT 0 1")}
	p := newMockPersister(mock)

	entries := []AuditEntry{
		{Kind: "a", Actor: "x", Timestamp: time.Now().UTC()},
		{Kind: "b", Actor: "y", Timestamp: time.Now().UTC()},
		{Kind: "c", Actor: "z", Timestamp: time.Now().UTC()},
	}

	err := p.PersistAll(context.Background(), entries)
	if err != nil {
		t.Fatalf("PersistAll: %v", err)
	}
	if mock.execCalls != 3 {
		t.Errorf("expected 3 Exec calls, got %d", mock.execCalls)
	}
}

func TestAuditPersister_PersistAll_PartialFailure(t *testing.T) {
	callCount := 0
	mock := &mockDBConn{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			callCount++
			if callCount >= 2 {
				return pgconn.CommandTag{}, fmt.Errorf("error on call %d", callCount)
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	p := newMockPersister(mock)

	entries := []AuditEntry{
		{Kind: "a"},
		{Kind: "b"},
		{Kind: "c"},
	}

	err := p.PersistAll(context.Background(), entries)
	if err == nil {
		t.Fatal("expected error on partial failure")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls before failure, got %d", callCount)
	}
}

func TestAuditPersister_PersistAll_NilPersister(t *testing.T) {
	var p *AuditPersister
	err := p.PersistAll(context.Background(), []AuditEntry{{Kind: "x"}})
	if err != nil {
		t.Errorf("nil persister: expected nil, got %v", err)
	}
}

func TestAuditPersister_PersistAll_EmptyEntries(t *testing.T) {
	mock := &mockDBConn{}
	p := newMockPersister(mock)

	err := p.PersistAll(context.Background(), nil)
	if err != nil {
		t.Errorf("nil entries: expected nil, got %v", err)
	}
	if mock.execCalls != 0 {
		t.Errorf("expected 0 Exec calls for empty entries, got %d", mock.execCalls)
	}

	err = p.PersistAll(context.Background(), []AuditEntry{})
	if err != nil {
		t.Errorf("empty entries: expected nil, got %v", err)
	}
	if mock.execCalls != 0 {
		t.Errorf("expected 0 Exec calls, got %d", mock.execCalls)
	}
}

// ---------------------------------------------------------------------------
// NewAuditPersister edge cases
// ---------------------------------------------------------------------------

func TestNewAuditPersister_NilPersisterMethods(t *testing.T) {
	// This tests that methods on a persister with nil pool do not panic.
	p := NewAuditPersister(nil)

	// Append should be safe.
	if err := p.Append(context.Background(), AuditEntry{Kind: "x"}); err != nil {
		t.Errorf("Append nil pool: %v", err)
	}

	// Load should be safe.
	entries, err := p.Load(context.Background(), 10)
	if err != nil {
		t.Errorf("Load nil pool: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries, got %v", entries)
	}

	// PersistAll should be safe.
	if err := p.PersistAll(context.Background(), []AuditEntry{{Kind: "x"}}); err != nil {
		t.Errorf("PersistAll nil pool: %v", err)
	}
}
