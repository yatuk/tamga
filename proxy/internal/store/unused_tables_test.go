package store

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// droppedTables are the tables migration 019 removes.
var droppedTables = []string{"api_keys", "policies", "alerts", "daily_stats", "event_outbox"}

// No SQL in the proxy names a table that 019 drops. The check reads the
// source, so it also covers queries no test runs.
func TestNoQueryUsesDroppedTables(t *testing.T) {
	re := regexp.MustCompile(`(?i)\b(from|into|update|join|table|on|truncate)\s+(only\s+)?(if\s+(not\s+)?exists\s+)?(public\.)?"?(` +
		strings.Join(droppedTables, "|") + `)\b`)
	root := filepath.Join("..", "..") // the proxy module
	checked := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "testdata" || name == "bin" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		checked++
		for i, line := range strings.Split(string(b), "\n") {
			if m := re.FindString(line); m != "" {
				t.Errorf("%s:%d refers to a table dropped by migration 019: %q", path, i+1, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 50 {
		t.Fatalf("only %d source files were read; the walk did not find the proxy module", checked)
	}
}

// tableShape describes the given tables as text: every column, constraint
// and index, and whether row-level security is on. Two databases in which a
// table is the same give the same lines.
func tableShape(t *testing.T, pool *pgxpool.Pool, tables []string) []string {
	t.Helper()
	ctx := context.Background()
	var out []string
	for _, q := range []string{
		`SELECT 'column ' || table_name || '.' || column_name || ' ' || data_type || ' ' || udt_name ||
		        ' null=' || is_nullable || ' default=' || COALESCE(column_default, '-') || ' pos=' || ordinal_position
		   FROM information_schema.columns
		  WHERE table_schema = 'public' AND table_name = ANY($1)`,
		`SELECT 'constraint ' || conrelid::regclass::text || ' ' || conname || ' ' || pg_get_constraintdef(oid)
		   FROM pg_constraint
		  WHERE conrelid::regclass::text = ANY($1) OR confrelid::regclass::text = ANY($1)`,
		`SELECT 'index ' || indexdef FROM pg_indexes WHERE schemaname = 'public' AND tablename = ANY($1)`,
		`SELECT 'rls ' || relname || ' ' || relrowsecurity::text || ' ' || relforcerowsecurity::text
		   FROM pg_class WHERE relnamespace = 'public'::regnamespace AND relkind = 'r' AND relname = ANY($1)`,
		`SELECT 'policy ' || tablename || ' ' || policyname FROM pg_policies WHERE tablename = ANY($1)`,
	} {
		rows, err := pool.Query(ctx, q, tables)
		if err != nil {
			t.Fatalf("%v\nquery: %s", err, q)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	sort.Strings(out)
	return out
}

func existingTables(t *testing.T, pool *pgxpool.Pool, tables []string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT relname::text FROM pg_class
		  WHERE relnamespace = 'public'::regnamespace AND relkind = 'r' AND relname = ANY($1) ORDER BY 1`, tables)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

// migrationFile is one migration as a single string.
func migrationFile(t *testing.T, name string) string {
	t.Helper()
	dir, err := findMigrationsDir()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// schemaBefore019 builds a second database in the same server with the
// migrations up to 018 only, which is the schema 019's down has to restore.
func schemaBefore019(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE DATABASE before_019`); err != nil {
		t.Fatalf("create database: %v", err)
	}
	cfg := pool.Config().Copy()
	cfg.ConnConfig.Database = "before_019"
	old, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect to before_019: %v", err)
	}
	t.Cleanup(old.Close)

	dir, err := findMigrationsDir()
	if err != nil {
		t.Fatal(err)
	}
	names, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	applied := 0
	for _, path := range names {
		name := filepath.Base(path)
		if name >= "019" {
			continue
		}
		for _, stmt := range splitPlainStatementsOrWhole(migrationFile(t, name)) {
			if _, err := old.Exec(ctx, stmt); err != nil {
				t.Fatalf("migration %s on before_019: %v", name, err)
			}
		}
		applied++
	}
	if applied != 18 {
		t.Fatalf("applied %d migrations before 019, want 18", applied)
	}
	return old
}

// splitPlainStatementsOrWhole runs a migration the way runMigrations does:
// statement by statement when it builds an index CONCURRENTLY, whole
// otherwise.
func splitPlainStatementsOrWhole(sql string) []string {
	if strings.Contains(strings.ToUpper(sql), "CONCURRENTLY") {
		return splitPlainStatements(sql)
	}
	return []string{sql}
}

// Migration 019 on a real database: the tables are gone after the
// migrations, the retention job runs without them, the migration can be run
// again, and its down brings the tables back as the earlier migrations made
// them.
func TestMigration019_DropUnusedTables(t *testing.T) {
	s := NewTestPostgresStore(t)
	ctx := context.Background()
	up := migrationFile(t, "019_drop_unused_tables.up.sql")
	down := migrationFile(t, "019_drop_unused_tables.down.sql")

	retention := func(t *testing.T) {
		t.Helper()
		policy := DefaultRetentionPolicy()
		policy.IncidentClosedDays = 30
		pm := NewPartitionManager(s.Pool(), policy, zerolog.Nop())
		if err := pm.RunMaintenanceCycle(ctx); err != nil {
			t.Fatalf("the retention job failed: %v", err)
		}
		// The job writes each step to retention_run_log; a step that hit a
		// missing table would be there with the error as its message.
		rows, err := s.pool.Query(ctx, `SELECT phase, COALESCE(message, '') FROM retention_run_log`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		steps := 0
		for rows.Next() {
			var phase, msg string
			if err := rows.Scan(&phase, &msg); err != nil {
				t.Fatal(err)
			}
			steps++
			if msg != "ok" {
				t.Errorf("retention step %s: %s", phase, msg)
			}
		}
		if steps == 0 {
			t.Error("the retention job logged no steps")
		}
	}

	t.Run("the tables are gone after the migrations", func(t *testing.T) {
		if got := existingTables(t, s.pool, droppedTables); len(got) != 0 {
			t.Fatalf("still there after 019: %v", got)
		}
		// What the proxy does use is untouched.
		kept := []string{"organizations", "request_logs_default", "audit_log", "virtual_keys", "policy_revisions",
			"policy_proposals", "retention_run_log", "incident_lifecycle", "stored_documents"}
		if got := existingTables(t, s.pool, kept); len(got) != len(kept) {
			t.Fatalf("tables in use after 019: %v, want %v", got, kept)
		}
	})

	t.Run("the retention job runs without them", retention)

	t.Run("019 can be run again", func(t *testing.T) {
		if _, err := s.pool.Exec(ctx, up); err != nil {
			t.Fatalf("second run of 019: %v", err)
		}
	})

	t.Run("down restores the tables as they were, and up drops them again", func(t *testing.T) {
		want := tableShape(t, schemaBefore019(t, s.pool), droppedTables)
		if len(want) < 40 {
			t.Fatalf("the schema before 019 describes as %d lines; the comparison would prove nothing", len(want))
		}

		if _, err := s.pool.Exec(ctx, down); err != nil {
			t.Fatalf("019 down: %v", err)
		}
		if _, err := s.pool.Exec(ctx, down); err != nil {
			t.Fatalf("second run of 019 down: %v", err)
		}
		got := tableShape(t, s.pool, droppedTables)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("019 down does not restore the schema of 001, 002 and 010.\n--- restored\n%s\n--- original\n%s",
				strings.Join(got, "\n"), strings.Join(want, "\n"))
		}

		// A row someone put there is what the migration's header warns
		// about: up drops the table with it.
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO alerts (org_id, request_id, severity, finding_type, message) VALUES ($1, 'r', 'high', 'pii', 'm')`,
			testOrgID); err != nil {
			t.Fatalf("insert into the restored alerts: %v", err)
		}
		if _, err := s.pool.Exec(ctx, up); err != nil {
			t.Fatalf("019 up after down: %v", err)
		}
		if got := existingTables(t, s.pool, droppedTables); len(got) != 0 {
			t.Fatalf("still there after 019 was run again: %v", got)
		}
		var orgs int
		if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM organizations`).Scan(&orgs); err != nil || orgs == 0 {
			t.Fatalf("organizations after 019: %d rows, err %v", orgs, err)
		}
	})

	t.Run("the retention job still runs", func(t *testing.T) {
		if _, err := s.pool.Exec(ctx, `DELETE FROM retention_run_log`); err != nil {
			t.Fatal(err)
		}
		retention(t)
	})
}
