#!/usr/bin/env bash
# Run a check in CI and report what it found without failing the job, unless
# the gate is switched on.
#
#   GATE_MODE=report  scripts/ci-report-step.sh "npm audit (dashboard)" -- npm audit --omit=dev
#   GATE_MODE=enforce scripts/ci-report-step.sh "govulncheck" -- govulncheck ./...
#
# report  (the default): the command's output is printed and put in the job
#          summary; a non-zero exit becomes a warning annotation and this
#          script exits 0.
# enforce: the same, and this script exits with the command's exit code.
#
# A command that cannot be started at all (exit 126 or 127) fails in both
# modes: a check that did not run is not a clean result.

set -uo pipefail

title="${1:?usage: ci-report-step.sh <title> -- <command...>}"
shift
[ "${1:-}" = "--" ] && shift
[ "$#" -gt 0 ] || { echo "ci-report-step: no command given" >&2; exit 2; }

mode="${GATE_MODE:-report}"
case "$mode" in
  report|enforce) ;;
  *) echo "ci-report-step: GATE_MODE must be report or enforce, got '$mode'" >&2; exit 2 ;;
esac

out="$(mktemp)"
trap 'rm -f "$out"' EXIT

"$@" 2>&1 | tee "$out"
code="${PIPESTATUS[0]}"

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### ${title}"
    echo
    if [ "$code" -eq 0 ]; then
      echo "Exit 0: nothing reported."
    else
      echo "Exit ${code} (${mode} mode). Last 150 lines of output:"
      echo
      echo '```text'
      tail -n 150 "$out"
      echo '```'
    fi
    echo
  } >> "$GITHUB_STEP_SUMMARY"
fi

if [ "$code" -eq 126 ] || [ "$code" -eq 127 ]; then
  echo "::error title=${title}::the check could not be started (exit ${code})"
  exit "$code"
fi

if [ "$code" -ne 0 ]; then
  if [ "$mode" = "enforce" ]; then
    echo "::error title=${title}::exit ${code}; see the job log"
    exit "$code"
  fi
  echo "::warning title=${title}::exit ${code} in report mode: the job does not fail. See the job summary."
fi
exit 0
