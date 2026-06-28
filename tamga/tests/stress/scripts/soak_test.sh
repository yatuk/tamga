#!/bin/bash
# Soak test: constant 100 RPS for specified duration, capture memory snapshots
DURATION_MIN=${1:-15}
TARGET_RPS=${2:-100}
BASE_URL=${TAMGA_BASE_URL:-http://localhost:8443}
API_KEY=${TAMGA_API_KEY:-test-key}
RESULT_DIR="tests/stress/results"

mkdir -p "$RESULT_DIR"

echo "=== SOAK TEST ==="
echo "Duration: ${DURATION_MIN}min | RPS: $TARGET_RPS"
echo "Start: $(date -Iseconds)"

# Baseline memory
BASELINE=$(curl -s http://localhost:6060/debug/pprof/heap 2>/dev/null | wc -c)
echo "Baseline heap profile: ${BASELINE} bytes"

# Run k6 in background
k6 run --duration "${DURATION_MIN}m" --rps "$TARGET_RPS" tests/stress/k6/soak_constant.js \
  -e TAMGA_BASE_URL="$BASE_URL" -e TAMGA_API_KEY="$API_KEY" \
  --summary-export="$RESULT_DIR/soak_summary.json" 2>&1 &
K6_PID=$!

# Periodic snapshots
INTERVAL=$((DURATION_MIN * 60 / 6))
[ $INTERVAL -lt 60 ] && INTERVAL=60

for i in $(seq 1 $((DURATION_MIN * 60 / INTERVAL))); do
  sleep $INTERVAL
  TS=$(date +%s)
  MEM=$(powershell -Command "Get-Process tamga -ErrorAction SilentlyContinue | Select-Object -ExpandProperty WorkingSet64 2>/dev/null" 2>/dev/null || echo "0")
  MEM_MB=$((MEM / 1024 / 1024))
  echo "$TS: ${MEM_MB}MB" >> "$RESULT_DIR/soak_memory_timeline.txt"
  echo "Snapshot $i: ${MEM_MB}MB"
done

wait $K6_PID 2>/dev/null

# Final heap
FINAL=$(curl -s http://localhost:6060/debug/pprof/heap 2>/dev/null | wc -c)
echo "Final heap profile: ${FINAL} bytes"
echo "Delta: $((FINAL - BASELINE)) bytes"

# Analysis
python -c "
import json
try:
    with open('$RESULT_DIR/soak_summary.json') as f:
        s = json.load(f)
    metrics = s.get('metrics', {})
    print(f'Requests: {metrics.get(\"http_reqs\",{}).get(\"count\",\"?\")}')
    print(f'Failed: {metrics.get(\"http_req_failed\",{}).get(\"value\",\"?\")}')
    dur = metrics.get('http_req_duration', {})
    print(f'p95: {dur.get(\"p(95)\",\"?\")}ms')
except: pass
" 2>/dev/null

echo "Soak test complete. Results in $RESULT_DIR/"
