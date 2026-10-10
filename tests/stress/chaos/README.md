# Chaos scripts

Each script stops or isolates one dependency of the proxy in a local Docker
Compose stack and prints what the proxy reports. They are for a development
stack only.

The scripts print; they do not assert. None of them exits non-zero because
the proxy misbehaved, so read the output against the "Expected output"
sections below.

## Prerequisites

- Docker with Docker Compose, and the stack from `deploy/docker-compose.yml`
  running, started from the repository root:

  ```bash
  docker compose --env-file .env -f deploy/docker-compose.yml up -d
  ```

- Run every script from the repository root. Compose names the project
  `deploy` after the directory of the compose file, which is where the
  hard-coded names `deploy-postgres-1`, `deploy-redis-1` and
  `deploy_internal` come from.
- `TAMGA_ADMIN_KEY` exported in the shell, set to the key the proxy was
  started with:

  ```bash
  export TAMGA_ADMIN_KEY=...   # the value in your .env
  ```

  `kill_analyzer.sh`, `kill_postgres.sh`, `kill_redis.sh` and
  `network_partition.sh` read `GET /api/v1/health/detailed` and send the key
  in the `X-Tamga-Admin-Key` header. Without a key that endpoint returns only
  `proxy`, `status` and scan latency, not `database`, `redis`, `analyzer` or
  `scanner_count`. These four scripts stop with
  `set TAMGA_ADMIN_KEY: the health detail needs it` when the variable is unset.
- `curl`, and `python` (Python 3) on the `PATH`: the scripts use it to read
  the JSON.
- A shell with `bash`. On Windows use WSL2 or Git Bash.
- `kill_nats.sh` and `kill_scanner_service.sh` call
  `docker compose -f deploy/docker-compose.yml` without `--env-file`. Export
  the variables of your `.env` first (for example `set -a; . ./.env; set +a`),
  otherwise Compose substitutes empty values when it brings the service back.

## What the health endpoint reports

With the key, `GET /api/v1/health/detailed` returns among others:

| Field | Values |
|---|---|
| `status` | `ok`, or `degraded` when the database, Redis or the analyzer is down |
| `database` | `connected`, `disconnected`, `not_configured` |
| `redis` | `connected`, `disconnected`, `not_configured` |
| `analyzer` | `reachable`, `unreachable`, `not_configured` |
| `scanner_count` | number of registered scanners |

The HTTP status is 503 while `status` is `degraded`. The scripts call `curl`
without `-f`, so they print the field in either case.

`GET /health` is the liveness check. It needs no key and does not look at any
dependency.

## The requests the scripts send

Several scripts send `POST http://localhost:8443/v1/messages` with the header
`x-api-key: test-key` and print the HTTP status and the time taken. The
status is whatever the proxy answered for that request and that key; the
scripts do not check it. Compare the line printed during the outage with the
same request sent before it.

## Scripts

### kill_analyzer.sh

Does not stop anything. It prints `scanner_count` and sends three requests
that contain an email address. Stop the analyzer yourself first to see the
proxy without it:

```bash
docker compose --env-file .env -f deploy/docker-compose.yml stop analyzer
bash tests/stress/chaos/kill_analyzer.sh
docker compose --env-file .env -f deploy/docker-compose.yml start analyzer
```

Expected output:

```
=== CHAOS: Kill Analyzer ===
Before:
Scanners: <number>
Sending PII requests with analyzer down...
Request 1: HTTP <code> | <seconds>s
Request 2: HTTP <code> | <seconds>s
Request 3: HTTP <code> | <seconds>s
```

While the analyzer is stopped, `/api/v1/health/detailed` reports
`analyzer: "unreachable"` and answers 503.

### kill_postgres.sh

Stops `deploy-postgres-1`, prints the database status before, during and
after, sends three requests during the outage, then starts the container
again.

```bash
bash tests/stress/chaos/kill_postgres.sh
```

Expected output:

```
Before:
DB: connected
After PostgreSQL kill:
DB: disconnected
Test requests during DB outage:
Request 1: HTTP <code> | <seconds>s
...
Recovery:
DB: connected
```

The last line is read three seconds after `docker start`. If it still says
`disconnected`, query the endpoint again a few seconds later.

### kill_redis.sh

Stops `deploy-redis-1`, prints the Redis status before, during and after,
sends one request during the outage and prints how long it took, then starts
the container again.

```bash
bash tests/stress/chaos/kill_redis.sh
```

Expected output:

```
Before:
Redis: connected
After Redis kill:
Redis: disconnected
Test request during Redis outage:
HTTP <code> | <seconds>s
Request took <n> seconds wall-clock
Recovery:
Redis: connected
```

If the proxy runs without `REDIS_URL`, every line says `not_configured` and
the script tests nothing.

### network_partition.sh

Disconnects `deploy-postgres-1` from the Docker network, waits 30 seconds,
and reconnects it. The container keeps running, so this exercises connection
timeouts rather than a refused connection.

```bash
bash tests/stress/chaos/network_partition.sh            # network deploy_internal
bash tests/stress/chaos/network_partition.sh my_network
```

Expected output:

```
DB: disconnected, Proxy: up
Waiting 30s...
Reconnecting...
DB: connected, Proxy: up
```

### kill_nats.sh

Kills the `nats` service, waits (30 seconds, or the first argument), checks
`GET /health`, and brings NATS back with `up -d`.

```bash
bash tests/stress/chaos/kill_nats.sh
bash tests/stress/chaos/kill_nats.sh 60
```

Expected output: a line ending in
`PASS: proxy healthy after NATS kill (in-memory fallback)`, then
`NATS restarted`. The check is the liveness endpoint only: it shows that the
process still answers, not that events were delivered. During the outage the
proxy logs `nats publish failed` for each event it could not publish.

### kill_scanner_service.sh

Kills the `scanner-service` service, waits (30 seconds, or the second
argument), checks `GET /health`, and brings the service back.

```bash
bash tests/stress/chaos/kill_scanner_service.sh
bash tests/stress/chaos/kill_scanner_service.sh ignored 45
```

The first argument is only printed; the script always kills the Compose
service `scanner-service`.

The proxy uses the scanner service only when `TAMGA_SCANNER_SERVICE_ADDR` is
set (for example `scanner-service:50052`). Without it the proxy scans
in-process and this script tests nothing. With it, a failed gRPC call falls
back to in-process scanning.

Expected output: a line ending in
`PASS: proxy healthy after scanner-service kill`, then
`scanner-service restarted`. As with NATS, the check is the liveness endpoint
only.

## Notes

- Run the scripts one at a time.
- `kill_postgres.sh`, `kill_redis.sh` and `network_partition.sh` restore what
  they changed only if they run to the end. If one is interrupted, restore the
  stack:

  ```bash
  docker compose --env-file .env -f deploy/docker-compose.yml up -d
  ```

- Follow the proxy log while a script runs:

  ```bash
  docker compose --env-file .env -f deploy/docker-compose.yml logs -f proxy
  ```
