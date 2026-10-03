#!/usr/bin/env python3
"""Regression checker for Tamga stress test results.

Compares current adversarial bypass counts and load test P95/error_rate
against a baseline JSON file. Used as the final gate in the stress suite.

Usage:
    python check_regression.py --results-dir results/20260617-120000 --baseline baseline.json
    python check_regression.py --results-dir results/20260617-120000 --baseline baseline.json --json

With --load-gate-levels (or STRESS_LOAD_GATE_LEVELS), e.g. "100rps", only the
listed load levels gate on P95; a P95 above its threshold at any other level is
reported but does not fail the check. P95 depends on the machine, so on a
shared CI runner the higher levels measure the runner, not the change.
Adversarial bypass counts and the load error rate gate at every level.

Exit codes:
    0 — stable or improved (no regression detected)
    1 — regression detected (current > baseline beyond tolerance)
    2 — baseline file missing or unreadable
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path
from typing import Any

DEFAULT_TOLERANCE = 0.20  # 20% headroom for load test P95
ERROR_RATE_TOLERANCE = 0.01  # absolute: 1 percentage point above baseline


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
        if fpath.name == "adversarial_results.json":
            continue  # the suite's own merged summary, not a per-category result
        data = load_json(fpath)
        cat = data.get("category", fpath.stem.replace("adversarial_", ""))
        merged[cat] = {
            "total": data.get("total", 0),
            "detected": data.get("detected", 0),
            "bypassed": data.get("bypassed", 0),
            "controls_failed": data.get("controls_failed", 0),
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
            p95, error_rate = extract_k6_summary(data)
        except (KeyError, TypeError):
            # Unreadable results must not pass as "0 ms, 0 errors": that would
            # score as an improvement over any baseline.
            print(f"ERROR: {fpath} is not a k6 summary this checker understands", file=sys.stderr)
            merged[label] = {"p95_ms": float("inf"), "error_rate": 1.0}
            continue
        merged[label] = {"p95_ms": round(float(p95), 2), "error_rate": round(float(error_rate), 4)}
    return merged


def extract_k6_summary(data: dict[str, Any]) -> tuple[float, float]:
    """Return (p95_ms, error_rate) from either k6 summary shape.

    `k6 run --summary-export` (what the suite uses) writes metrics flat:
        metrics.http_req_duration["p(95)"], metrics.http_req_failed["value"]
    A handleSummary() export nests them under "values":
        metrics.http_req_duration.values["p(95)"], metrics.http_req_failed.values["rate"]
    """
    duration = data["metrics"]["http_req_duration"]
    failed = data["metrics"]["http_req_failed"]
    if "values" in duration:
        return duration["values"]["p(95)"], failed["values"]["rate"]
    return duration["p(95)"], failed["value"]


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

        # A control is a legitimate request the suite expects to go through.
        # One that is refused is a false positive, whatever the baseline says.
        if cur.get("controls_failed", 0) > 0:
            verdict = "CONTROL FAILED"
            has_regression = True
        elif cur_bypassed > base_bypassed:
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
    gate_levels: set[str] | None = None,
) -> tuple[bool, list[dict[str, Any]]]:
    """Return (has_regression, rows).

    Regression = P95 > baseline * (1+tolerance), or the error rate rising more
    than ERROR_RATE_TOLERANCE above baseline. Only levels that were actually
    run are compared; a baseline level with no current result is skipped.

    gate_levels limits the P95 gate to the named levels (None = all). At any
    other level a P95 over its threshold is labelled "SLOW (advisory)" and does
    not count as a regression. Errors and unreadable results still do.
    """
    has_regression = False
    rows: list[dict[str, Any]] = []

    for label in sorted(current):
        cur = current.get(label, {})
        base = baseline.get(label, {})
        cur_p95 = cur.get("p95_ms", 0.0)
        base_p95 = base.get("p95_ms", 0.0)
        cur_err = cur.get("error_rate", 0.0)
        base_err = base.get("error_rate", 0.0)

        threshold = base_p95 * (1.0 + tolerance) if base_p95 > 0 else 999.0

        if cur_err > base_err + ERROR_RATE_TOLERANCE:
            # A run that mostly errors out returns fast; its P95 says nothing.
            verdict = "REGRESSION (errors)"
            has_regression = True
        elif cur_p95 == float("inf"):
            verdict = "REGRESSION"  # unreadable result file
            has_regression = True
        elif cur_p95 > threshold:
            if gate_levels is not None and label not in gate_levels:
                verdict = "SLOW (advisory)"
            else:
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


def print_table(
    adversarial_rows: list[dict[str, Any]],
    load_rows: list[dict[str, Any]],
    gate_levels: set[str] | None = None,
) -> None:
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
    scope = "all levels" if gate_levels is None else ", ".join(sorted(gate_levels)) or "no level"
    print(f"  LOAD TEST REGRESSION CHECK (P95 gate: {scope}; error rate: all levels)")
    print(sep)
    header2 = f"  {'Label':<12} {'Cur P95':>9} {'Base P95':>9} {'Threshold':>10} {'Cur Err%':>9}  Verdict"
    print(header2)
    print("  " + "-" * 76)
    for r in load_rows:
        verdict_icon = {"REGRESSION": "[FAIL]", "REGRESSION (errors)": "[FAIL]", "STABLE/IMPROVED": "[OK]", "WITHIN TOLERANCE": "[~]", "SLOW (advisory)": "[WARN]"}.get(r["verdict"], "?")
        print(f"  {r['label']:<12} {r['current_p95_ms']:>8.2f}ms {r['baseline_p95_ms']:>8.2f}ms {r['threshold_p95_ms']:>9.2f}ms {r['current_error_rate']*100:>8.2f}%  {verdict_icon} {r['verdict']}")
    print()


def annotate_github(adversarial_rows: list[dict[str, Any]], load_rows: list[dict[str, Any]]) -> None:
    """Emit GitHub Actions annotations so a result is readable from the run page."""
    for r in adversarial_rows:
        if r["verdict"] == "REGRESSION":
            print(f"::error title=Adversarial regression::{r['category']}: "
                  f"{r['current_bypassed']} bypassed, baseline {r['baseline_bypassed']}")
    for r in load_rows:
        detail = (f"{r['label']}: p95 {r['current_p95_ms']:.2f} ms, threshold {r['threshold_p95_ms']:.2f} ms, "
                  f"errors {r['current_error_rate'] * 100:.2f}%")
        if r["verdict"].startswith("REGRESSION"):
            print(f"::error title=Load regression::{detail}")
        elif r["verdict"] == "SLOW (advisory)":
            print(f"::warning title=Load P95 above threshold (advisory)::{detail}")


def main() -> int:
    parser = argparse.ArgumentParser(description="Tamga stress regression checker")
    parser.add_argument("--results-dir", required=True, help="Path to results/<timestamp>/ directory")
    parser.add_argument("--baseline", required=True, help="Path to baseline.json")
    parser.add_argument("--tolerance", type=float, default=DEFAULT_TOLERANCE, help="P95 tolerance (default: 0.20 = 20%%)")
    parser.add_argument("--json", action="store_true", help="Output machine-readable JSON to stdout")
    parser.add_argument(
        "--load-gate-levels",
        default=os.environ.get("STRESS_LOAD_GATE_LEVELS"),
        help="Comma-separated load levels that gate on P95, e.g. '100rps' "
             "(default: all; also STRESS_LOAD_GATE_LEVELS)",
    )
    args = parser.parse_args()

    gate_levels: set[str] | None = None
    if args.load_gate_levels:
        gate_levels = {lvl.strip() for lvl in args.load_gate_levels.split(",") if lvl.strip()}

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
    load_reg, load_rows = check_load(load_current, baseline.get("load", {}), args.tolerance, gate_levels)

    has_regression = adv_reg or load_reg

    # ── output ───────────────────────────────────────────────────────────
    if args.json:
        output = {
            "has_regression": has_regression,
            "load_gate_levels": sorted(gate_levels) if gate_levels is not None else None,
            "exit_code": 1 if has_regression else 0,
            "adversarial": adv_rows,
            "load": load_rows,
        }
        json.dump(output, sys.stdout, indent=2)
    else:
        print_table(adv_rows, load_rows, gate_levels)
        if os.environ.get("GITHUB_ACTIONS") == "true":
            annotate_github(adv_rows, load_rows)

        if has_regression:
            print("RESULT: REGRESSION DETECTED — see above for details\n")
        else:
            print("RESULT: STABLE or IMPROVED — no regression detected\n")

    return 1 if has_regression else 0


if __name__ == "__main__":
    sys.exit(main())
