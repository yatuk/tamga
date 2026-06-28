#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Tamga Stress Test Suite Runner (Linux / macOS)
#
# Single-command orchestration:
#   docker compose up → wait healthy → adversarial tests → k6 load tests →
#   collect results → check regression → docker compose down (always)
#
# Usage:
#   ./run_stress_suite.sh                    # default: 100/500/1000 RPS
#   ./run_stress_suite.sh --skip-load        # adversarial only
#   ./run_stress_suite.sh --skip-adversarial # load only
#   ./run_stress_suite.sh --rps 100          # single RPS level
#
# Exit codes:
#   0 — all tests passed, no regression
#   1 — regression detected
#   2 — infrastructure error (compose failed, health timeout, missing binary)
# ─────────────────────────────────────────────────────────────────────────────

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DOCKER_COMPOSE_FILE="$PROJECT_ROOT/deploy/docker-compose.yml"
RESULTS_BASE="$SCRIPT_DIR/results"
BASELINE_FILE="$SCRIPT_DIR/baseline.json"
TIMESTAMP="$(date '+%Y%m%d-%H%M%S')"
RESULTS_DIR="$RESULTS_BASE/$TIMESTAMP"

HEALTH_URL="${TAMGA_HEALTH_URL:-http://localhost:8443/api/v1/health}"
HEALTH_TIMEOUT="${TAMGA_HEALTH_TIMEOUT:-60}"
K6_BIN="${K6_BIN:-k6}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
TOTAL_TIMEOUT="${STRESS_SUITE_TIMEOUT:-600}"  # 10 minutes hard limit

SKIP_LOAD=false
SKIP_ADVERSARIAL=false
RPS_LEVELS=("100" "500" "1000")
WORKLOAD_DURATION="180s"  # 3 minutes for CI (full is 12m)

# ── helpers ─────────────────────────────────────────────────────────────────

_red()    { echo -e "\033[31m$*\033[0m"; }
_green()  { echo -e "\033[32m$*\033[0m"; }
_yellow() { echo -e "\033[33m$*\033[0m"; }
_dim()    { echo -e "\033[2m$*\033[0m"; }

log()     { echo "[$(date '+%H:%M:%S')] $*"; }
ok()      { _green "  ✓ $*"; }
fail()    { _red "  ✗ $*"; }
warn()    { _yellow "  ⚠ $*"; }

cleanup() {
    local exit_code=$?
    log "Cleaning up... (exit=$exit_code)"
    cd "$PROJECT_ROOT"
    docker compose -f "$DOCKER_COMPOSE_FILE" down --timeout 30 2>/dev/null || true
    if [ $exit_code -eq 0 ]; then
        _green "Stress suite complete — exit 0"
    elif [ $exit_code -eq 1 ]; then
        _red "Stress suite complete — REGRESSION DETECTED (exit 1)"
    else
        _red "Stress suite complete — INFRA ERROR (exit $exit_code)"
    fi
    exit $exit_code
}

trap cleanup EXIT

# ── arg parsing ─────────────────────────────────────────────────────────────

while [ $# -gt 0 ]; do
    case "$1" in
        --skip-load) SKIP_LOAD=true; shift ;;
        --skip-adversarial) SKIP_ADVERSARIAL=true; shift ;;
        --rps) RPS_LEVELS=("$2"); shift 2 ;;
        --health-url) HEALTH_URL="$2"; shift 2 ;;
        *) echo "Unknown flag: $1"; exit 2 ;;
    esac
done

# ── pre-flight ──────────────────────────────────────────────────────────────

log "Tamga Stress Test Suite"
log "  Results dir : $RESULTS_DIR"
log "  Baseline    : $BASELINE_FILE"
log "  Compose file: $DOCKER_COMPOSE_FILE"
log ""

if ! command -v docker &>/dev/null; then
    _red "docker not found in PATH"
    exit 2
fi

if ! $SKIP_LOAD && ! command -v "$K6_BIN" &>/dev/null; then
    _red "k6 not found. Install: https://k6.io/docs/get-started/installation/"
    _red "  or run with --skip-load"
    exit 2
fi

if [ ! -f "$DOCKER_COMPOSE_FILE" ]; then
    _red "docker-compose.yml not found at $DOCKER_COMPOSE_FILE"
    exit 2
fi

mkdir -p "$RESULTS_DIR"

# ── infrastructure ──────────────────────────────────────────────────────────

log "Starting Tamga stack..."
cd "$PROJECT_ROOT"
docker compose -f "$DOCKER_COMPOSE_FILE" up -d --wait 2>&1 | _dim

# Wait for proxy health
log "Waiting for proxy health ($HEALTH_TIMEOUT seconds)..."
START_TS=$(date +%s)
while true; do
    if curl -sf -o /dev/null "$HEALTH_URL"; then
        ok "Proxy healthy at $HEALTH_URL"
        break
    fi
    ELAPSED=$(($(date +%s) - START_TS))
    if [ "$ELAPSED" -ge "$HEALTH_TIMEOUT" ]; then
        _red "Health check timed out after ${HEALTH_TIMEOUT}s"
        exit 2
    fi
    sleep 2
done

# Give postgres migrations a moment
sleep 3

# ── adversarial tests ───────────────────────────────────────────────────────

if ! $SKIP_ADVERSARIAL; then
    log "Running adversarial bypass tests..."
    ADV_DIR="$SCRIPT_DIR/adversarial"

    for script in pii_bypass.py injection_bypass.py secret_bypass.py policy_bypass.py; do
        script_path="$ADV_DIR/$script"
        if [ ! -f "$script_path" ]; then
            warn "Skipping $script (not found)"
            continue
        fi
        log "  → $script"
        "$PYTHON_BIN" "$script_path" --json --output-dir "$RESULTS_DIR" > "$RESULTS_DIR/adversarial_$(basename "$script" .py).json" 2>&1 || {
            warn "$script exited non-zero (bypasses found — this is expected)"
        }
        ok "$script complete"
    done

    # Merge adversarial results into a single file
    "$PYTHON_BIN" -c "
import json, pathlib
results = {}
for f in sorted(pathlib.Path('$RESULTS_DIR').glob('adversarial_*.json')):
    data = json.loads(f.read_text())
    cat = data.get('category', f.stem.replace('adversarial_', ''))
    results[cat] = {
        'total': data.get('total', 0),
        'detected': data.get('detected', 0),
        'bypassed': data.get('bypassed', 0),
    }
total_bypassed = sum(r['bypassed'] for r in results.values())
results['total_bypassed'] = total_bypassed
with open('$RESULTS_DIR/adversarial_results.json', 'w') as out:
    json.dump(results, out, indent=2)
print(f'Adversarial complete: {total_bypassed} total bypassed across {len(results)-1} categories')
"
fi

# ── k6 load tests ───────────────────────────────────────────────────────────

if ! $SKIP_LOAD; then
    log "Running k6 load tests..."
    K6_DIR="$SCRIPT_DIR/k6"
    PROXY_URL="${TAMGA_BASE_URL:-http://localhost:8443}"
    API_KEY="${TAMGA_API_KEY:-test-key}"

    for rps in "${RPS_LEVELS[@]}"; do
        script_file="$K6_DIR/baseline_${rps}rps.js"
        if [ ! -f "$script_file" ]; then
            warn "Skipping ${rps}rps (script not found: $script_file)"
            continue
        fi
        log "  → ${rps} RPS baseline"
        TAMGA_BASE_URL="$PROXY_URL" TAMGA_API_KEY="$API_KEY" \
            "$K6_BIN" run --summary-export="$RESULTS_DIR/load_test_${rps}rps.json" \
            "$script_file" 2>&1 | _dim
        ok "${rps} RPS complete"
    done

    # Workload mix (short version)
    workload_file="$K6_DIR/workload_mix.js"
    if [ -f "$workload_file" ]; then
        log "  → workload_mix (${WORKLOAD_DURATION})"
        TAMGA_BASE_URL="$PROXY_URL" TAMGA_API_KEY="$API_KEY" \
            "$K6_BIN" run --duration "$WORKLOAD_DURATION" \
            --summary-export="$RESULTS_DIR/workload_mix.json" \
            "$workload_file" 2>&1 | _dim
        ok "workload_mix complete"
    fi
fi

# ── regression check ────────────────────────────────────────────────────────

log "Running regression check..."
"$PYTHON_BIN" "$SCRIPT_DIR/check_regression.py" \
    --results-dir "$RESULTS_DIR" \
    --baseline "$BASELINE_FILE"
REGRESSION_EXIT=$?

# ── done ────────────────────────────────────────────────────────────────────

echo ""
if [ $REGRESSION_EXIT -eq 0 ]; then
    _green "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    _green "  STRESS SUITE PASSED — No regression detected"
    _green "  Results: $RESULTS_DIR"
    _green "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
elif [ $REGRESSION_EXIT -eq 1 ]; then
    _red "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    _red "  STRESS SUITE FAILED — Regression detected"
    _red "  Results: $RESULTS_DIR"
    _red "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
fi

exit $REGRESSION_EXIT
