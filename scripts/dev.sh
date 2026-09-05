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

# System Go may be 1.25.x while whatsmeow needs >=1.26 / toolchain 1.27.
# Use the official downloaded toolchain; never mix with a forced GOROOT.
unset GOROOT
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.27.1}"
export PATH="/usr/local/go/bin:${PATH}"

API_ADDR="${HTTP_ADDR:-0.0.0.0:8080}"
API_PORT="${API_ADDR##*:}"
# Health check must hit a reachable loopback even when API binds 0.0.0.0
HEALTH_URL="http://127.0.0.1:${API_PORT}/api/health"

LAN_IP="$(ip -4 -o addr show scope global 2>/dev/null | awk '/192\.168\.100\./{print $4; exit}' | cut -d/ -f1 || true)"
if [[ -z "${LAN_IP}" ]]; then
  LAN_IP="$(ip route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src") {print $(i+1); exit}}' || true)"
fi

echo "==> drp-billing dev"
echo "    Go:       $(go version 2>/dev/null || echo missing) (GOTOOLCHAIN=${GOTOOLCHAIN})"
echo "    API:      http://${API_ADDR}"
echo "    FE local: http://127.0.0.1:5173"
if [[ -n "${LAN_IP}" ]]; then
  echo "    FE LAN:   http://${LAN_IP}:5173   ← PC lain di Wi‑Fi"
fi
echo "    Ctrl+C to stop both"
echo

echo "==> starting API…"
go run ./cmd/api &
API_PID=$!

# Wait until API accepts connections so Vite proxy doesn't spam ECONNREFUSED.
echo "==> waiting for API on 127.0.0.1:${API_PORT}"
for i in $(seq 1 90); do
  if ! kill -0 "$API_PID" 2>/dev/null; then
    echo "API process exited before becoming ready"
    exit 1
  fi
  if curl -sf "$HEALTH_URL" >/dev/null 2>&1; then
    echo "    API ready (${i}s)"
    break
  fi
  if [[ "$i" -eq 90 ]]; then
    echo "API did not become ready within 90s"
    exit 1
  fi
  sleep 1
done

echo "==> starting Vite (0.0.0.0:5173)…"
(cd web && npm run dev -- --host 0.0.0.0 --port 5173) &
WEB_PID=$!

wait "$API_PID" "$WEB_PID"
