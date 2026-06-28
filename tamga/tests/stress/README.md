# Tamga Stress Test Suite

Automated adversarial bypass and load test suite with regression detection.

## Quick Start

```bash
# From the repository root:
./tests/stress/run_stress_suite.sh
```

This single command:
1. Starts the full Tamga stack (`docker compose up -d`)
2. Waits for the proxy to become healthy (up to 60 seconds)
3. Runs 4 adversarial bypass test suites (PII, injection, secret, policy)
4. Runs k6 load tests at 100, 500, and 1000 RPS
5. Runs a short workload mix test (3 minutes)
6. Checks results against `baseline.json` for regressions
7. Tears down the stack (`docker compose down`) — always, even on failure

**Requirements:** Docker, Python 3.9+, k6 (for load tests)

**Duration:** 5-8 minutes

## Options

```bash
./run_stress_suite.sh --skip-load          # Adversarial tests only
./run_stress_suite.sh --skip-adversarial   # Load tests only
./run_stress_suite.sh --rps 100            # Single RPS level
```

**Windows:**
```powershell
.\run_stress_suite.ps1 -SkipLoad
```

## Results

Results are written to `tests/stress/results/<YYYYMMDD-HHMMSS>/`:

| File | Content |
|------|---------|
| `adversarial_pii_bypass.json` | PII test vectors and detection results |
| `adversarial_injection_bypass.json` | Injection test vectors |
| `adversarial_secret_bypass.json` | Secret test vectors |
| `adversarial_policy_bypass.json` | Policy test vectors |
| `adversarial_results.json` | Merged adversarial summary |
| `load_test_100rps.json` | k6 summary for 100 RPS |
| `load_test_500rps.json` | k6 summary for 500 RPS |
| `load_test_1000rps.json` | k6 summary for 1000 RPS |
| `workload_mix.json` | Workload mix test summary |

## Regression Check

```bash
python check_regression.py \
  --results-dir results/20260617-120000 \
  --baseline baseline.json
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Stable or improved — no regression |
| 1 | Regression detected (more bypasses or higher P95) |
| 2 | Baseline file missing or unreadable |

### Rules

- **Adversarial:** Any category with more bypasses than baseline → regression
- **Load:** P95 latency exceeds baseline by more than 20% → regression
- **Error rate:** Not checked for regression (only P95)

### JSON Output

```bash
python check_regression.py --results-dir results/... --baseline baseline.json --json
```

## Adversarial Test Categories

| Category | Test Vectors | Bypass Techniques |
|----------|-------------|-------------------|
| **PII** | 17 | Unicode evasion (math bold, fullwidth), homoglyphs, zero-width chars, base64, HTML entities, Turkish word-to-number, indirect description |
| **Injection** | 22 | Prompt injection variants, role confusion, encoding tricks, delimiter injection, context manipulation |
| **Secret** | 12 | API key fragments, obfuscated tokens, multi-line secrets, environment variable leaks |
| **Policy** | 11 | Provider allowlist bypass, rate limit evasion, budget overflow, rule precedence edge cases |

Each test script supports `--json` for machine-readable output and `--output-dir` for custom result paths.

## Load Test Profiles

| Profile | Duration | Pattern | Purpose |
|---------|----------|---------|---------|
| `baseline_100rps.js` | 60s | Constant 100 RPS | Light load baseline |
| `baseline_500rps.js` | 60s | Constant 500 RPS | Moderate load baseline |
| `baseline_1000rps.js` | 60s | Constant 1000 RPS | High load baseline |
| `workload_mix.js` | 180s (CI) / 720s (full) | Mixed endpoints, ramp-up | Realistic traffic pattern |

## Updating the Baseline

After a scanner hardening sprint or policy improvement that reduces bypasses:

1. Run the full suite and verify results:
   ```bash
   ./run_stress_suite.sh
   ```

2. Copy the improved results as the new baseline:
   ```bash
   # Take the adversarial counts and load P95 values from the latest results
   # and update baseline.json manually. The format is:
   ```

   ```json
   {
     "version": "1.0.0",
     "updated_at": "2026-06-17",
     "source_commit": "<git-sha>",
     "adversarial": { ... },
     "load": { ... }
   }
   ```

3. Commit the updated baseline:
   ```bash
   git add tests/stress/baseline.json
   git commit -m "test(stress): update baseline after hardening sprint"
   ```

**Important:** Only update the baseline when improvements are intentional. Never lower the baseline to make a regression disappear.

## CI Integration

The `adversarial-gate.yml` workflow runs on every PR to `dev` or `main` that touches proxy, analyzer, or stress test files. It:

- Builds Docker images
- Runs the full suite
- Uploads results as a 30-day artifact
- Posts a summary comment on the PR
- Fails the check if regression is detected

Manual trigger: **Actions → Adversarial & Load Regression Gate → Run workflow**

## Troubleshooting

### "k6 not found"
Install k6: https://k6.io/docs/get-started/installation/
Or skip load tests: `./run_stress_suite.sh --skip-load`

### "Health check timed out"
The proxy didn't become healthy within 60 seconds. Check Docker logs:
```bash
docker compose -f deploy/docker-compose.yml logs proxy
```

Increase timeout:
```bash
TAMGA_HEALTH_TIMEOUT=120 ./run_stress_suite.sh
```

### "docker compose: command not found"
Use `docker-compose` (v1) or install Docker Compose v2.

### All tests fail with connection errors
Ensure no other process is using port 8443. The proxy must be reachable at `http://localhost:8443`.

## Out of Scope

These are NOT run by the suite (too long for CI, suitable for nightly/manual):

- 2000 and 5000 RPS load tests (`baseline_2000rps.js`)
- Soak test (2-hour constant load, `soak_constant.js`)
- Connection exhaustion test (`connection_exhaustion.js`)
- Endpoint mix test (`endpoint_mix.js`)
