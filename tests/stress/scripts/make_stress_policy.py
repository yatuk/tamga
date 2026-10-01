#!/usr/bin/env python3
"""Derive the stress-suite policy from the shipped default policy.

The operator_state adversarial vectors need an authorization allowlist, which
the default policy leaves empty. Rather than keep a second policy file that can
drift from the default, this rewrites exactly that one line and fails loudly if
the default no longer contains it.

Usage:
    python make_stress_policy.py <default-policy.yaml> <output-policy.yaml>
"""

import sys
from pathlib import Path

MARKER = "  authorization: []"
REPLACEMENT = """  authorization:
    - decision_pattern: "D-.*"
      allowed_operators: ["mike", "yatuk"]"""


def main() -> int:
    if len(sys.argv) != 3:
        print(__doc__, file=sys.stderr)
        return 2
    src, dst = Path(sys.argv[1]), Path(sys.argv[2])
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
    dst.parent.mkdir(parents=True, exist_ok=True)
    dst.write_text("\n".join(lines) + "\n", encoding="utf-8", newline="\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
