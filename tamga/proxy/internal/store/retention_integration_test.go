package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func skipIfNoDBRetention(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping retention integration test in short mode")
	}
	if os.Getenv("TAMGA_TEST_DB_URL") == "" {
		t.Skip("TAMGA_TEST_DB_URL not set; skipping retention integration test")
	}
}

func TestPG_Retention_NewPartitionManager(t *testing.T) {
	skipIfNoDBRetention(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()

	policy := DefaultRetentionPolicy()
	pm := NewPartitionManager(s.Pool(), policy, zerolog.Nop())
	if pm == nil {
		t.Fatal("NewPartitionManager returned nil")
	}
	if pm.LastRun().IsZero() != true {
		t.Error("fresh PartitionManager should have zero last run time")
	}
	// lastRunUnix should be loadable
	pm.lastRunUnix.Store(time.Now().UTC().Unix())
	if pm.LastRun().IsZero() {
		t.Error("after store, LastRun should not be zero")
	}
}

func TestPG_Retention_CreateUpcomingPartitions(t *testing.T) {
	skipIfNoDBRetention(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()

	policy := DefaultRetentionPolicy()
	pm := NewPartitionManager(s.Pool(), policy, zerolog.Nop())

	ctx := context.Background()
	if err := pm.createUpcomingPartitions(ctx, 2); err != nil {
		t.Fatalf("createUpcomingPartitions(2): %v", err)
	}

	// Verify partitions exist
	rows, err := s.pool.Query(ctx, `
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
			t.Fatalf("scan: %v", err)
		}
		names = append(names, name)
	}
	if len(names) < 2 {
		t.Errorf("expected at least 2 upcoming partitions, got %d: %v", len(names), names)
	}
}

func TestPG_Retention_DropExpiredPartitions(t *testing.T) {
	skipIfNoDBRetention(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()

	// Use a policy where request logs expire in 1 day
	policy := RetentionPolicy{
		RequestLogsDays:    1,
		AlertsDays:         90,
		DailyStatsDays:     365,
		AuditLogsDays:      365,
		IncidentClosedDays: 0,
	}
	pm := NewPartitionManager(s.Pool(), policy, zerolog.Nop())

	ctx := context.Background()

	// Create partitions for current and next month
	if err := pm.createUpcomingPartitions(ctx, 2); err != nil {
		t.Fatalf("create partitions: %v", err)
	}

	// Try to drop expired partitions (none should be old enough with 1-day window)
	if err := pm.dropExpiredPartitions(ctx); err != nil {
		t.Fatalf("dropExpiredPartitions: %v", err)
	}

	// Verify current month partition still exists
	now := time.Now().UTC()
	currentPart := fmt.Sprintf("request_logs_%04d_%02d", now.Year(), now.Month())
	var count int
	if err := s.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM pg_class WHERE relname = $1", currentPart).Scan(&count); err != nil {
		t.Fatalf("check partition: %v", err)
	}
	if count != 1 {
		t.Errorf("expected current partition %s to exist after drop, got count=%d", currentPart, count)
	}
}

func TestPG_Retention_PurgeOldRows(t *testing.T) {
	skipIfNoDBRetention(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Insert an old alert (100 days ago)
	oldTime := time.Now().UTC().Add(-100 * 24 * time.Hour)
	_, err := s.pool.Exec(ctx,
		"INSERT INTO alerts (title, severity, created_at) VALUES ($1, $2, $3)",
		"old-alert", "critical", oldTime)
	if err != nil {
		t.Fatalf("insert alert: %v", err)
	}

	// Insert an old audit log
	_, err = s.pool.Exec(ctx,
		"INSERT INTO audit_log (ts, action) VALUES ($1, $2)",
		oldTime, "test-action")
	if err != nil {
		t.Fatalf("insert audit_log: %v", err)
	}

	// Insert an old daily_stat
	oldDateStr := oldTime.Format("2006-01-02")
	_, err = s.pool.Exec(ctx,
		"INSERT INTO daily_stats (org_id, stat_date) VALUES ($1, $2)",
		testOrgID, oldDateStr)
	if err != nil {
		t.Fatalf("insert daily_stats: %v", err)
	}

	policy := RetentionPolicy{
		RequestLogsDays:    30,
		AlertsDays:         30, // Will purge anything older than 30 days
		DailyStatsDays:     30, // Same
		AuditLogsDays:      30, // Same
		IncidentClosedDays: 0,
	}
	pm := NewPartitionManager(s.Pool(), policy, zerolog.Nop())

	if err := pm.purgeOldRows(ctx); err != nil {
		t.Fatalf("purgeOldRows: %v", err)
	}

	// Verify old alert was purged
	var alertCount int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM alerts WHERE title = 'old-alert'").Scan(&alertCount); err != nil {
		t.Fatalf("check alert: %v", err)
	}
	if alertCount != 0 {
		t.Errorf("expected old alert to be purged, got %d", alertCount)
	}

	// Verify old audit_log was purged
	var auditCount int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_log WHERE action = 'test-action'").Scan(&auditCount); err != nil {
		t.Fatalf("check audit_log: %v", err)
	}
	if auditCount != 0 {
		t.Errorf("expected old audit_log to be purged, got %d", auditCount)
	}
}

func TestPG_Retention_LogRun(t *testing.T) {
	skipIfNoDBRetention(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()

	policy := DefaultRetentionPolicy()
	pm := NewPartitionManager(s.Pool(), policy, zerolog.Nop())
	ctx := context.Background()

	pm.logRun(ctx, "test-phase", "test message", map[string]interface{}{"key": "value"})

	// Verify the log was inserted
	var phase, msg string
	if err := s.pool.QueryRow(ctx,
		"SELECT phase, message FROM retention_run_log WHERE phase = 'test-phase' ORDER BY run_at DESC LIMIT 1",
	).Scan(&phase, &msg); err != nil {
		t.Fatalf("check retention_run_log: %v", err)
	}
	if phase != "test-phase" {
		t.Errorf("expected phase 'test-phase', got %q", phase)
	}
	if msg != "test message" {
		t.Errorf("expected message 'test message', got %q", msg)
	}
}

func TestPG_Retention_AnalyzeRequestLogs(t *testing.T) {
	skipIfNoDBRetention(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()

	policy := DefaultRetentionPolicy()
	pm := NewPartitionManager(s.Pool(), policy, zerolog.Nop())
	ctx := context.Background()

	if err := pm.analyzeRequestLogs(ctx); err != nil {
		t.Fatalf("analyzeRequestLogs: %v", err)
	}
}
