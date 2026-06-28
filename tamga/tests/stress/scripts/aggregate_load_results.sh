#!/bin/bash
# Aggregate all load test JSON results into a single summary
echo "=== LOAD TEST AGGREGATE ==="
for f in tests/stress/results/load_*.json tests/stress/results/soak_*.json; do
  [ -f "$f" ] && python -c "
import json
with open('$f') as fp:
    d = json.load(fp)
print(f'  {d.get(\"test\",\"?\"):30s} | {d.get(\"results\",{}).get(\"max_sustained_rps\",\"?\")} RPS')
" 2>/dev/null
done
echo "Aggregation complete."
