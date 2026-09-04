#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ ! -f .env ]]; then
  echo "missing .env — copy from .env.example and configure DATABASE_URL"
  exit 1
fi

if [[ ! -d web/node_modules ]]; then
  echo "==> installing frontend dependencies"
  (cd web && npm install)
fi

cleanup() {
  trap - INT TERM
  jobs -p | xargs -r kill 2>/dev/null || true
  wait 2>/dev/null || true
}
trap cleanup INT TERM EXIT

# shellcheck disable=SC1091
set -a
# shellcheck source=/dev/null
source .env
set +a

API_ADDR="${HTTP_ADDR:-127.0.0.1:8080}"
API_HOST="${API_ADDR%:*}"
API_PORT="${API_ADDR##*:}"
HEALTH_URL="http://${API_ADDR}/api/health"

echo "==> drp-billing dev"
echo "    API: http://${API_ADDR}"
echo "    FE:  http://localhost:5173"
echo "    Ctrl+C to stop both"
echo

echo "==> starting API…"
go run ./cmd/api &
API_PID=$!

# Wait until API accepts connections so Vite proxy doesn't spam ECONNREFUSED.
echo "==> waiting for API on ${API_ADDR}"
for i in $(seq 1 60); do
  if ! kill -0 "$API_PID" 2>/dev/null; then
    echo "API process exited before becoming ready"
    exit 1
  fi
  if curl -sf "$HEALTH_URL" >/dev/null 2>&1; then
    echo "    API ready (${i}s)"
    break
  fi
  if [[ "$i" -eq 60 ]]; then
    echo "API did not become ready within 60s"
    exit 1
  fi
  sleep 1
done

echo "==> starting Vite…"
(cd web && npm run dev) &
WEB_PID=$!

wait "$API_PID" "$WEB_PID"
