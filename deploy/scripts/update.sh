#!/usr/bin/env bash
# Update produksi (idempotent): git pull → build jika ada perubahan → migrate → restart bila perlu.
#
# Semua path di folder situs (user dianrp):
#   APP_DIR=/www/wwwroot/delimanet.dianrp.com
#   WEB_ROOT=$APP_DIR/web/dist
#   binary    → $APP_DIR/bin
#   env       → $APP_DIR/.env
#   data      → $APP_DIR/data
#
# Jalankan sebagai root:
#   sudo bash /www/wwwroot/delimanet.dianrp.com/deploy/scripts/update.sh
#
# Paksa ulang: FORCE_NPM=1 FORCE_WEB=1 FORCE_GO=1 FORCE_RESTART=1
set -euo pipefail

APP_DIR="${APP_DIR:-/www/wwwroot/delimanet.dianrp.com}"
WEB_ROOT="${WEB_ROOT:-${APP_DIR}/web/dist}"
BIN_DIR="${BIN_DIR:-${APP_DIR}/bin}"
ENV_FILE="${ENV_FILE:-${APP_DIR}/.env}"
DATA_DIR="${DATA_DIR:-${APP_DIR}/data}"
CACHE_DIR="${CACHE_DIR:-${APP_DIR}/.update-cache}"
APP_USER="${APP_USER:-dianrp}"
BRANCH="${BRANCH:-main}"
REMOTE="${REMOTE:-origin}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8088/api/health}"
SERVICES="${SERVICES:-d5net-billing-api d5net-billing-worker}"
FORCE_NPM="${FORCE_NPM:-0}"
FORCE_WEB="${FORCE_WEB:-0}"
FORCE_GO="${FORCE_GO:-0}"
FORCE_RESTART="${FORCE_RESTART:-0}"

log() { printf '\n==> %s\n' "$*"; }
die() { printf '!! %s\n' "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "perintah '$1' tidak ditemukan. Pasang dulu, lalu ulangi."
}

# Root tidak punya akses repo GitHub private. Git/SSH memakai HOME + kunci APP_USER.
as_app() {
  if [[ -n "${SSH_AUTH_SOCK:-}" && -S "${SSH_AUTH_SOCK}" ]]; then
    sudo -u "${APP_USER}" -H --preserve-env=SSH_AUTH_SOCK env PATH="${PATH}" "$@"
  else
    sudo -u "${APP_USER}" -H env PATH="${PATH}" "$@"
  fi
}

database_url_from_env() {
  local f="$1" line
  [[ -f "$f" ]] || die "file env tidak ada: $f"
  line="$(grep -E '^[[:space:]]*DATABASE_URL=' "$f" | tail -n1 || true)"
  [[ -n "$line" ]] || die "DATABASE_URL tidak ada di $f"
  printf '%s\n' "${line#*=}"
}

paths_digest() {
  local f
  {
    for f in "$@"; do
      if [[ -f "$f" ]]; then
        sha256sum "$f"
      elif [[ -d "$f" ]]; then
        find "$f" -type f ! -path '*/node_modules/*' ! -path '*/dist/*' -print0 \
          | sort -z | xargs -0 -r sha256sum
      fi
    done
  } | sha256sum | awk '{print $1}'
}

stamp_ok() {
  local stamp="$1" digest="$2"
  [[ -f "$stamp" ]] && [[ "$(cat "$stamp")" == "$digest" ]]
}

write_stamp() {
  local stamp="$1" digest="$2"
  mkdir -p "$(dirname "$stamp")"
  printf '%s\n' "$digest" >"$stamp"
  chown "${APP_USER}:${APP_USER}" "$stamp" "$(dirname "$stamp")" 2>/dev/null || true
}

# Node/npm: nvm user situs (dianrp), aaPanel, lalu PATH sistem.
# `sudo` memakai HOME=/root, jadi nvm di /home/dianrp/.nvm tidak ketemu tanpa ini.
APP_HOME="$(getent passwd "${APP_USER}" | cut -d: -f6 || true)"
if [[ -n "${APP_HOME}" ]]; then
  shopt -s nullglob
  nvm_bins=("${APP_HOME}/.nvm/versions/node"/v*/bin)
  shopt -u nullglob
  if ((${#nvm_bins[@]} > 0)); then
    nvm_bin="$(printf '%s\n' "${nvm_bins[@]}" | sort -V | tail -n1)"
    export PATH="${nvm_bin}:${PATH}"
  fi
fi
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
id "${APP_USER}" &>/dev/null || die "user '${APP_USER}' tidak ada"
need_cmd git
need_cmd go
need_cmd npm
printf '    npm: %s\n' "$(command -v npm)"
need_cmd rsync
need_cmd systemctl
need_cmd curl
need_cmd sha256sum
need_cmd find

exec 9>"/tmp/d5net-billing-update.lock"
flock -n 9 || die "update lain sedang berjalan"

cd "${APP_DIR}"
mkdir -p "${CACHE_DIR}" "${BIN_DIR}" "${DATA_DIR}"/{uploads,exports,router-backups,whatsapp,db-backups}

log "git fetch/pull ${REMOTE}/${BRANCH} sebagai ${APP_USER}"
chown "${APP_USER}:${APP_USER}" "${APP_DIR}"
chown -R "${APP_USER}:${APP_USER}" "${APP_DIR}/.git"
as_app git -C "${APP_DIR}" fetch --prune "${REMOTE}"
as_app git -C "${APP_DIR}" checkout "${BRANCH}"
as_app git -C "${APP_DIR}" pull --ff-only "${REMOTE}" "${BRANCH}"
as_app git -C "${APP_DIR}" submodule update --init --recursive 2>/dev/null || true
printf '    HEAD: %s\n' "$(git log -1 --oneline)"

npm_digest="$(paths_digest web/package.json web/package-lock.json)"
web_digest="$(paths_digest web/package.json web/package-lock.json web/vite.config.ts web/tsconfig.json web/index.html web/src)"
go_digest="$(paths_digest go.mod go.sum cmd internal)"

rebuilt_web=0
rebuilt_go=0

log "frontend (npm + vite)"
if [[ "${FORCE_NPM}" != "1" ]] && [[ -d web/node_modules ]] && stamp_ok "${CACHE_DIR}/npm-ci" "${npm_digest}"; then
  printf '    skip npm ci (package-lock tidak berubah)\n'
else
  (
    cd web
    if [[ -f package-lock.json ]]; then
      as_app npm ci --prefix "${APP_DIR}/web"
    else
      as_app npm install --prefix "${APP_DIR}/web"
    fi
  )
  write_stamp "${CACHE_DIR}/npm-ci" "${npm_digest}"
fi

if [[ "${FORCE_WEB}" != "1" ]] && [[ -f "${APP_DIR}/web/dist/index.html" ]] && stamp_ok "${CACHE_DIR}/web-build" "${web_digest}"; then
  printf '    skip vite build (sumber frontend tidak berubah)\n'
else
  as_app npm run build --prefix "${APP_DIR}/web"
  write_stamp "${CACHE_DIR}/web-build" "${web_digest}"
  rebuilt_web=1
fi

log "Go (api, worker, migrate, drpctl) → ${BIN_DIR}"
need_go=0
if [[ "${FORCE_GO}" == "1" ]]; then
  need_go=1
elif ! stamp_ok "${CACHE_DIR}/go-build" "${go_digest}"; then
  need_go=1
else
  for b in d5net-billing-api d5net-billing-worker drp-migrate drpctl; do
    [[ -x "${BIN_DIR}/${b}" ]] || need_go=1
  done
fi
if [[ "${need_go}" -eq 0 ]]; then
  printf '    skip go build (sumber Go tidak berubah)\n'
else
  as_app env CGO_ENABLED=0 GOTOOLCHAIN="${GOTOOLCHAIN}" go build -trimpath -ldflags="-s -w" -o "${BIN_DIR}/d5net-billing-api" ./cmd/api
  as_app env CGO_ENABLED=0 GOTOOLCHAIN="${GOTOOLCHAIN}" go build -trimpath -ldflags="-s -w" -o "${BIN_DIR}/d5net-billing-worker" ./cmd/worker
  as_app env CGO_ENABLED=0 GOTOOLCHAIN="${GOTOOLCHAIN}" go build -trimpath -ldflags="-s -w" -o "${BIN_DIR}/drp-migrate" ./cmd/migrate
  as_app env CGO_ENABLED=0 GOTOOLCHAIN="${GOTOOLCHAIN}" go build -trimpath -ldflags="-s -w" -o "${BIN_DIR}/drpctl" ./cmd/drpctl
  chmod 0755 "${BIN_DIR}/d5net-billing-api" "${BIN_DIR}/d5net-billing-worker" "${BIN_DIR}/drp-migrate" "${BIN_DIR}/drpctl"
  write_stamp "${CACHE_DIR}/go-build" "${go_digest}"
  rebuilt_go=1
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

log "hak akses ${APP_USER}"
chown -R "${APP_USER}:${APP_USER}" "${BIN_DIR}" "${DATA_DIR}" "${CACHE_DIR}" "${APP_DIR}/web/dist" 2>/dev/null || true
if [[ -f "${ENV_FILE}" ]]; then
  chown "${APP_USER}:${APP_USER}" "${ENV_FILE}"
  chmod 600 "${ENV_FILE}"
fi

log "migrate up"
DATABASE_URL="$(database_url_from_env "${ENV_FILE}")"
DATABASE_URL="${DATABASE_URL%\"}"
DATABASE_URL="${DATABASE_URL#\"}"
DATABASE_URL="${DATABASE_URL%\'}"
DATABASE_URL="${DATABASE_URL#\'}"
[[ -n "${DATABASE_URL}" ]] || die "DATABASE_URL kosong di ${ENV_FILE}"
as_app env \
  DATABASE_URL="${DATABASE_URL}" \
  MIGRATIONS_DIR="${APP_DIR}/migrations" \
  "${BIN_DIR}/drp-migrate" up

if [[ "${FORCE_RESTART}" == "1" || "${rebuilt_go}" -eq 1 || "${rebuilt_web}" -eq 1 ]]; then
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
else
  log "skip restart (binary/frontend tidak berubah; FORCE_RESTART=1 untuk memaksa)"
  # shellcheck disable=SC2086
  systemctl is-active --quiet ${SERVICES} || {
    log "service belum aktif — start ${SERVICES}"
    # shellcheck disable=SC2086
    systemctl start ${SERVICES}
    sleep 2
  }
fi

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
printf '    env    : %s\n' "${ENV_FILE}"
printf '    data   : %s\n' "${DATA_DIR}"
printf '    unit   : %s\n' "${SERVICES}"
printf '    commit : %s\n' "$(git log -1 --oneline)"
printf '\nArahkan root situs aaPanel ke:\n  %s\n' "${WEB_ROOT}"
