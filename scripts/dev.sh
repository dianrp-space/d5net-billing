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

# System Go may be older than the module toolchain (go.mod pins go1.27.1).
# Use the official downloaded toolchain; never mix with a forced GOROOT.
unset GOROOT
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.27.1}"
export PATH="/usr/local/go/bin:${PATH}"

API_ADDR="${HTTP_ADDR:-0.0.0.0:8087}"
API_PORT="${API_ADDR##*:}"
# Health check must hit a reachable loopback even when API binds 0.0.0.0
HEALTH_URL="http://127.0.0.1:${API_PORT}/api/health"
WEB_PORT=5173

LAN_IP="$(ip -4 -o addr show scope global 2>/dev/null | awk '/192\.168\.100\./{print $4; exit}' | cut -d/ -f1 || true)"
if [[ -z "${LAN_IP}" ]]; then
  LAN_IP="$(ip route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src") {print $(i+1); exit}}' || true)"
fi

echo "==> drp-billing dev"
echo "    Go:       $(go version 2>/dev/null || echo missing) (GOTOOLCHAIN=${GOTOOLCHAIN})"
echo "    API:      http://${API_ADDR}"
echo "    FE local: http://127.0.0.1:${WEB_PORT}"
if [[ -n "${LAN_IP}" ]]; then
  echo "    FE LAN:   http://${LAN_IP}:${WEB_PORT}   ← PC lain di Wi‑Fi"
fi
echo "    Ctrl+C to stop both"
echo

# Stale API/Vite on the same ports makes health checks pass while the new
# `go run` fails with "address already in use" — FE then talks to an old binary
# (e.g. missing /api/settings/isolir → 404).
free_port() {
  local port="$1"
  if command -v fuser >/dev/null 2>&1; then
    fuser -k "${port}/tcp" >/dev/null 2>&1 || true
  elif command -v lsof >/dev/null 2>&1; then
    lsof -ti ":${port}" | xargs -r kill -9 2>/dev/null || true
  fi
}
echo "==> freeing ports ${API_PORT} and ${WEB_PORT} (if busy)…"
free_port "$API_PORT"
free_port "$WEB_PORT"
sleep 1

echo "==> starting API…"
go run ./cmd/api &
API_PID=$!

WORKER_PID=""
if [[ "${WORKER_ENABLED:-true}" == "true" ]]; then
  echo "==> starting worker…"
  go run ./cmd/worker &
  WORKER_PID=$!
fi

# Wait until THIS API process is ready (not a leftover listener).
echo "==> waiting for API on 127.0.0.1:${API_PORT}"
for i in $(seq 1 90); do
  if ! kill -0 "$API_PID" 2>/dev/null; then
    echo "API process exited before becoming ready (check logs above — often port still busy)"
    exit 1
  fi
  if curl -sf "$HEALTH_URL" >/dev/null 2>&1; then
    # Confirm our process still owns the port shortly after health OK
    sleep 0.3
    if ! kill -0 "$API_PID" 2>/dev/null; then
      echo "API died right after health check (likely bind race with old process on :${API_PORT})"
      echo "    Run: fuser -k ${API_PORT}/tcp   then make dev again"
      exit 1
    fi
    echo "    API ready (${i}s) pid=${API_PID}"
    break
  fi
  if [[ "$i" -eq 90 ]]; then
    echo "API did not become ready within 90s"
    exit 1
  fi
  sleep 1
done

echo "==> starting Vite (0.0.0.0:${WEB_PORT})…"
(cd web && npm run dev -- --host 0.0.0.0 --port "${WEB_PORT}") &
WEB_PID=$!

wait "$API_PID" "$WEB_PID"
