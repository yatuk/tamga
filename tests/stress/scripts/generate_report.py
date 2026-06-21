#!/usr/bin/env python3
"""Aggregate all stress test JSON results into a single markdown report."""
import json, os, glob
from datetime import datetime

RESULT_DIR = "tests/stress/results"
REPORT_DIR = "tests/stress/reports"

def load_results(pattern):
    files = glob.glob(os.path.join(RESULT_DIR, pattern))
    results = []
    for f in files:
        try:
            with open(f) as fp:
                results.append(json.load(fp))
        except Exception as e:
            print(f"Error loading {f}: {e}")
    return results

def generate():
    os.makedirs(REPORT_DIR, exist_ok=True)
    lines = []
    lines.append("# Tamga Stress Test — Aggregate Report")
    lines.append(f"**Generated:** {datetime.now().isoformat()}")
    lines.append("")

    # Load tests
    load_tests = load_results("load_*.json")
    adv_tests = load_results("adversarial_*.json")

    lines.append("## Load Test Results")
    for t in load_tests:
        lines.append(f"- **{t.get('test', '?')}**: max_sustained={t.get('max_sustained_rps', '?')} RPS")
        for r in t.get('results', []):
            lines.append(f"  - {r.get('rps_target', '?')} RPS: p95={r.get('p95_ms', '?')}ms | status={r.get('status', '?')}")

    lines.append("")
    lines.append("## Adversarial Test Results")
    for t in adv_tests:
        lines.append(f"- **{t.get('test', '?')}**: {t.get('detected', '?')}/{t.get('total_vectors', '?')} detected, {t.get('bypassed', '?')} bypassed ({t.get('bypass_rate', 0)*100:.1f}% bypass)")
        for d in t.get('details', []):
            if d.get('bypassed'):
                lines.append(f"  - BYPASS: {d.get('name', '?')}")

    report_path = os.path.join(REPORT_DIR, f"AGGREGATE_{datetime.now().strftime('%Y%m%d_%H%M%S')}.md")
    with open(report_path, 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines))
    print(f"Report written to {report_path}")
    print('\n'.join(lines))

if __name__ == "__main__":
    generate()
