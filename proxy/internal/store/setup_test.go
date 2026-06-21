package store

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testDBURL is set by TestMain when a Docker PostgreSQL container is available.
var testDBURL string

func TestMain(m *testing.M) {
	// If TAMGA_TEST_DB_URL is explicitly set, use it directly (CI / manual mode).
	if url := os.Getenv("TAMGA_TEST_DB_URL"); url != "" {
		testDBURL = url
		os.Exit(m.Run())
	}

	// No env var — try Docker auto-start for local development.
	if _, err := exec.LookPath("docker"); err != nil {
		// Docker not available, run tests anyway (PG tests will skip gracefully).
		os.Exit(m.Run())
	}

	containerName := "tamga-store-test-pg"
	// Kill any leftover container from a previous run.
	_ = exec.Command("docker", "rm", "-f", containerName).Run()

	startCmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"-e", "POSTGRES_PASSWORD=test",
		"-e", "POSTGRES_DB=test",
		"-p", "5432",
		"postgres:16-alpine",
	)
	out, err := startCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "store: failed to start Docker PostgreSQL: %v: %s\n", err, out)
		os.Exit(m.Run())
	}

	// Get the dynamically assigned host port.
	portCmd := exec.Command("docker", "inspect", "-f",
		"{{(index (index .NetworkSettings.Ports \"5432/tcp\") 0).HostPort}}", containerName)
	portOut, err := portCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "store: failed to inspect container port: %v\n", err)
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
		os.Exit(m.Run())
	}
	port := string(portOut)
	port = port[:len(port)-1] // trim newline

	testDBURL = fmt.Sprintf("postgres://postgres:test@localhost:%s/test?sslmode=disable", port)
	_ = os.Setenv("TAMGA_TEST_DB_URL", testDBURL)

	// Wait for PostgreSQL to accept connections.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ready := false
	for i := 0; i < 30; i++ {
		pool, err := pgxpool.New(ctx, testDBURL)
		if err == nil {
			if pool.Ping(ctx) == nil {
				pool.Close()
				ready = true
				break
			}
			pool.Close()
		}
		time.Sleep(1 * time.Second)
	}
	if !ready {
		fmt.Fprintf(os.Stderr, "store: PostgreSQL container did not become ready within 30s\n")
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
		_ = os.Unsetenv("TAMGA_TEST_DB_URL")
		os.Exit(m.Run())
	}

	// Run all tests.
	code := m.Run()

	// Cleanup: kill the shared container.
	_ = exec.Command("docker", "rm", "-f", containerName).Run()
	_ = os.Unsetenv("TAMGA_TEST_DB_URL")

	os.Exit(code)
}
