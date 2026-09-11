#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:?Usage: rollback.sh <release-dir>}"
ln -sfn "$TARGET" /opt/drp-billing/current
systemctl restart d5net-billing-api d5net-billing-worker
curl -sf http://127.0.0.1:8088/api/health
echo "rolled back to $TARGET"
