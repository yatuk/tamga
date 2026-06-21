#!/bin/bash
# Chaos: Kill Redis and observe fail-open behavior
echo "=== CHAOS: Kill Redis ==="
echo "Before:"
curl -s http://localhost:8443/api/v1/health/detailed -H "x-api-key: test-key" | python -c "import sys,json; d=json.load(sys.stdin); print(f'Redis: {d[\"redis\"]}')"
docker stop deploy-redis-1
sleep 3
echo "After Redis kill:"
curl -s http://localhost:8443/api/v1/health/detailed -H "x-api-key: test-key" | python -c "import sys,json; d=json.load(sys.stdin); print(f'Redis: {d[\"redis\"]}')"
echo "Test request during Redis outage:"
START=$(date +%s)
curl -s -o /dev/null -w "HTTP %{http_code} | %{time_total}s\n" http://localhost:8443/v1/messages -H "x-api-key: test-key" -H "Content-Type: application/json" -d '{"model":"claude-3-haiku-20240307","messages":[{"role":"user","content":"Hello"}]}'
END=$(date +%s)
echo "Request took $((END-START)) seconds wall-clock"
echo "Recovery:"
REDIS_PASSWORD=test docker start deploy-redis-1
sleep 2
curl -s http://localhost:8443/api/v1/health/detailed -H "x-api-key: test-key" | python -c "import sys,json; d=json.load(sys.stdin); print(f'Redis: {d[\"redis\"]}')"
echo "Redis chaos complete."
