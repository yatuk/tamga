#!/usr/bin/env python3
"""Analyze soak test memory timeline for leak detection."""
import sys

def analyze(filepath):
    try:
        with open(filepath) as f:
            lines = [l.strip() for l in f if l.strip()]
    except FileNotFoundError:
        print(f"File not found: {filepath}")
        return

    timestamps = []
    mems = []
    for line in lines:
        parts = line.split(":")
        if len(parts) >= 2:
            try:
                ts = int(parts[0].strip())
                mem = int(parts[1].strip().replace("MB", ""))
                timestamps.append(ts)
                mems.append(mem)
            except ValueError:
                continue

    if len(mems) < 2:
        print("Not enough data points for analysis")
        return

    first = mems[0]
    last = mems[-1]
    delta = last - first
    growth_rate = delta / len(mems) if len(mems) > 0 else 0

    print(f"Data points: {len(mems)}")
    print(f"Start: {first}MB | End: {last}MB | Delta: {delta:+d}MB")
    print(f"Growth rate: {growth_rate:+.1f}MB per sample")
    print(f"Min: {min(mems)}MB | Max: {max(mems)}MB | Avg: {sum(mems)//len(mems)}MB")

    # Simple leak detection: monotonic growth + significant delta
    if delta > 50 and growth_rate > 5:
        print("WARNING: Possible memory leak detected! (steady growth, >50MB delta)")
    elif delta > 100:
        print("CRITICAL: Large memory growth detected! (>100MB delta)")
    elif delta < 10:
        print("OK: Memory stable (delta < 10MB)")
    else:
        print("INCONCLUSIVE: Moderate growth, run longer soak to confirm")

    # Trend direction
    increasing = sum(1 for i in range(1, len(mems)) if mems[i] > mems[i-1])
    print(f"Trend: {increasing}/{len(mems)-1} samples increasing")


if __name__ == "__main__":
    filepath = sys.argv[1] if len(sys.argv) > 1 else "tests/stress/results/soak_memory_timeline.txt"
    analyze(filepath)
