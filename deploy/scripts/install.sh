#!/usr/bin/env bash
# Skrip tarball lama (/opt, /etc/drp-billing, user drp).
# Untuk billing.dianrp.com pakai: deploy/scripts/update.sh
set -euo pipefail

VERSION="${1:?Usage: install.sh <version>}"
APP_USER="drp"
APP_ROOT="/opt/drp-billing"
RELEASE_DIR="${APP_ROOT}/releases/${VERSION}"
WEB_ROOT="/www/wwwroot/drp-billing/web"
ENV_FILE="/etc/drp-billing/drp-billing.env"

echo "==> Installing drp-billing ${VERSION}"

id "${APP_USER}" &>/dev/null || useradd --system --home "${APP_ROOT}" --shell /usr/sbin/nologin "${APP_USER}"

mkdir -p "${RELEASE_DIR}" "${APP_ROOT}" /var/lib/drp-billing/{uploads,exports,router-backups} /var/log/drp-billing /etc/drp-billing
chown -R "${APP_USER}:${APP_USER}" "${APP_ROOT}" /var/lib/drp-billing /var/log/drp-billing

cp d5net-billing-api d5net-billing-worker "${RELEASE_DIR}/"
cp -r migrations "${RELEASE_DIR}/"
chmod +x "${RELEASE_DIR}/d5net-billing-api" "${RELEASE_DIR}/d5net-billing-worker"
chown -R "${APP_USER}:${APP_USER}" "${RELEASE_DIR}"

ln -sfn "${RELEASE_DIR}" "${APP_ROOT}/current"

if [[ -d web-dist ]]; then
  mkdir -p "${WEB_ROOT}"
  rsync -a --delete web-dist/ "${WEB_ROOT}/"
fi

if [[ ! -f "${ENV_FILE}" ]]; then
  cp .env.example "${ENV_FILE}"
  chmod 600 "${ENV_FILE}"
  echo "!! Edit ${ENV_FILE} before starting services"
fi

echo "==> Running migrations"
sudo -u "${APP_USER}" env $(grep -v '^#' "${ENV_FILE}" | xargs) \
  MIGRATIONS_DIR="${RELEASE_DIR}/migrations" \
  "${RELEASE_DIR}/d5net-billing-api" --help 2>/dev/null || true

export DATABASE_URL
DATABASE_URL=$(grep DATABASE_URL "${ENV_FILE}" | cut -d= -f2-)
MIGRATIONS_DIR="${RELEASE_DIR}/migrations" go run ./cmd/migrate up 2>/dev/null || \
  echo "Run migrations manually: MIGRATIONS_DIR=${RELEASE_DIR}/migrations DATABASE_URL=... ./drp-migrate up"

cp deploy/systemd/d5net-billing-api.service deploy/systemd/d5net-billing-worker.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable d5net-billing-api d5net-billing-worker
systemctl restart d5net-billing-api d5net-billing-worker

echo "==> Install complete. Configure Nginx via aaPanel using deploy/nginx/drp-billing.conf"
