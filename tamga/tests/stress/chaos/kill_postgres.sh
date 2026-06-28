#!/bin/bash
# Chaos: Kill PostgreSQL and observe fail-open behavior
echo "=== CHAOS: Kill PostgreSQL ==="
echo "Before:"
curl -s http://localhost:8443/api/v1/health/detailed -H "x-api-key: test-key" | python -c "import sys,json; d=json.load(sys.stdin); print(f'DB: {d[\"database\"]}')"
docker stop deploy-postgres-1
sleep 3
echo "After PostgreSQL kill:"
curl -s http://localhost:8443/api/v1/health/detailed -H "x-api-key: test-key" | python -c "import sys,json; d=json.load(sys.stdin); print(f'DB: {d[\"database\"]}')"
echo "Test requests during DB outage:"
for i in 1 2 3; do
  curl -s -o /dev/null -w "Request $i: HTTP %{http_code} | %{time_total}s\n" http://localhost:8443/v1/messages -H "x-api-key: test-key" -H "Content-Type: application/json" -d "{\"model\":\"claude-3-haiku-20240307\",\"messages\":[{\"role\":\"user\",\"content\":\"Test $i\"}]}"
done
echo "Recovery:"
POSTGRES_PASSWORD=test docker start deploy-postgres-1
sleep 3
curl -s http://localhost:8443/api/v1/health/detailed -H "x-api-key: test-key" | python -c "import sys,json; d=json.load(sys.stdin); print(f'DB: {d[\"database\"]}')"
echo "PostgreSQL chaos complete."
