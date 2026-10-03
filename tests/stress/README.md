# Tamga Stress Test Suite

Automated adversarial bypass and load test suite with regression detection.

## Quick Start

```bash
# From the repository root:
./tests/stress/run_stress_suite.sh
```

This single command:
1. Starts the full Tamga stack with `docker-compose.stress.yml` layered on top
2. Waits for the proxy to become healthy (up to 60 seconds)
3. Runs 6 adversarial bypass test suites (PII, injection, secret, policy,
   operator state, request structure)
4. Runs k6 load tests at 100, 500, and 1000 RPS
5. Runs a short workload mix test (3 minutes)
6. Checks results against `baseline.json` for regressions
7. Tears down the stack (`docker compose down`) — always, even on failure

**Requirements:** Docker, Python 3.9+, k6 (for load tests)

**Duration:** 7-8 minutes

### What the suite sets up for itself

- **Mocked upstream** (`TAMGA_MOCK_UPSTREAM=true`). Scanning and policy run as
  usual, but nothing is forwarded to a real provider — the suite sends
  adversarial payloads and must not leak them, or spend API credit.
- **The repo-root `.env`**, passed with `--env-file` when it exists. Compose
  does not pick it up on its own.
- **The shipped default policy**, plus the operator_state authorization
  allowlist those vectors need. `scripts/make_stress_policy.py` derives it
  from `proxy/tamga-policy.yaml` at run time, so there is no second policy
  file to drift.
- **Operator-state fixtures** from `proxy/testdata/operator_state/`.
- **In-process scanning**, regardless of `TAMGA_SCANNER_SERVICE_ADDR`.
- **One API key per load-test request.** The default policy rate-limits per
  key (60/min); a single key would measure the limiter's 429s, not the proxy.

### How a vector is scored

A vector counts as detected when the response carries a finding
(`X-Tamga-Findings-Count`), a high or critical risk level, or a 403. It is a
detection metric, not an enforcement one: a finding that the policy lets
through still counts. Enforcement of the default policy is covered by
`proxy/internal/policy/default_policy_test.go`.

The policy category contains 4 control requests that are expected to pass
(the health endpoint, and three admin endpoints called with the admin key).
They are not counted as detections; a control that does not return 200 is
reported under `controls_failed`. The suite starts the stack with a known
`TAMGA_ADMIN_KEY` so the controls can authenticate.

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
| 1 | Regression detected (more bypasses, higher P95, or more errors under load) |
| 2 | Baseline file missing or unreadable |

### Rules

- **Adversarial:** Any category with more bypasses than baseline → regression
- **Load P95:** P95 latency exceeds baseline by more than 20% → regression
- **Load error rate:** more than 1 percentage point above baseline → regression

### Limiting the P95 gate

P95 depends on the machine. `--load-gate-levels 100rps` (or
`STRESS_LOAD_GATE_LEVELS=100rps`) gates on P95 only at the listed levels; at
the others a P95 over its threshold is printed as `SLOW (advisory)` and does
not fail the check. Bypass counts and the load error rate gate at every level.
A local run gates on every level by default.

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
- Runs the adversarial tests (`--skip-load`)
- Uploads results as a 30-day artifact
- Posts a summary comment on the PR
- Fails the check if a category has more bypasses than the baseline

Load tests are not part of the CI gate. A shared runner hosts the whole stack
and k6 on the same few cores, so latency there measures the runner, not the
change: the same code measured p95 4.8 ms at 1000 RPS on a 16-core laptop and
1340 ms on the runner. Run them locally before a release, or tick **load** on a
manual run; that run gates on P95 at 100 RPS only
(see [Limiting the P95 gate](#limiting-the-p95-gate)).

Manual trigger: **Actions → Adversarial Regression Gate → Run workflow**

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
