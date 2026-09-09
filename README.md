# drp-billing

Aplikasi billing ISP fullstack untuk jaringan MikroTik (PPPoE/Hotspot).

- Backend: Go (chi + huma + pgx)
- Frontend: React 19 + Vite + Tailwind + ECharts
- Database: PostgreSQL
- Deployment: Linux + aaPanel + Nginx + systemd (tanpa Docker Compose)

## Pengembangan

```bash
cp .env.example .env
# set DATABASE_URL, JWT_SECRET, ENCRYPTION_KEY (32 byte)
make migrate-up
make run-api
# terminal lain:
cd web && npm install && npm run dev
```

Vite listen di `0.0.0.0:5173`. PC lain di Wi‑Fi `192.168.100.0/24` bisa buka `http://<IP-laptop>:5173` (contoh `http://192.168.100.67:5173`). API default `0.0.0.0:8080`; frontend mem-proxy `/api`, `/uploads`, `/events`.

Pastikan firewall mengizinkan port **5173** (dan **8080** jika API dipanggil langsung):

```bash
sudo ufw allow from 192.168.100.0/24 to any port 5173 proto tcp
sudo ufw allow from 192.168.100.0/24 to any port 8080 proto tcp
```

Buat superadmin platform + tenant pertama:

```bash
go run ./cmd/drpctl create-platform-admin --email super@demo.local --password rahasia123 --full-name "Platform Admin"
go run ./cmd/drpctl create-tenant --slug demo --name "ISP Demo" --email admin@demo.local --password rahasia123 --full-name Admin
```

| Path | Fungsi |
|------|--------|
| `/` | Landing |
| `/login` | Superadmin (kelola tenant) |
| `/<slug>/login` | Login admin tenant |
| `/<slug>/client/login` | Portal pelanggan tenant |

Contoh: `/demo/login`, `/demo/client/login`

Go **tidak hot-reload**. Setelah ubah kode backend, restart proses API/worker (di produksi: `systemctl restart drp-api drp-worker`).

## Produksi (aaPanel + systemd)

Alur yang dipakai repo ini: **clone git di wwwroot**, frontend di `web/dist`, binary Go di `/opt/drp-billing/current`, dijalankan **systemd** (`drp-api` + `drp-worker`). Nginx hanya static + reverse proxy.

`deploy/scripts/install.sh` adalah sisa alur tarball (`make release`) dengan path web lama (`/www/wwwroot/drp-billing/web`) dan migrate yang tidak lengkap. **Jangan dipakai** untuk server ini. Pakai langkah di bawah + [`deploy/scripts/update.sh`](deploy/scripts/update.sh).

### Layout

| Path | Isi |
|------|-----|
| `/www/wwwroot/billing.dianrp.com` | Clone git |
| `/www/wwwroot/billing.dianrp.com/web/dist` | Root situs aaPanel / Nginx (`index.html`) |
| `/opt/drp-billing/current` | `drp-api`, `drp-worker`, `drp-migrate`, `drpctl`, `migrations/` |
| `/etc/drp-billing/drp-billing.env` | Secret produksi (chmod 600) |
| `/var/lib/drp-billing/` | Upload, backup router, sesi WhatsApp, backup DB |
| `/etc/systemd/system/drp-api.service` | Unit API — [`deploy/systemd/drp-api.service`](deploy/systemd/drp-api.service) |
| `/etc/systemd/system/drp-worker.service` | Unit worker — [`deploy/systemd/drp-worker.service`](deploy/systemd/drp-worker.service) |

Unit systemd memakai `ProtectSystem=strict` dan hanya boleh tulis ke `/var/lib/drp-billing` serta `/var/log/drp-billing`. Jangan taruh sesi WhatsApp atau backup di `./data/...` relatif ke binary.

### Prasyarat

- aaPanel: Nginx, PostgreSQL 16+ (atau 14+), SSL Let's Encrypt
- Di server: `git`, **Go 1.26+** (toolchain `go1.27.1`), **Node.js 20+** + `npm`, `rsync`, `curl`
- User sistem `drp` (dibuat di langkah 2)

### 1. Database

Di aaPanel → PostgreSQL, buat database + user (contoh `drp_billing` / `drp`). Catat DSN:

```
postgres://USER:PASSWORD@127.0.0.1:5432/drp_billing?sslmode=disable
```

### 2. User dan direktori

```bash
sudo useradd --system --home /opt/drp-billing --shell /usr/sbin/nologin drp || true
sudo mkdir -p /opt/drp-billing/current /etc/drp-billing \
  /var/lib/drp-billing/{uploads,exports,router-backups,whatsapp,db-backups} \
  /var/log/drp-billing
sudo chown -R drp:drp /opt/drp-billing /var/lib/drp-billing /var/log/drp-billing
```

### 3. Clone repo

```bash
sudo mkdir -p /www/wwwroot
sudo git clone git@github.com:dianrp-space/drp-billing.git /www/wwwroot/billing.dianrp.com
# atau HTTPS:
# sudo git clone https://github.com/dianrp-space/drp-billing.git /www/wwwroot/billing.dianrp.com
```

User yang menjalankan `update.sh` (root) harus bisa `git pull` (deploy key / credential).

### 4. File env

```bash
sudo cp /www/wwwroot/billing.dianrp.com/.env.example /etc/drp-billing/drp-billing.env
sudo chmod 640 /etc/drp-billing/drp-billing.env
sudo chown root:drp /etc/drp-billing/drp-billing.env
sudo nano /etc/drp-billing/drp-billing.env
```

Isi minimal (sesuaikan):

```bash
APP_ENV=production
HTTP_ADDR=127.0.0.1:8080
DATABASE_URL=postgres://USER:PASSWORD@127.0.0.1:5432/drp_billing?sslmode=disable
JWT_SECRET='<acak panjang, openssl rand -hex 32>'
ENCRYPTION_KEY='<tepat 32 karakter, openssl rand -base64 24 | cut -c1-32>'
CORS_ORIGINS=https://billing.dianrp.com
UPLOAD_DIR=/var/lib/drp-billing/uploads
ROUTER_BACKUP_DIR=/var/lib/drp-billing/router-backups
WHATSAPP_SESSION_DIR=/var/lib/drp-billing/whatsapp
DB_BACKUP_DIR=/var/lib/drp-billing/db-backups
WORKER_ENABLED=true
```

`ENCRYPTION_KEY` wajib **32 byte** (32 karakter). Ganti `JWT_SECRET` dari contoh. Setelah ubah env: `sudo systemctl restart drp-api drp-worker`.

### 5. systemd

```bash
sudo cp /www/wwwroot/billing.dianrp.com/deploy/systemd/drp-api.service /etc/systemd/system/
sudo cp /www/wwwroot/billing.dianrp.com/deploy/systemd/drp-worker.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable drp-api drp-worker
```

Jangan `start` dulu sebelum binary ada (langkah 6).

### 6. Build, migrate, start

```bash
sudo bash /www/wwwroot/billing.dianrp.com/deploy/scripts/update.sh
```

Skrip ini: `git pull` → `npm ci` + Vite build → compile Go → pasang binary ke `/opt/drp-billing/current` → `migrate up` → restart service → `GET /api/health`.

Cek:

```bash
curl -sf http://127.0.0.1:8080/api/health
sudo systemctl status drp-api drp-worker --no-pager
```

### 7. Situs Nginx (aaPanel)

1. Buat website `billing.dianrp.com`, aktifkan SSL Let's Encrypt.
2. Set **website root** ke `/www/wwwroot/billing.dianrp.com/web/dist` (bukan folder git).
3. Gabungkan reverse proxy dari template [`deploy/nginx/drp-billing.conf`](deploy/nginx/drp-billing.conf) (lokasi `/api/`, `/uploads/`, `/events/`, `try_files` SPA). Reload Nginx.

| Lokasi | Peran |
|--------|--------|
| `/` | Static SPA; fallback `index.html` untuk `/<slug>/...` |
| `/api/` | Proxy ke `127.0.0.1:8080` |
| `/uploads/` | Proxy file branding |
| `/events/` | SSE (buffering off) |

Jangan expose `.git`. Template sudah `deny` path itu.

### 8. Akun pertama

`drpctl` ikut dipasang oleh `update.sh`. Load env lalu buat platform admin + tenant:

```bash
sudo -u drp bash -c '
  set -a
  source /etc/drp-billing/drp-billing.env
  set +a
  /opt/drp-billing/current/drpctl create-platform-admin \
    --email super@dianrp.com --password "GANTI" --full-name "Platform Admin"
  /opt/drp-billing/current/drpctl create-tenant \
    --slug demo --name "ISP Demo" --email admin@demo.local --password "GANTI" --full-name Admin
'
```

| URL | Fungsi |
|-----|--------|
| `https://billing.dianrp.com/login` | Superadmin |
| `https://billing.dianrp.com/<slug>/login` | Admin tenant |
| `https://billing.dianrp.com/<slug>/client/login` | Portal pelanggan |

### 9. Update berikutnya

```bash
sudo bash /www/wwwroot/billing.dianrp.com/deploy/scripts/update.sh
```

Override path jika perlu: `APP_DIR=... WEB_ROOT=... BRANCH=main sudo -E bash .../update.sh`.

Rollback tarball lama: [`deploy/scripts/rollback.sh`](deploy/scripts/rollback.sh) (symlink `/opt/drp-billing/current`). Untuk git, `git checkout` commit sebelumnya lalu jalankan `update.sh` lagi.

### 10. Log

```bash
sudo journalctl -u drp-api -u drp-worker -f
```

Log Nginx: `/www/wwwlogs/billing.dianrp.com.*.log` (sesuai template).

### MikroTik

Enable API: `/ip service enable api` — port **8728** (TLS **8729**). Isi alamat, user, password di menu Router. Interval poller API (sesi/CPU) diatur di **Cronjob**, terpisah dari interval tagihan/isolir.

## Nginx: cuplikan

Template penuh: [`deploy/nginx/drp-billing.conf`](deploy/nginx/drp-billing.conf).

```nginx
root /www/wwwroot/billing.dianrp.com/web/dist;
index index.html;
client_max_body_size 20m;

location / {
    try_files $uri $uri/ /index.html;
}

location /api/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 120s;
}

location /uploads/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

location /events/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Connection '';
    proxy_buffering off;
    proxy_cache off;
    chunked_transfer_encoding off;
}
```

## Integrasi (admin tenant)

Di menu **Integrasi**:

- **Payment Gateway** — DRP Payment (QRIS), kredensial per tenant atau env `DRP_PAYMENT_*`
- **Messaging Gateway** — tab **WhatsApp** (pairing QR via [whatsmeow](https://github.com/tulir/whatsmeow), ke pelanggan) dan tab **Telegram** (bot token + chat ID **ops tenant** saja)
- **Backup / Restore** — tenant: export/import JSON data tenant; platform/owner: `pg_dump` / `psql` penuh (dir `DB_BACKUP_DIR`)

Sesi WhatsApp produksi: `WHATSAPP_SESSION_DIR=/var/lib/drp-billing/whatsapp`. Setelah Connect + scan QR, notifikasi invoice/pembayaran memakai sesi tersebut.

## Fitur

Pelanggan, paket, langganan, invoice, pembayaran manual/gateway, isolir otomatis, portal pelanggan, voucher, tiket, MAP FTTH, coverage, monitoring, notifikasi WA/Telegram/email, RADIUS opsional, import pelanggan MySQL legacy.
