#!/usr/bin/env python3
"""PII Scanner Bypass Test Suite.

Sends adversarial prompts to the Tamga proxy and checks whether PII scanners
correctly detect obfuscated personal data.

Usage:
    TAMGA_BASE_URL=http://localhost:8443 TAMGA_API_KEY=test-key python pii_bypass.py
"""

import json
import os
import sys
import urllib.request
from dataclasses import dataclass, field
from datetime import datetime, timezone
import argparse

BASE = os.environ.get("TAMGA_BASE_URL", "http://localhost:8443")
API_KEY = os.environ.get("TAMGA_API_KEY", "test-key")


@dataclass
class TestVector:
    name: str
    content: str
    expected_finding: bool = True  # True = should be detected as PII


@dataclass
class TestResult:
    vector: TestVector
    status_code: int
    findings_count: int
    findings: list = field(default_factory=list)
    bypassed: bool = False
    error: str = ""


def get_detection_count(headers_dict) -> int:
    """Extract detection count from various response headers."""
    for key in ("X-Tamga-Findings-Count", "X-Tamga-Redacted-Count", "x-tamga-findings-count", "x-tamga-redacted-count"):
        val = headers_dict.get(key, "0")
        if val and val != "0":
            try:
                return int(val)
            except (ValueError, TypeError):
                pass
    return 0


def is_detected(status_code: int, headers_dict: dict) -> bool:
    """Check whether PII/secrets/injection was detected."""
    # Check explicit count headers
    if get_detection_count(headers_dict) > 0:
        return True
    # Check risk level (critical/high means detection happened)
    risk = (headers_dict.get("X-Tamga-Risk-Level", "") or
            headers_dict.get("x-tamga-risk-level", ""))
    if risk in ("critical", "high"):
        return True
    # Check if blocked (403) or action reason present
    if status_code == 403:
        return True
    action_reason = (headers_dict.get("X-Tamga-Action-Reason", "") or
                     headers_dict.get("x-tamga-action-reason", ""))
    if action_reason:
        return True
    return False


def send_request(content: str) -> tuple[int, dict, str]:
    """Send a single prompt to the proxy and return status, headers, body."""
    payload = json.dumps({
        "model": "claude-3-haiku-20240307",
        "messages": [{"role": "user", "content": content}],
    }, ensure_ascii=False).encode()
    req = urllib.request.Request(
        f"{BASE}/v1/messages",
        data=payload,
        headers={
            "Content-Type": "application/json",
            "x-api-key": API_KEY,
        },
    )
    try:
        resp = urllib.request.urlopen(req, timeout=30)
        body = resp.read().decode()
        return (resp.status,
                {k.lower(): v for k, v in resp.headers.items()},
                body)
    except urllib.error.HTTPError as e:
        return (e.code,
                {k.lower(): v for k, v in e.headers.items()},
                e.read().decode()[:500])
    except Exception as e:
        return (0, {}, str(e)[:200])


def run_test(vec: TestVector) -> TestResult:
    status, headers, body = send_request(vec.content)
    detected = is_detected(status, headers)
    findings_count = get_detection_count(headers)
    bypassed = vec.expected_finding and not detected
    return TestResult(
        vector=vec,
        status_code=status,
        findings_count=findings_count,
        bypassed=bypassed,
        error="" if status else body,
    )


def main():
    parser = argparse.ArgumentParser(description="PII Scanner Bypass Test Suite")
    parser.add_argument("--json", action="store_true", help="Output JSON to stdout instead of text")
    parser.add_argument("--output-dir", default="tests/stress/results", help="Directory for JSON results file")
    args = parser.parse_args()
    json_mode = args.json
    output_dir = args.output_dir

    vectors = [
        # --- Unicode evasion ---
        TestVector("TCKN — Mathematical bold digits",
                   "My ID is \U0001D7CF\U0001D7CE\U0001D7CE\U0001D7CE\U0001D7CE\U0001D7CE\U0001D7CE\U0001D7CE\U0001D7CF\U0001D7D2\U0001D7D4"),  # 10000000146 (valid TCKN)00000146
        TestVector("TCKN — Homoglyph I",
                   "TC Kimlik No: 10000000146 ile Ι karakteri"),  # Greek uppercase iota
        TestVector("Email — Homoglyph @",
                   "Contact: user＠example.com"),  # Fullwidth @

        # --- Format manipulation ---
        TestVector("TCKN — Dots between digits",
                   "TC: 1.0000.0001.46"),  # 11 digits
        TestVector("TCKN — Spaces between digits",
                   "Kimlik: 1 0 0 0 0 0 0 0 1 4 6"),
        TestVector("TCKN — Reversed",
                   "No: 64100000001"),

        # --- Encoding ---
        TestVector("Email — Base64 wrapped",
                   "Here is my email in base64: dXNlckBleGFtcGxlLmNvbQ=="),
        TestVector("TCKN — Base64",
                   "ID: MTAwMDAwMDAxNDY="),

        # --- Zero-width characters ---
        TestVector("Email — Zero-width space in address",
                   "user​@example.com"),
        TestVector("TCKN — Zero-width space",
                   "1​0000000146"),

        # --- HTML entities ---
        TestVector("Email — HTML entities",
                   "&#117;&#115;&#101;&#114;&#64;example.com"),

        # --- Turkish word-to-number ---
        TestVector("Phone — Turkish word digits",
                   "Numaram: sıfır beş üç iki beş elli beş on iki otuz dört"),

        # --- Indirect reference ---
        TestVector("TCKN — Indirect description",
                   "My citizen ID starts with 100000, ends with 146, has five zeros in the middle"),

        # --- Baseline (should always be detected) ---
        TestVector("Email — Plain baseline",
                   "My email is user@example.com"),
        TestVector("TCKN — Plain baseline",
                   "TC: 10000000146"),
        TestVector("Phone — Plain baseline",
                   "Call me at +90 532 555 1234"),
        TestVector("Credit card — Plain baseline",
                   "Card: 4532015112830366"),
    ]

    results = []
    bypassed = []
    detected = []
    error_count = 0

    if not json_mode:
        print("=" * 70)
        print("PII SCANNER BYPASS TEST SUITE")
        print(f"Target: {BASE}")
        print(f"Total vectors: {len(vectors)}")
        print("=" * 70)

    for vec in vectors:
        result = run_test(vec)
        results.append(result)
        if result.error:
            error_count += 1

        if result.bypassed:
            status_icon = "!! BYPASS"
        elif result.status_code >= 200 and result.status_code < 500:
            status_icon = "OK"
        else:
            status_icon = "!! ERR"
        name = vec.name.encode('ascii', errors='replace').decode()
        if not json_mode:
            print(f"  {status_icon:10s} | {name:45s} | findings={result.findings_count} | HTTP {result.status_code}")

        if result.bypassed:
            bypassed.append(result)
        elif vec.expected_finding and result.findings_count > 0:
            detected.append(result)

    # Summary
    total_expected = sum(1 for v in vectors if v.expected_finding)
    bypass_count = len(bypassed)
    detected_count = len(detected)

    # Build consistent JSON output
    json_output = {
        "category": "pii",
        "test": "pii_bypass",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "total": len(vectors),
        "detected": detected_count,
        "bypassed": bypass_count,
        "bypass_rate": bypass_count / total_expected if total_expected else 0,
        "error": error_count,
        "vectors": [
            {
                "name": r.vector.name,
                "expected_finding": r.vector.expected_finding,
                "detected": bool(r.findings_count > 0 or (r.status_code == 403)),
                "bypassed": r.bypassed,
                "findings_count": r.findings_count,
                "status_code": r.status_code,
            }
            for r in results
        ],
    }

    # Write results file
    os.makedirs(output_dir, exist_ok=True)
    results_file = os.path.join(output_dir, "adversarial_pii_bypass.json")
    with open(results_file, "w") as f:
        json.dump(json_output, f, indent=2, default=str)

    if json_mode:
        print(json.dumps(json_output, indent=2, default=str))
        return 0

    print()
    print("=" * 70)
    print(f"RESULTS: {detected_count}/{total_expected} detected, {bypass_count} bypassed")
    if bypassed:
        print(f"BYPASS RATE: {bypass_count}/{total_expected} = {bypass_count/total_expected*100:.1f}%")
        print()
        print("BYPASSED VECTORS:")
        for r in bypassed:
            print(f"  - {r.vector.name}: {r.vector.content[:80]}...")
    else:
        print("ALL PII DETECTED - No bypasses found")
    print("=" * 70)
    print(f"\nResults written to {results_file}")

    return 0 if bypass_count == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
