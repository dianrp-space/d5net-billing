#!/usr/bin/env bash
# Update produksi: git pull → build → migrate → restart systemd.
#
# Default path aaPanel:
#   APP_DIR=/www/wwwroot/billing.dianrp.com   (repo git)
#   WEB_ROOT=$APP_DIR/web/dist                (arahkan root situs aaPanel ke sini)
#   binary Go  → /opt/drp-billing/current     (sesuai unit systemd)
#   env        → /etc/drp-billing/drp-billing.env
#
# Jalankan sebagai root:
#   sudo bash /www/wwwroot/billing.dianrp.com/deploy/scripts/update.sh
#
# Override:
#   APP_DIR=... WEB_ROOT=... BRANCH=main sudo -E bash deploy/scripts/update.sh
set -euo pipefail

APP_DIR="${APP_DIR:-/www/wwwroot/billing.dianrp.com}"
WEB_ROOT="${WEB_ROOT:-${APP_DIR}/web/dist}"
BIN_DIR="${BIN_DIR:-/opt/drp-billing/current}"
ENV_FILE="${ENV_FILE:-/etc/drp-billing/drp-billing.env}"
APP_USER="${APP_USER:-drp}"
BRANCH="${BRANCH:-main}"
REMOTE="${REMOTE:-origin}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8080/api/health}"
SERVICES="${SERVICES:-drp-api drp-worker}"

log() { printf '\n==> %s\n' "$*"; }
die() { printf '!! %s\n' "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "perintah '$1' tidak ditemukan. Pasang dulu, lalu ulangi."
}

database_url_from_env() {
  local f="$1" line
  [[ -f "$f" ]] || die "file env tidak ada: $f"
  line="$(grep -E '^[[:space:]]*DATABASE_URL=' "$f" | tail -n1 || true)"
  [[ -n "$line" ]] || die "DATABASE_URL tidak ada di $f"
  printf '%s\n' "${line#*=}"
}

# aaPanel / instalasi Go-Node umum
export PATH="/usr/local/go/bin:/usr/local/bin:${HOME}/go/bin:${PATH}"
shopt -s nullglob
for d in /www/server/nodejs/v*/bin; do
  export PATH="${d}:${PATH}"
done
shopt -u nullglob
unset GOROOT || true
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.27.1}"

[[ "$(id -u)" -eq 0 ]] || die "jalankan sebagai root (sudo)"
[[ -d "${APP_DIR}/.git" ]] || die "bukan git repo: ${APP_DIR}"
need_cmd git
need_cmd go
need_cmd npm
need_cmd rsync
need_cmd systemctl
need_cmd curl
need_cmd install

exec 9>"/tmp/drp-billing-update.lock"
flock -n 9 || die "update lain sedang berjalan"

cd "${APP_DIR}"

log "git fetch/pull ${REMOTE}/${BRANCH}"
git fetch --prune "${REMOTE}"
git checkout "${BRANCH}"
git pull --ff-only "${REMOTE}" "${BRANCH}"
git submodule update --init --recursive 2>/dev/null || true
printf '    HEAD: %s\n' "$(git log -1 --oneline)"

log "build frontend"
(
  cd web
  if [[ -f package-lock.json ]]; then
    npm ci
  else
    npm install
  fi
  npm run build
)

log "build Go (api, worker, migrate, drpctl)"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/drp-api ./cmd/api
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/drp-worker ./cmd/worker
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/drp-migrate ./cmd/migrate
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/drpctl ./cmd/drpctl

log "pasang binary ke ${BIN_DIR}"
mkdir -p "${BIN_DIR}" /var/lib/drp-billing/{uploads,exports,router-backups} /var/log/drp-billing
install -m 0755 bin/drp-api bin/drp-worker bin/drp-migrate bin/drpctl "${BIN_DIR}/"
rsync -a --delete migrations/ "${BIN_DIR}/migrations/"
if id "${APP_USER}" &>/dev/null; then
  chown -R "${APP_USER}:${APP_USER}" "${BIN_DIR}" /var/lib/drp-billing /var/log/drp-billing
fi

log "publish frontend → ${WEB_ROOT}"
mkdir -p "${WEB_ROOT}"
APP_REAL="$(readlink -f "${APP_DIR}")"
DIST_REAL="$(readlink -f "${APP_DIR}/web/dist")"
WEB_REAL="$(readlink -f "${WEB_ROOT}")"
if [[ "${WEB_REAL}" == "${DIST_REAL}" ]]; then
  printf '    skip rsync (WEB_ROOT = web/dist hasil build)\n'
elif [[ "${WEB_REAL}" == "${APP_REAL}" ]]; then
  printf '    WEB_ROOT = repo: salin index.html + assets (tanpa --delete)\n'
  rsync -a "${APP_DIR}/web/dist/" "${WEB_ROOT}/"
  printf '    peringatan: block akses /.git di Nginx jika root situs = repo\n'
else
  rsync -a --delete "${APP_DIR}/web/dist/" "${WEB_ROOT}/"
fi

log "migrate up"
DATABASE_URL="$(database_url_from_env "${ENV_FILE}")"
DATABASE_URL="${DATABASE_URL%\"}"
DATABASE_URL="${DATABASE_URL#\"}"
DATABASE_URL="${DATABASE_URL%\'}"
DATABASE_URL="${DATABASE_URL#\'}"
[[ -n "${DATABASE_URL}" ]] || die "DATABASE_URL kosong di ${ENV_FILE}"
MIGRATE_BIN="${BIN_DIR}/drp-migrate"
MIG_DIR="${BIN_DIR}/migrations"
if id "${APP_USER}" &>/dev/null; then
  sudo -u "${APP_USER}" env \
    DATABASE_URL="${DATABASE_URL}" \
    MIGRATIONS_DIR="${MIG_DIR}" \
    "${MIGRATE_BIN}" up
else
  DATABASE_URL="${DATABASE_URL}" MIGRATIONS_DIR="${MIG_DIR}" "${MIGRATE_BIN}" up
fi

log "restart systemd: ${SERVICES}"
# shellcheck disable=SC2086
systemctl restart ${SERVICES}
sleep 2
# shellcheck disable=SC2086
systemctl is-active --quiet ${SERVICES} || {
  # shellcheck disable=SC2086
  systemctl status ${SERVICES} --no-pager -l || true
  die "service tidak aktif setelah restart"
}

log "health check ${HEALTH_URL}"
ok=0
for i in 1 2 3 4 5 6; do
  if curl -sf "${HEALTH_URL}" >/dev/null; then
    ok=1
    break
  fi
  sleep 1
done
[[ "${ok}" -eq 1 ]] || die "health check gagal: ${HEALTH_URL}"

log "selesai"
printf '    situs  : %s\n' "${WEB_ROOT}"
printf '    binary : %s\n' "${BIN_DIR}"
printf '    unit   : %s\n' "${SERVICES}"
printf '    commit : %s\n' "$(git log -1 --oneline)"
printf '\nArahkan root situs aaPanel ke:\n  %s\n' "${WEB_ROOT}"
