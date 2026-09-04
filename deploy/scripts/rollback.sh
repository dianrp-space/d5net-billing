#!/usr/bin/env bash
set -euo pipefail
TARGET="${1:?Usage: rollback.sh <release-dir>}"
ln -sfn "$TARGET" /opt/drp-billing/current
systemctl restart drp-api drp-worker
curl -sf http://127.0.0.1:8080/api/health
echo "rolled back to $TARGET"
