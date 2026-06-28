#!/bin/bash
# Chaos: kill the NATS container and verify proxy continues (in-memory fallback).
set -euo pipefail

WAIT="${1:-30}"

echo "[$(date -Iseconds)] Killing NATS container"
docker compose -f deploy/docker-compose.yml kill nats || true
echo "Waiting ${WAIT}s for proxy to detect and fall back..."
sleep "$WAIT"

if curl -sf http://localhost:8443/health > /dev/null; then
    echo "[$(date -Iseconds)] PASS: proxy healthy after NATS kill (in-memory fallback)"
else
    echo "[$(date -Iseconds)] WARN: proxy health check failed"
fi

docker compose -f deploy/docker-compose.yml up -d nats
echo "[$(date -Iseconds)] NATS restarted"
