# Tamga Chaos Engineering Scripts

This directory contains chaos engineering scripts that test the Tamga proxy's
resilience against infrastructure component failures. Each script kills or
isolates a specific component and verifies the proxy continues to serve traffic
(fail-open), then restores the component and verifies recovery.

## Prerequisites

- **Docker** and **Docker Compose** installed and running.
- The Tamga stack deployed via `deploy/docker-compose.yml` from the repo root.
- `curl` available on the host.
- `python` available for JSON parsing in the health-check one-liners (Python 3).
- All commands must be run from the repository root (so `deploy/docker-compose.yml`
  resolves correctly).
- **Linux or macOS** host (the scripts use `docker` commands; on Windows, use
  WSL2 with Docker Desktop).

## Scripts

### 1. kill_analyzer.sh

**Purpose:** Kill the analyzer container and verify the proxy continues
processing requests in fail-open mode (without analyzer risk scoring).

**Expected behavior:**
- The `/health/detailed` endpoint reports `analyzer: "unreachable"` (HTTP 503 status).
- Proxy requests still succeed (HTTP 200), falling back to local DFA-only
  scanning with no analyzer enrichment.
- PII detection continues via the local scanner pool.

**How to run:**
```bash
bash tests/stress/chaos/kill_analyzer.sh
```

**Recovery:**
- The analyzer is not restarted by this script. To restore:
  ```bash
  docker compose -f deploy/docker-compose.yml up -d analyzer
  ```

**Verification:**
- Check `/health/detailed` for `analyzer: "reachable"` after restart.
- Send PII requests; confirm risk scores return in event detail.

---

### 2. kill_nats.sh

**Purpose:** Kill the NATS container and verify the proxy continues using the
in-memory event bus as a fallback without JetStream persistence.

**Arguments:**
- `$1` -- wait time in seconds (default: 30). Controls how long the script
  pauses before checking proxy health.

**Expected behavior:**
- Proxy health check passes (`curl -sf http://localhost:8443/health` returns 0).
- Events dispatched during the outage are handled by the in-memory bus
  (LogHandler, MetricsHandler, RecentBuffer, liveBroker).
- Events are NOT persisted to JetStream until NATS recovers.

**How to run:**
```bash
# Default 30-second wait
bash tests/stress/chaos/kill_nats.sh

# Custom wait time
bash tests/stress/chaos/kill_nats.sh 60
```

**Recovery:**
- The script automatically restarts NATS after the health check:
  ```bash
  docker compose -f deploy/docker-compose.yml up -d nats
  ```

**Verification:**
- Check `docker compose ps nats` -- status should be "running" after script completes.
- Watch proxy logs for `nats publish failed` during outage and
  `creating NATS stream` on reconnect.

---

### 3. kill_postgres.sh

**Purpose:** Kill the PostgreSQL container and verify the proxy continues
serving requests without database persistence (in-memory fallback).

**Expected behavior:**
- `/health/detailed` reports `database: "disconnected"` (HTTP 503 status).
- Proxy requests still succeed (HTTP 200), falling back to the in-memory
  RecentBuffer for event storage and in-memory counters for stats.
- Dashboard endpoints (`/stats`, `/events`) return in-memory data only.

**How to run:**
```bash
bash tests/stress/chaos/kill_postgres.sh
```

**Recovery:**
- The script automatically restarts PostgreSQL:
  ```bash
  POSTGRES_PASSWORD=test docker start deploy-postgres-1
  ```
- Waits 3 seconds, then re-checks `/health/detailed` for `database: "connected"`.

**Verification:**
- During outage: `/health/detailed` returns `database: "disconnected"`, proxy returns 200.
- After recovery: `/health/detailed` returns `database: "connected"`, proxy returns 200.
- Dashboard queries that rely on DB return real data again.

---

### 4. kill_redis.sh

**Purpose:** Kill the Redis container and verify the proxy continues serving
requests without Redis (rate limiting and caching disabled, fail-open).

**Expected behavior:**
- `/health/detailed` reports `redis: "disconnected"` (HTTP 503 status if Redis
  is the only unhealthy component, HTTP 503 overall).
- Proxy requests still succeed (HTTP 200) -- rate limiting is bypassed (no Redis
  means no token bucket state), caching misses are passed through to upstream.
- Request wall-clock time may increase if Redis was absorbing cache traffic.

**How to run:**
```bash
bash tests/stress/chaos/kill_redis.sh
```

**Recovery:**
- The script automatically restarts Redis:
  ```bash
  REDIS_PASSWORD=test docker start deploy-redis-1
  ```
- Waits 2 seconds, re-checks `/health/detailed` for `redis: "connected"`.

**Verification:**
- During outage: `/health/detailed` returns `redis: "disconnected"`, proxy returns 200.
- After recovery: `/health/detailed` returns `redis: "connected"`.
- Rate limiting resumes after Redis recovery (subsequent requests count toward
  rate limits again).

---

### 5. kill_scanner_service.sh

**Purpose:** Kill the scanner-service gRPC container and verify the proxy
falls back to its local scanner registry (in-process DFA scanners).

**Arguments:**
- `$1` -- container name (default: `tamga-scanner-service-1`).
- `$2` -- wait time in seconds (default: 30).

**Expected behavior:**
- Proxy health check passes (`curl -sf http://localhost:8443/health` returns 0).
- Scanning continues using the **local** scanner registry compiled into the
  proxy binary (Aho-Corasick DFA, custom entity scanners, competitor check).
- gRPC-based scanners (if any are external-only) are unavailable; the proxy
  transparently skips them and uses local equivalents.
- The `scanner_count` on `/health/detailed` reflects only in-process scanners.

**How to run:**
```bash
# Default settings
bash tests/stress/chaos/kill_scanner_service.sh

# Custom container name and wait
bash tests/stress/chaos/kill_scanner_service.sh my-scanner-svc 45
```

**Recovery:**
- The script automatically restarts the service:
  ```bash
  docker compose -f deploy/docker-compose.yml up -d scanner-service
  ```

**Verification:**
- Send scan requests during outage; confirm findings appear in events.
- After restart, gRPC-based scanners are available again.
- Check proxy logs for gRPC connection errors during outage and
  successful reconnection after restart.

---

### 6. network_partition.sh

**Purpose:** Disconnect PostgreSQL from the Docker network (simulating a
network partition) and verify the proxy handles the isolation gracefully,
then reconnect and verify recovery.

**Arguments:**
- `$1` -- Docker network name (default: `deploy_internal`).

**Expected behavior:**
- After disconnect: `/health/detailed` reports `database: "disconnected"`.
- Proxy continues serving on the in-memory path.
- After reconnect: `/health/detailed` reports `database: "connected"`.

**How to run:**
```bash
# Default network
bash tests/stress/chaos/network_partition.sh

# Custom network
bash tests/stress/chaos/network_partition.sh my_custom_network
```

**Recovery:**
- The script automatically reconnects PostgreSQL after a 30-second wait:
  ```bash
  docker network connect $NETWORK deploy-postgres-1
  ```

**Verification:**
- During partition: proxy returns 200 for API requests, DB shows disconnected.
- After reconnection: DB shows connected within a few seconds.
- This test differs from `kill_postgres.sh` because the container is still
  running but inaccessible -- exercises TCP connection timeout handling in pgx.

---

## General Notes

- All kill scripts are **safe for development environments** -- they target
  containers by name and restart them automatically (except `kill_analyzer.sh`
  which leaves the analyzer stopped for manual verification).
- The proxy is designed to **fail open**: when any backend component is
  unavailable, the proxy continues serving LLM traffic without blocking or
  crashing. This is a core architectural property validated by these tests.
- Run these scripts **sequentially**, not in parallel, to isolate each
  component's failure mode.
- Monitor proxy logs during chaos testing:
  ```bash
  docker compose -f deploy/docker-compose.yml logs -f proxy
  ```
- To reset all components to a known good state:
  ```bash
  docker compose -f deploy/docker-compose.yml up -d
  ```
