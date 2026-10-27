#!/usr/bin/env bash
# Run govulncheck on the proxy, leaving out the findings listed in
# proxy/govulncheck.ignore.
#
#   scripts/govulncheck.sh [govulncheck flags...]
#
# Fails when the code calls a vulnerable symbol, whether or not its ID is in
# the ignore file: an entry there only hides a finding in a required module
# or an imported package that the code does not call. The govulncheck binary
# is taken from $GOVULNCHECK, or from PATH. Needs jq.

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ignore_file="$root/proxy/govulncheck.ignore"
govulncheck="${GOVULNCHECK:-govulncheck}"

out="$(mktemp)"
trap 'rm -f "$out"' EXIT

# With -format json govulncheck exits 0 whatever it finds; a non-zero exit
# means the scan itself failed.
(cd "$root/proxy" && "$govulncheck" -format json "$@" ./...) > "$out"

ignored="$(grep -v '^[[:space:]]*\(#\|$\)' "$ignore_file" | awk '{print $1}' | sort -u)"

# One line per ID: the ID, then "called" when any trace reaches a function
# in this code, otherwise "not-called".
findings="$(jq -r '
  select(.finding) | .finding
  | [.osv, (if (.trace[0].function // "") != "" then "called" else "not-called" end)]
  | @tsv' "$out" | sort -u | awk -F'\t' '
  { if ($2 == "called") s[$1] = "called"; else if (!($1 in s)) s[$1] = "not-called" }
  END { for (id in s) print id "\t" s[id] }' | sort)"

called=0
shown=0
hidden=0
while IFS=$'\t' read -r id state; do
  [ -n "$id" ] || continue
  if [ "$state" = "called" ]; then
    called=$((called + 1))
    echo "CALLED      $id  https://pkg.go.dev/vuln/$id"
  elif grep -qx "$id" <<<"$ignored"; then
    hidden=$((hidden + 1))
  else
    shown=$((shown + 1))
    echo "not called  $id  https://pkg.go.dev/vuln/$id"
  fi
done <<<"$findings"

while read -r id; do
  [ -n "$id" ] || continue
  if ! cut -f1 <<<"$findings" | grep -qx "$id"; then
    echo "note: $id is in proxy/govulncheck.ignore but no longer found; remove it."
  fi
done <<<"$ignored"

echo "govulncheck: $called called, $shown not called, $hidden left out by proxy/govulncheck.ignore."
if [ "$called" -gt 0 ]; then
  echo "Run govulncheck -show traces ./... in proxy/ for where the code calls them."
  exit 3
fi
