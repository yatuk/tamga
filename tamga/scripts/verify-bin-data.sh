#!/usr/bin/env bash
# verify-bin-data.sh — SHA256 integrity check for embedded BIN dataset
# Called from CI before go build. Fails if the CSV has been modified
# without updating the expected hash in bindata/embed.go.

set -euo pipefail

CSV_FILE="${1:-proxy/internal/scanner/bindata/binlist.csv}"
EXPECTED="f72afb84a444064972d6119de2098488c06b975b54910b6f220c7a8a4cf11ffd"

if [ ! -f "$CSV_FILE" ]; then
    echo "ERROR: BIN dataset not found at $CSV_FILE"
    exit 1
fi

ACTUAL=$(sha256sum "$CSV_FILE" | awk '{print $1}')

if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "ERROR: BIN dataset SHA256 mismatch!"
    echo "  Expected: $EXPECTED"
    echo "  Actual:   $ACTUAL"
    echo ""
    echo "The BIN dataset has been modified. If this is intentional:"
    echo "  1. Review the change per docs/BIN_DATA_UPDATE_RUNBOOK.md"
    echo "  2. Update ExpectedSHA256 in proxy/internal/scanner/bindata/embed.go"
    exit 1
fi

echo "OK: BIN dataset SHA256 verified ($CSV_FILE)"
