#!/usr/bin/env python3
"""Derive the stress-suite policy from the shipped default policy.

The operator_state adversarial vectors need an authorization allowlist, which
the default policy leaves empty. Rather than keep a second policy file that can
drift from the default, this rewrites exactly that one line and fails loudly if
the default no longer contains it.

With --classifier the inline classifier is turned on as well, by adding a
classifier block to the scan section; the suite is then run against rules
plus classifier.

Usage:
    python make_stress_policy.py <default-policy.yaml> <output-policy.yaml> [--classifier]
"""

import sys
from pathlib import Path

MARKER = "  authorization: []"
REPLACEMENT = """  authorization:
    - decision_pattern: "D-.*"
      allowed_operators: ["mike", "yatuk"]"""


SCAN_MARKER = "scan:"
CLASSIFIER_BLOCK = """scan:
  classifier:
    enabled: true"""


def main() -> int:
    args = [a for a in sys.argv[1:] if a != "--classifier"]
    with_classifier = "--classifier" in sys.argv[1:]
    if len(args) != 2:
        print(__doc__, file=sys.stderr)
        return 2
    src, dst = Path(args[0]), Path(args[1])
    lines = src.read_text(encoding="utf-8").splitlines()
    hits = [i for i, line in enumerate(lines) if line.rstrip() == MARKER]
    if len(hits) != 1:
        print(
            f"ERROR: expected exactly one {MARKER.strip()!r} line in {src}, found {len(hits)}; "
            "update make_stress_policy.py to match the default policy",
            file=sys.stderr,
        )
        return 2
    lines[hits[0]] = REPLACEMENT
    if with_classifier:
        scan = [i for i, line in enumerate(lines) if line.rstrip() == SCAN_MARKER]
        if len(scan) != 1:
            print(f"ERROR: expected exactly one top-level 'scan:' line in {src}, found {len(scan)}", file=sys.stderr)
            return 2
        lines[scan[0]] = CLASSIFIER_BLOCK
    dst.parent.mkdir(parents=True, exist_ok=True)
    dst.write_text("\n".join(lines) + "\n", encoding="utf-8", newline="\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
