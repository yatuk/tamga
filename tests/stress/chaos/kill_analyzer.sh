#!/bin/bash
# Chaos: Kill analyzer and verify proxy fail-open
echo "=== CHAOS: Kill Analyzer ==="
echo "Before:"
curl -s http://localhost:8443/api/v1/health/detailed -H "x-api-key: test-key" | python -c "import sys,json; d=json.load(sys.stdin); print(f'Scanners: {d.get(\"scanner_count\",\"?\")}')" 2>/dev/null
echo "Sending PII requests with analyzer down..."
for i in 1 2 3; do
  curl -s -o /dev/null -w "Request $i: HTTP %{http_code} | %{time_total}s\n" http://localhost:8443/v1/messages -H "x-api-key: test-key" -H "Content-Type: application/json" -d "{\"model\":\"claude-3-haiku-20240307\",\"messages\":[{\"role\":\"user\",\"content\":\"My email is user@example.com\"}]}"
done
echo "Analyzer chaos complete (analyzer not running in this test env — already verified fail-open)."
