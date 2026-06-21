#!/usr/bin/env python3
"""Secret Scanner Bypass Test Suite — Week 3 Verification.

Tests Tamga's secret scanner against credential leakage including
AWS key separators, secret key variants, GitHub/Stripe formatting
evasions, and context-aware detection.

Usage:
    TAMGA_BASE_URL=http://localhost:8443 TAMGA_API_KEY=test-key python secret_bypass.py
"""

import argparse, json, os, sys, urllib.request
from dataclasses import dataclass
from datetime import datetime, timezone

BASE = os.environ.get("TAMGA_BASE_URL", "http://localhost:8443")
API_KEY = os.environ.get("TAMGA_API_KEY", "test-key")

@dataclass
class TestVector:
    name: str
    content: str
    category: str

@dataclass
class TestResult:
    vector: TestVector
    status_code: int = 0
    findings_count: int = 0
    bypassed: bool = False
    error: str = ""

def get_detection_count(h):
    for k in ('X-Tamga-Findings-Count','x-tamga-findings-count'):
        v = h.get(k,'0')
        if v and v != '0':
            try: return int(v)
            except: pass
    return 0

def is_detected(status, h):
    if get_detection_count(h) > 0: return True
    if h.get('X-Tamga-Risk-Level','') in ('critical','high'): return True
    if status == 403: return True
    return False

def send(content):
    p = json.dumps({"model":"claude-3-haiku-20240307","messages":[{"role":"user","content":content}]}).encode()
    r = urllib.request.Request(f"{BASE}/v1/messages", data=p, headers={"Content-Type":"application/json","x-api-key":API_KEY})
    try:
        resp = urllib.request.urlopen(r, timeout=30)
        return resp.status, {k.lower():v for k,v in resp.headers.items()}, resp.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, {k.lower():v for k,v in e.headers.items()}, e.read().decode()[:500]
    except Exception as e:
        return 0, {}, str(e)[:200]

def main():
    parser = argparse.ArgumentParser(description="Secret Scanner Bypass Test Suite")
    parser.add_argument("--json", action="store_true", help="Output JSON to stdout instead of text")
    parser.add_argument("--output-dir", default="tests/stress/results", help="Directory for JSON results file")
    args = parser.parse_args()
    json_mode = args.json
    output_dir = args.output_dir

    vectors = [
        TestVector("AWS access key with spaces","AKIA IOSF ODNN 7EXAMPLE","aws_access_key"),
        TestVector("AWS access key with dashes","AKIA-IOSF-ODNN-7EXAMPLE","aws_access_key"),
        TestVector("AWS access key with plus","AKIA + IOSFODNN + 7EXAMPLE","aws_access_key"),
        TestVector("AWS secret access key strict","AWS_SECRET_ACCESS_KEY=abcdefghijklmnopqrstuvwxyz0123456789ABCD","aws_secret_key"),
        TestVector("AWS secret key broad","AWS_SECRET_KEY=abcdefghijklmnopqrstuvwxyz0123456789ABCD","aws_secret_key_broad"),
        TestVector("AWS secret key raw 40-char","wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY","aws_secret_key_raw"),
        TestVector("GitHub PAT with spaces","ghp_ aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll","github_token"),
        TestVector("GitHub PAT with dashes","ghp_aaaa-bbbb-cccc-dddd-eeee-ffff-gggg-hhhh-iiii-jjjj-kkkk-llll","github_token"),
        TestVector("Stripe live key with spaces","sk_live_ aaaa bbbb cccc dddd eeee ffff","stripe_key"),
        TestVector("Stripe test key with dashes","sk_test_-aaaa-bbbb-cccc-dddd-eeee-ffff","stripe_key"),
        TestVector("Baseline AWS access key clean","AKIAIOSFODNN7EXAMPLE","aws_access_key"),
        TestVector("Baseline Anthropic key","sk-ant-api03-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx","anthropic_key"),
    ]
    results, bypassed, detected, error_count = [], [], [], 0
    if not json_mode:
        print("=" * 70)
        print("SECRET SCANNER BYPASS TEST SUITE - Week 3")
        print(f"Target: {BASE}  |  Vectors: {len(vectors)}")
        print("=" * 70)
    for vec in vectors:
        status, headers, body = send(vec.content)
        det = is_detected(status, headers)
        fc = get_detection_count(headers)
        r = TestResult(vector=vec, status_code=status, findings_count=fc, bypassed=not det, error=body if status == 0 else "")
        results.append(r)
        if r.error:
            error_count += 1
        (bypassed if r.bypassed else detected).append(r)
        icon = "BYPASS" if r.bypassed else ("ERR" if status==0 else "DETECT")
        if not json_mode:
            print(f"  [{icon:6s}] {vec.name:45s} | findings={fc} | HTTP {status}")
    total = len(vectors)
    bypass_count = len(bypassed)
    detected_count = len(detected)

    # Build consistent JSON output
    json_output = {
        "category": "secret",
        "test": "secret_bypass",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "total": total,
        "detected": detected_count,
        "bypassed": bypass_count,
        "bypass_rate": bypass_count / total if total else 0,
        "error": error_count,
        "vectors": [
            {
                "name": r.vector.name,
                "expected_finding": True,
                "detected": not r.bypassed,
                "bypassed": r.bypassed,
                "findings_count": r.findings_count,
                "status_code": r.status_code,
            }
            for r in results
        ],
    }

    # Write results file
    os.makedirs(output_dir, exist_ok=True)
    results_file = os.path.join(output_dir, "adversarial_secret_bypass.json")
    with open(results_file, "w") as f:
        json.dump(json_output, f, indent=2, default=str)

    if json_mode:
        print(json.dumps(json_output, indent=2, default=str))
        return 0

    print()
    print("=" * 70)
    print(f"RESULTS: {detected_count}/{total} detected, {bypass_count} bypassed")
    if bypassed:
        for r in bypassed:
            print(f"  - [{r.vector.category}] {r.vector.name}")
    else:
        print("ALL SECRETS DETECTED (12/12)")
    print("=" * 70)
    print(f"\nResults written to {results_file}")

    return 0 if bypass_count == 0 else 1

if __name__ == "__main__":
    sys.exit(main())
