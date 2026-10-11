#!/usr/bin/env bash
# End-to-end test of the LiteLLM bridge (docs/litellm.md).
#
# Builds the proxy image from this checkout, pulls the pinned LiteLLM image
# and a Python image for the fake model, and runs check.py inside the compose
# network. Needs Docker with the compose plugin.
#
#   bash deploy/e2e/litellm/run.sh
#   E2E_KEEP_STACK=1 bash deploy/e2e/litellm/run.sh   # leave the stack up
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
compose() { docker compose -f docker-compose.yml "$@"; }

finish() {
  status=$?
  if [ "$status" -ne 0 ]; then
    echo "---- container logs ----"
    compose logs --no-color --tail=200 || true
  fi
  if [ -z "${E2E_KEEP_STACK:-}" ]; then
    compose down --volumes --remove-orphans || true
  fi
  exit "$status"
}
trap finish EXIT

# Tamga and the fake model first: LiteLLM is given a Tamga key when it
# starts, and the key is made by the Tamga that is running.
compose up -d --build upstream tamga
compose exec -T upstream python /e2e/check.py wait tamga

TAMGA_KEY="$(compose exec -T upstream python /e2e/check.py create-key | tr -d '\r\n')"
if [ -z "$TAMGA_KEY" ]; then
  echo "no Tamga key was created" >&2
  exit 1
fi
# Keep the key out of the log of a CI run.
if [ -n "${GITHUB_ACTIONS:-}" ]; then
  echo "::add-mask::$TAMGA_KEY"
fi
export TAMGA_KEY

# --no-deps: starting LiteLLM must not touch Tamga. Compose recreated the
# Tamga container here on the first CI run, and with no database in this
# stack the key made above lived only in that container's memory, so every
# guardrail call was answered 401.
compose up -d --no-deps litellm
compose exec -T upstream python /e2e/check.py wait litellm
compose exec -T upstream python /e2e/check.py run
