#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:?Usage: upgrade.sh <version>}"
PREVIOUS=$(readlink -f /opt/drp-billing/current 2>/dev/null || echo "")

echo "==> Upgrading to ${VERSION}"
bash deploy/scripts/install.sh "${VERSION}"

export DATABASE_URL
DATABASE_URL=$(grep DATABASE_URL /etc/drp-billing/drp-billing.env | cut -d= -f2-)
MIGRATIONS_DIR="/opt/drp-billing/current/migrations" \
  DATABASE_URL="${DATABASE_URL}" \
  /opt/drp-billing/current/d5net-billing-api --version 2>/dev/null || true

if ! curl -sf http://127.0.0.1:8088/api/health > /dev/null; then
  echo "!! Health check failed, rolling back"
  if [[ -n "${PREVIOUS}" ]]; then
    ln -sfn "${PREVIOUS}" /opt/drp-billing/current
    systemctl restart d5net-billing-api d5net-billing-worker
  fi
  exit 1
fi

echo "==> Upgrade successful (tarball). Untuk update dari git: deploy/scripts/update.sh"
