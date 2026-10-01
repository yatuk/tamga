#!/usr/bin/env python3
"""Policy Bypass Test Suite — Week 3 Verification.

Tests that Tamga's policy enforcement correctly handles various
evasion attempts including path traversal, malformed requests,
and policy boundary conditions.

Usage:
    TAMGA_BASE_URL=http://localhost:8443 TAMGA_API_KEY=test-key python policy_bypass.py
"""

import argparse, json, os, sys, urllib.request
from datetime import datetime, timezone
from dataclasses import dataclass

BASE = os.environ.get("TAMGA_BASE_URL", "http://localhost:8443")
API_KEY = os.environ.get("TAMGA_API_KEY", "test-key")
# The management API authenticates with X-Tamga-Admin-Key, not the proxy-path
# x-api-key. run_stress_suite starts the stack with this same value.
ADMIN_KEY = os.environ.get("TAMGA_ADMIN_KEY", "")

@dataclass
class TestVector:
    name: str
    path: str
    method: str = "GET"
    expect_blocked: bool = True
    as_admin: bool = False  # send the admin key (control requests that must succeed)

@dataclass
class TestResult:
    vector: TestVector
    status_code: int = 0
    bypassed: bool = False
    error: str = ""

def send(method, path, as_admin=False):
    headers = {"x-api-key": API_KEY}
    if as_admin and ADMIN_KEY:
        headers["X-Tamga-Admin-Key"] = ADMIN_KEY
    r = urllib.request.Request(f"{BASE}{path}", method=method, headers=headers)
    try:
        resp = urllib.request.urlopen(r, timeout=10)
        return resp.status, resp.read().decode()[:200]
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()[:200]
    except Exception as e:
        return 0, str(e)[:200]

def main():
    parser = argparse.ArgumentParser(description="Policy Bypass Test Suite")
    parser.add_argument("--json", action="store_true", help="Output JSON to stdout instead of text")
    parser.add_argument("--output-dir", default="tests/stress/results", help="Directory for JSON results file")
    args = parser.parse_args()
    json_mode = args.json
    output_dir = args.output_dir

    vectors = [
        # Path traversal (Week 3.3) - should be blocked with 403
        TestVector("Path traversal %2F encoded","/api/v1/..%2F..%2Fadmin"),
        TestVector("Path traversal %252F double","/api/v1/..%252F..%252Fadmin"),
        TestVector("Path traversal overlong UTF-8","/api/v1/..%c0%af..%c0%afadmin"),
        TestVector("Path traversal encoded dots","/api/v1/%2e%2e/%2e%2e/admin"),
        TestVector("Path traversal plain ../","/api/v1/../admin"),
        TestVector("Path traversal backslash","/api/v1/..%5C..%5Cadmin"),

        # Policy boundary - health endpoint (should be accessible)
        TestVector("Health endpoint accessible","/health",expect_blocked=False),

        # No admin key - should be blocked
        TestVector("API without auth key","/api/v1/stats"),

        # Controls: the same endpoints with the admin key must be reachable
        TestVector("Stats with auth","/api/v1/stats",expect_blocked=False,as_admin=True),
        TestVector("Policies with auth","/api/v1/policies",expect_blocked=False,as_admin=True),
        TestVector("Events with auth","/api/v1/events",expect_blocked=False,as_admin=True),
    ]

    results, bypassed, blocked, error_count = [], [], [], 0
    controls_failed = []
    if not json_mode:
        print("=" * 70)
        print("POLICY BYPASS TEST SUITE - Week 3")
        print(f"Target: {BASE}  |  Vectors: {len(vectors)}")
        print("=" * 70)

    for vec in vectors:
        status, body = send(vec.method, vec.path, vec.as_admin)
        was_blocked = status in (403, 401)
        bypassed_flag = vec.expect_blocked and not was_blocked
        # A control that does not come back 200 means the suite, or the proxy,
        # is refusing legitimate traffic. It is not a detection.
        control_failed = (not vec.expect_blocked) and status != 200
        r = TestResult(vector=vec, status_code=status, bypassed=bypassed_flag, error=body if status == 0 else "")
        results.append(r)
        if r.error:
            error_count += 1
        if control_failed:
            controls_failed.append(r)
        (bypassed if bypassed_flag else blocked).append(r)
        icon = "BYPASS" if bypassed_flag else ("FAIL" if control_failed else ("OK" if not vec.expect_blocked else "BLOCK"))
        if not json_mode:
            print(f"  [{icon:6s}] {vec.name:45s} | HTTP {status}")

    bypass_count = len(bypassed)
    # Only attack vectors can be "detected"; controls are reported separately.
    detected_count = sum(1 for r in results if r.vector.expect_blocked and r.status_code in (403, 401))
    total = len(vectors)
    total_expected = sum(1 for v in vectors if v.expect_blocked)

    # Build consistent JSON output
    json_output = {
        "category": "policy",
        "test": "policy_bypass",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "total": total,
        "detected": detected_count,
        "bypassed": bypass_count,
        "bypass_rate": bypass_count / total_expected if total_expected else 0,
        "error": error_count,
        "controls": total - total_expected,
        "controls_failed": len(controls_failed),
        "vectors": [
            {
                "name": r.vector.name,
                "expected_finding": r.vector.expect_blocked,
                "detected": r.vector.expect_blocked and r.status_code in (403, 401),
                "bypassed": r.bypassed,
                "findings_count": 0,
                "status_code": r.status_code,
            }
            for r in results
        ],
    }

    # Write results file
    os.makedirs(output_dir, exist_ok=True)
    results_file = os.path.join(output_dir, "adversarial_policy_bypass.json")
    with open(results_file, "w") as f:
        json.dump(json_output, f, indent=2, default=str)

    if json_mode:
        print(json.dumps(json_output, indent=2, default=str))
        return 0

    print()
    print("=" * 70)
    wrong = bypass_count + len(controls_failed)
    print(f"RESULTS: {total - wrong}/{total} handled correctly")
    if bypassed:
        print(f"BYPASSED: {bypass_count}")
        for r in bypassed:
            print(f"  - {r.vector.name}")
    if controls_failed:
        print(f"CONTROLS FAILED: {len(controls_failed)} (legitimate request refused; is TAMGA_ADMIN_KEY set to the stack's key?)")
        for r in controls_failed:
            print(f"  - {r.vector.name} | HTTP {r.status_code}")
    if not wrong:
        print(f"ALL POLICY CHECKS PASSED ({total}/{total})")
    print("=" * 70)
    print(f"\nResults written to {results_file}")

    return 0 if wrong == 0 else 1

if __name__ == "__main__":
    sys.exit(main())
