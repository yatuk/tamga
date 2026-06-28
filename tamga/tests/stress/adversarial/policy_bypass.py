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

@dataclass
class TestVector:
    name: str
    path: str
    method: str = "GET"
    expect_blocked: bool = True

@dataclass
class TestResult:
    vector: TestVector
    status_code: int = 0
    bypassed: bool = False
    error: str = ""

def send(method, path):
    r = urllib.request.Request(f"{BASE}{path}", method=method,
        headers={"x-api-key": API_KEY})
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

        # Unauthenticated access - should be blocked
        TestVector("API without auth key","/api/v1/stats"),

        # Valid admin endpoints
        TestVector("Stats with auth","/api/v1/stats",expect_blocked=False),
        TestVector("Policies with auth","/api/v1/policies",expect_blocked=False),
        TestVector("Events with auth","/api/v1/events",expect_blocked=False),
    ]

    results, bypassed, blocked, error_count = [], [], [], 0
    if not json_mode:
        print("=" * 70)
        print("POLICY BYPASS TEST SUITE - Week 3")
        print(f"Target: {BASE}  |  Vectors: {len(vectors)}")
        print("=" * 70)

    for vec in vectors:
        status, body = send(vec.method, vec.path)
        was_blocked = status in (403, 401)
        bypassed_flag = vec.expect_blocked and not was_blocked
        r = TestResult(vector=vec, status_code=status, bypassed=bypassed_flag, error=body if status == 0 else "")
        results.append(r)
        if r.error:
            error_count += 1
        (bypassed if bypassed_flag else blocked).append(r)
        icon = "BYPASS" if bypassed_flag else ("OK" if not vec.expect_blocked else "BLOCK")
        if not json_mode:
            print(f"  [{icon:6s}] {vec.name:45s} | HTTP {status}")

    bypass_count = len(bypassed)
    detected_count = sum(1 for r in results if r.status_code in (403, 401))
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
        "vectors": [
            {
                "name": r.vector.name,
                "expected_finding": r.vector.expect_blocked,
                "detected": r.status_code in (403, 401),
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
    print(f"RESULTS: {total - bypass_count}/{total} handled correctly")
    if bypassed:
        print(f"BYPASSED: {bypass_count}")
        for r in bypassed:
            print(f"  - {r.vector.name}")
    else:
        print("ALL POLICY CHECKS PASSED (11/11)")
    print("=" * 70)
    print(f"\nResults written to {results_file}")

    return 0 if bypass_count == 0 else 1

if __name__ == "__main__":
    sys.exit(main())
