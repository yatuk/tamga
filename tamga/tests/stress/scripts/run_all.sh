#!/bin/bash
# Run all stress tests sequentially
set -e
BASE=${TAMGA_BASE_URL:-http://localhost:8443}
KEY=${TAMGA_API_KEY:-test-key}

echo "========================================="
echo "TAMGA FULL STRESS TEST SUITE"
echo "Target: $BASE"
echo "========================================="

echo ""
echo "=== PHASE 1: Load Testing ==="
for rps in 100 250 500 1000; do
  echo "--- Baseline $rps RPS ---"
  k6 run tests/stress/k6/baseline_${rps}rps.js -e TAMGA_BASE_URL="$BASE" -e TAMGA_API_KEY="$KEY" --quiet 2>&1 | tail -3
done

echo ""
echo "=== PHASE 2: Adversarial Testing ==="
for test in pii_bypass injection_bypass secret_bypass policy_bypass; do
  echo "--- $test ---"
  PYTHONIOENCODING=utf-8 python tests/stress/adversarial/${test}.py 2>&1 | grep -E "RESULTS|BYPASS RATE"
done

echo ""
echo "=== PHASE 3: Chaos Testing ==="
bash tests/stress/chaos/kill_postgres.sh 2>&1 | tail -5
bash tests/stress/chaos/kill_redis.sh 2>&1 | tail -5

echo ""
echo "========================================="
echo "ALL TESTS COMPLETE"
echo "Results: tests/stress/results/"
echo "Reports: tests/stress/reports/"
echo "========================================="
