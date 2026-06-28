#!/usr/bin/env python3
"""Regression checker for Tamga stress test results.

Compares current adversarial bypass counts and load test P95/error_rate
against a baseline JSON file. Used as the final gate in the stress suite.

Usage:
    python check_regression.py --results-dir results/20260617-120000 --baseline baseline.json
    python check_regression.py --results-dir results/20260617-120000 --baseline baseline.json --json

Exit codes:
    0 — stable or improved (no regression detected)
    1 — regression detected (current > baseline beyond tolerance)
    2 — baseline file missing or unreadable
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

DEFAULT_TOLERANCE = 0.20  # 20% headroom for load test P95


# ── helpers ──────────────────────────────────────────────────────────────────


def load_json(path: Path) -> dict[str, Any]:
    """Load and parse a JSON file, returning {} on error."""
    try:
        with open(path) as f:
            return json.load(f)
    except (FileNotFoundError, json.JSONDecodeError) as exc:
        print(f"ERROR: Cannot read {path}: {exc}", file=sys.stderr)
        return {}


def load_adversarial_results(results_dir: Path) -> dict[str, dict[str, int]]:
    """Scan results_dir for adversarial_*.json files and merge them.

    Returns dict like {'pii': {'total': 17, 'detected': 6, 'bypassed': 11}, ...}.
    """
    merged: dict[str, dict[str, int]] = {}
    for fpath in sorted(results_dir.glob("adversarial_*.json")):
        data = load_json(fpath)
        cat = data.get("category", fpath.stem.replace("adversarial_", ""))
        merged[cat] = {
            "total": data.get("total", 0),
            "detected": data.get("detected", 0),
            "bypassed": data.get("bypassed", 0),
        }
    return merged


def load_load_results(results_dir: Path) -> dict[str, dict[str, float]]:
    """Scan results_dir for load_test_*.json files and extract P95 + error_rate.

    k6 JSON summary is expected to have: metrics.http_req_duration.values['p(95)']
    and metrics.http_req_failed.values.rate.
    """
    merged: dict[str, dict[str, float]] = {}
    for fpath in sorted(results_dir.glob("load_test_*.json")):
        # Derive label from filename: load_test_100rps.json -> 100rps
        label = fpath.stem.replace("load_test_", "")
        data = load_json(fpath)
        try:
            p95 = data["metrics"]["http_req_duration"]["values"]["p(95)"]
            error_rate = data["metrics"]["http_req_failed"]["values"]["rate"]
        except (KeyError, TypeError):
            print(f"WARNING: {fpath} missing expected k6 summary fields", file=sys.stderr)
            p95 = 0.0
            error_rate = 0.0
        merged[label] = {"p95_ms": round(float(p95), 2), "error_rate": round(float(error_rate), 4)}
    return merged


# ── check functions ──────────────────────────────────────────────────────────


def check_adversarial(
    current: dict[str, dict[str, int]],
    baseline: dict[str, dict[str, int]],
) -> tuple[bool, list[dict[str, Any]]]:
    """Return (has_regression, rows). Regression = any category has more bypasses."""
    has_regression = False
    rows: list[dict[str, Any]] = []
    all_cats = sorted(set(current) | set(baseline))
    # Filter out aggregate keys that aren't real categories
    all_cats = [c for c in all_cats if c != "total_bypassed"]
    total_current = 0
    total_baseline = 0

    for cat in all_cats:
        cur = current.get(cat, {})
        base = baseline.get(cat, {})
        # Skip non-dict values (e.g. "total_bypassed": 29 in baseline)
        if not isinstance(cur, dict):
            cur = {}
        if not isinstance(base, dict):
            base = {}
        cur_bypassed = cur.get("bypassed", 0)
        base_bypassed = base.get("bypassed", 0)
        total_current += cur_bypassed
        total_baseline += base_bypassed

        if cur_bypassed > base_bypassed:
            verdict = "REGRESSION"
            has_regression = True
        elif cur_bypassed < base_bypassed:
            verdict = "IMPROVED"
        else:
            verdict = "STABLE"

        rows.append({
            "category": cat,
            "current_total": cur.get("total", 0),
            "current_detected": cur.get("detected", 0),
            "current_bypassed": cur_bypassed,
            "baseline_bypassed": base_bypassed,
            "delta": cur_bypassed - base_bypassed,
            "verdict": verdict,
        })

    rows.append({
        "category": "TOTAL",
        "current_total": sum(c.get("total", 0) for c in current.values()),
        "current_detected": sum(c.get("detected", 0) for c in current.values()),
        "current_bypassed": total_current,
        "baseline_bypassed": total_baseline,
        "delta": total_current - total_baseline,
        "verdict": "REGRESSION" if total_current > total_baseline else ("IMPROVED" if total_current < total_baseline else "STABLE"),
    })

    return has_regression, rows


def check_load(
    current: dict[str, dict[str, float]],
    baseline: dict[str, dict[str, float]],
    tolerance: float = DEFAULT_TOLERANCE,
) -> tuple[bool, list[dict[str, Any]]]:
    """Return (has_regression, rows). Regression = P95 > baseline * (1+tolerance)."""
    has_regression = False
    rows: list[dict[str, Any]] = []

    for label in sorted(set(current) | set(baseline)):
        cur = current.get(label, {})
        base = baseline.get(label, {})
        cur_p95 = cur.get("p95_ms", 0.0)
        base_p95 = base.get("p95_ms", 0.0)
        cur_err = cur.get("error_rate", 0.0)
        base_err = base.get("error_rate", 0.0)

        threshold = base_p95 * (1.0 + tolerance) if base_p95 > 0 else 999.0

        if cur_p95 > threshold:
            verdict = "REGRESSION"
            has_regression = True
        elif cur_p95 <= base_p95:
            verdict = "STABLE/IMPROVED"
        else:
            verdict = "WITHIN TOLERANCE"

        rows.append({
            "label": label,
            "current_p95_ms": cur_p95,
            "baseline_p95_ms": base_p95,
            "threshold_p95_ms": round(threshold, 2),
            "current_error_rate": cur_err,
            "baseline_error_rate": base_err,
            "verdict": verdict,
        })

    return has_regression, rows


# ── output ───────────────────────────────────────────────────────────────────


def print_table(adversarial_rows: list[dict[str, Any]], load_rows: list[dict[str, Any]]) -> None:
    """Pretty-print results to stdout (ASCII-safe, no emoji or box-drawing)."""
    sep = "=" * 80

    # Adversarial table
    print()
    print(sep)
    print("  ADVERSARIAL BYPASS REGRESSION CHECK")
    print(sep)
    header = f"  {'Category':<16} {'Cur Bypass':>11} {'Base Bypass':>11} {'Delta':>7}  Verdict"
    print(header)
    print("  " + "-" * 70)
    for r in adversarial_rows:
        delta_str = f"+{r['delta']}" if r["delta"] > 0 else str(r["delta"])
        verdict_icon = {"REGRESSION": "[FAIL]", "IMPROVED": "[OK]", "STABLE": "[-]"}.get(r["verdict"], "?")
        print(f"  {r['category']:<16} {r['current_bypassed']:>11} {r['baseline_bypassed']:>11} {delta_str:>7}  {verdict_icon} {r['verdict']}")

    # Load table
    print()
    print(sep)
    print("  LOAD TEST REGRESSION CHECK (P95, +/-20% tolerance)")
    print(sep)
    header2 = f"  {'Label':<12} {'Cur P95':>9} {'Base P95':>9} {'Threshold':>10} {'Cur Err%':>9}  Verdict"
    print(header2)
    print("  " + "-" * 76)
    for r in load_rows:
        verdict_icon = {"REGRESSION": "[FAIL]", "STABLE/IMPROVED": "[OK]", "WITHIN TOLERANCE": "[~]"}.get(r["verdict"], "?")
        print(f"  {r['label']:<12} {r['current_p95_ms']:>8.2f}ms {r['baseline_p95_ms']:>8.2f}ms {r['threshold_p95_ms']:>9.2f}ms {r['current_error_rate']*100:>8.2f}%  {verdict_icon} {r['verdict']}")
    print()


def main() -> int:
    parser = argparse.ArgumentParser(description="Tamga stress regression checker")
    parser.add_argument("--results-dir", required=True, help="Path to results/<timestamp>/ directory")
    parser.add_argument("--baseline", required=True, help="Path to baseline.json")
    parser.add_argument("--tolerance", type=float, default=DEFAULT_TOLERANCE, help="P95 tolerance (default: 0.20 = 20%%)")
    parser.add_argument("--json", action="store_true", help="Output machine-readable JSON to stdout")
    args = parser.parse_args()

    results_dir = Path(args.results_dir)
    baseline_path = Path(args.baseline)

    # ── load baseline ────────────────────────────────────────────────────
    if not baseline_path.exists():
        print(f"ERROR: baseline file not found: {baseline_path}", file=sys.stderr)
        return 2

    baseline = load_json(baseline_path)
    if not baseline:
        print("ERROR: baseline is empty or unparseable", file=sys.stderr)
        return 2

    # ── load current results ─────────────────────────────────────────────
    adv_current = load_adversarial_results(results_dir)
    load_current = load_load_results(results_dir)

    if not adv_current and not load_current:
        print(f"ERROR: No result files found in {results_dir}", file=sys.stderr)
        return 2

    # ── check ────────────────────────────────────────────────────────────
    adv_reg, adv_rows = check_adversarial(adv_current, baseline.get("adversarial", {}))
    load_reg, load_rows = check_load(load_current, baseline.get("load", {}), args.tolerance)

    has_regression = adv_reg or load_reg

    # ── output ───────────────────────────────────────────────────────────
    if args.json:
        output = {
            "has_regression": has_regression,
            "exit_code": 1 if has_regression else 0,
            "adversarial": adv_rows,
            "load": load_rows,
        }
        json.dump(output, sys.stdout, indent=2)
    else:
        print_table(adv_rows, load_rows)

        if has_regression:
            print("RESULT: REGRESSION DETECTED — see above for details\n")
        else:
            print("RESULT: STABLE or IMPROVED — no regression detected\n")

    return 1 if has_regression else 0


if __name__ == "__main__":
    sys.exit(main())
