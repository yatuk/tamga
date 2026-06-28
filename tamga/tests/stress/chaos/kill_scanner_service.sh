#!/bin/bash
# Chaos: kill the scanner-service container and verify proxy fail-open.
set -euo pipefail

CONTAINER="${1:-tamga-scanner-service-1}"
WAIT="${2:-30}"

echo "[$(date -Iseconds)] Killing scanner-service container: $CONTAINER"
docker compose -f deploy/docker-compose.yml kill scanner-service || true
echo "Waiting ${WAIT}s for proxy to detect and recover..."
sleep "$WAIT"

# Verify proxy is still serving (fail-open to local registry).
if curl -sf http://localhost:8443/health > /dev/null; then
    echo "[$(date -Iseconds)] PASS: proxy healthy after scanner-service kill"
else
    echo "[$(date -Iseconds)] WARN: proxy health check failed"
fi

# Restart scanner-service.
docker compose -f deploy/docker-compose.yml up -d scanner-service
echo "[$(date -Iseconds)] scanner-service restarted"
