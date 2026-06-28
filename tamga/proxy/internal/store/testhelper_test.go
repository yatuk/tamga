package store

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// ensureTestPostgres starts a temporary Postgres container and returns its URL.
// On CI, use the service container approach (TAMGA_TEST_DB_URL env var).
// For local dev, this tries Docker. Falls back to skip if Docker is unavailable.
func ensureTestPostgres(t *testing.T) string {
	t.Helper()
	if url := os.Getenv("TAMGA_TEST_DB_URL"); url != "" {
		return url
	}
	// Try starting a Docker Postgres container
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker not available and TAMGA_TEST_DB_URL not set; skipping Postgres integration test")
	}
	containerName := fmt.Sprintf("tamga-test-pg-%d", time.Now().UnixNano()%100000)
	startCmd := exec.Command("docker", "run", "--rm", "-d",
		"--name", containerName,
		"-e", "POSTGRES_PASSWORD=test",
		"-e", "POSTGRES_DB=test",
		"-p", "5432",
		"postgres:16-alpine",
	)
	out, err := startCmd.CombinedOutput()
	if err != nil {
		t.Skipf("failed to start postgres container: %v: %s", err, out)
	}
	containerID := string(out)
	containerID = containerID[:len(containerID)-1] // trim newline

	t.Cleanup(func() {
		killCmd := exec.Command("docker", "kill", containerID)
		_ = killCmd.Run()
	})

	// Get the mapped port
	portCmd := exec.Command("docker", "inspect", "-f", "{{(index (index .NetworkSettings.Ports \"5432/tcp\") 0).HostPort}}", containerID)
	portOut, err := portCmd.CombinedOutput()
	if err != nil {
		t.Skipf("failed to inspect postgres container: %v: %s", err, portOut)
	}
	port := string(portOut)
	port = port[:len(port)-1]

	url := fmt.Sprintf("postgres://postgres:test@localhost:%s/test?sslmode=disable", port)
	_ = os.Setenv("TAMGA_TEST_DB_URL", url)

	// Wait for Postgres to be ready
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for i := 0; i < 30; i++ {
		pool, err := pgxpool.New(ctx, url)
		if err == nil {
			_ = pool.Ping(ctx)
			pool.Close()
			return url
		}
		time.Sleep(1 * time.Second)
	}
	t.Skip("Postgres container did not become ready within 30s")
	return ""
}

// newTestPostgresStore creates a PostgresStore connected to the test database
// and applies the minimal schema needed for tests. Uses NewPostgresStore so the
// flush loop is running. Caller is responsible for cleanup via t.Cleanup.
func newTestPostgresStore(t *testing.T) *PostgresStore {
	t.Helper()
	url := ensureTestPostgres(t)

	// Apply minimal schema before creating the store
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("failed to connect to test postgres: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS request_logs (
			id UUID NOT NULL DEFAULT gen_random_uuid(),
			org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
			request_id TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			model TEXT DEFAULT '',
			endpoint TEXT DEFAULT '',
			input_tokens INT DEFAULT 0,
			output_tokens INT DEFAULT 0,
			findings JSONB NOT NULL DEFAULT '[]',
			findings_count INT NOT NULL DEFAULT 0,
			action_taken TEXT NOT NULL DEFAULT 'pass',
			scan_latency_ms REAL DEFAULT 0,
			total_latency_ms REAL DEFAULT 0,
			user_identifier TEXT DEFAULT '',
			cost_usd NUMERIC(12, 6) DEFAULT 0,
			model_family TEXT DEFAULT '',
			output_action TEXT DEFAULT '',
			output_findings JSONB DEFAULT '[]',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (id, created_at)
		) PARTITION BY RANGE (created_at)
	`); err != nil {
		pool.Close()
		t.Fatalf("failed to create request_logs table: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS request_logs_default PARTITION OF request_logs DEFAULT
	`); err != nil {
		pool.Close()
		t.Fatalf("failed to create default partition: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS daily_stats (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001',
			stat_date DATE NOT NULL,
			total_requests INT DEFAULT 0,
			blocked_requests INT DEFAULT 0,
			redacted_requests INT DEFAULT 0,
			warned_requests INT DEFAULT 0,
			total_input_tokens BIGINT DEFAULT 0,
			total_output_tokens BIGINT DEFAULT 0
		)
	`); err != nil {
		pool.Close()
		t.Fatalf("failed to create daily_stats table: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS model_pricing (
			id SERIAL PRIMARY KEY,
			provider TEXT NOT NULL,
			model_family TEXT NOT NULL,
			model_version TEXT NOT NULL,
			input_per_1k NUMERIC(10, 6) NOT NULL,
			output_per_1k NUMERIC(10, 6) NOT NULL,
			currency CHAR(3) NOT NULL DEFAULT 'USD',
			effective_from TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			effective_to TIMESTAMPTZ,
			source TEXT,
			notes TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		pool.Close()
		t.Fatalf("failed to create model_pricing table: %v", err)
	}
	pool.Close()

	// Use the real constructor so flushLoop, done channel, etc. are set up.
	log := zerolog.New(zerolog.NewTestWriter(t))
	s, err := NewPostgresStore(context.Background(), url, log)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
