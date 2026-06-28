package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// NewTestPostgres starts a testcontainers PostgreSQL container, runs all
// migrations from deploy/migrations/ in order, and returns a connected
// *pgxpool.Pool. The container and pool are cleaned up via t.Cleanup.
func NewTestPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("test"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := pgContainer.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}

	// Run all migrations from the deploy/migrations directory.
	runMigrations(t, pool)

	// Migration 011 enables Row Level Security with a policy that requires
	// app.tenant_id to be set. Disable RLS for tests so queries work without
	// session-level tenant configuration.
	for _, tbl := range []string{"request_logs", "daily_stats"} {
		if _, err := pool.Exec(ctx, "ALTER TABLE IF EXISTS "+tbl+" DISABLE ROW LEVEL SECURITY"); err != nil {
			t.Logf("disable RLS on %s: %v (table may not exist)", tbl, err)
		}
	}

	// Seed default org if not present (needed for FK references in other tables).
	_, _ = pool.Exec(ctx, `INSERT INTO organizations (name, slug, plan) VALUES ('Default Org', 'default', 'trial') ON CONFLICT DO NOTHING`)

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// NewTestPostgresStore creates a PostgresStore backed by the testcontainers
// PostgreSQL instance. The pool is obtained via NewTestPostgres and its
// lifecycle is managed by t.Cleanup. The store's Close() flushes the buffer
// and closes the pool; a second close from t.Cleanup is a no-op.
func NewTestPostgresStore(t *testing.T) *PostgresStore {
	t.Helper()
	pool := NewTestPostgres(t)

	s := &PostgresStore{
		pool: pool,
		log:  zerolog.Nop(),
		done: make(chan struct{}),
	}
	s.wg.Add(1)
	go s.flushLoop()

	t.Cleanup(func() {
		_ = s.Close()
	})

	return s
}

// runMigrations reads all *.up.sql files from the migrations directory sorted
// by filename and executes each one against the given pool.
func runMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	migrationsDir, err := findMigrationsDir()
	if err != nil {
		t.Fatalf("find migrations dir: %v", err)
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir %s: %v", migrationsDir, err)
	}

	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	if len(upFiles) == 0 {
		t.Fatalf("no .up.sql files found in %s", migrationsDir)
	}

	ctx := context.Background()
	for _, f := range upFiles {
		path := filepath.Join(migrationsDir, f)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", f, err)
		}
		sql := string(content)
		if strings.TrimSpace(sql) == "" {
			continue
		}
		// pgx Exec runs in auto-commit mode, so CREATE INDEX CONCURRENTLY
		// (migration 002) works correctly.
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("run migration %s: %v", f, err)
		}
		t.Logf("applied migration: %s", f)
	}
}

// findMigrationsDir locates the deploy/migrations directory.
func findMigrationsDir() (string, error) {
	// Try relative paths from proxy/internal/store/.
	candidates := []string{
		"../../deploy/migrations", // from proxy/internal/store/
		"../deploy/migrations",    // from proxy/
		"deploy/migrations",       // from project root
	}

	// Also try using TAMGA_PROJECT_ROOT if set.
	if root := os.Getenv("TAMGA_PROJECT_ROOT"); root != "" {
		candidates = append([]string{filepath.Join(root, "deploy", "migrations")}, candidates...)
	}

	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			// Verify it contains migration files.
			entries, err := os.ReadDir(abs)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".up.sql") {
					return abs, nil
				}
			}
		}
	}

	return "", fmt.Errorf("migrations directory not found, tried: %v; set TAMGA_PROJECT_ROOT=<project root>", candidates)
}
