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

Vite listen di `0.0.0.0:5173`. PC lain di Wi‑Fi `192.168.100.0/24` bisa buka `http://<IP-laptop>:5173` (contoh `http://192.168.100.67:5173`). API default `0.0.0.0:8087`; frontend mem-proxy `/api`, `/uploads`, `/events`.

Pastikan firewall mengizinkan port **5173** (dan **8087** jika API dipanggil langsung):

```bash
sudo ufw allow from 192.168.100.0/24 to any port 5173 proto tcp
sudo ufw allow from 192.168.100.0/24 to any port 8087 proto tcp
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

Semua file aplikasi ada di **satu folder situs**, dijalankan user **`dianrp`** (bukan user sistem `drp` baru). Nginx root hanya `web/dist`, jadi `.env`, `data/`, `bin/`, dan `.git` tidak ikut ter-serve.

Yang tetap di luar webroot hanya unit systemd (`/etc/systemd/system/`) — systemd memang wajib baca unit dari situ — dan PostgreSQL/Nginx milik aaPanel.

`deploy/scripts/install.sh` adalah sisa alur tarball lama. **Jangan dipakai.** Pakai langkah di bawah + [`deploy/scripts/update.sh`](deploy/scripts/update.sh).

### Layout

| Path | Isi |
|------|-----|
| `/www/wwwroot/billing.dianrp.com` | Clone git (working dir proses) |
| `web/dist/` | Root situs aaPanel / Nginx (`index.html`) |
| `bin/` | `drp-api`, `drp-worker`, `drp-migrate`, `drpctl` |
| `.env` | Secret produksi (chmod 600, gitignored) |
| `data/` | Upload, backup router, sesi WhatsApp, backup DB |
| `/etc/systemd/system/drp-api.service` | Unit API — [`deploy/systemd/drp-api.service`](deploy/systemd/drp-api.service) |
| `/etc/systemd/system/drp-worker.service` | Unit worker — [`deploy/systemd/drp-worker.service`](deploy/systemd/drp-worker.service) |

Unit systemd memakai `ProtectSystem=strict` dan hanya boleh tulis ke `data/`. Log proses lewat `journalctl` (bukan `/var/log/drp-billing`).

### Prasyarat

- aaPanel: Nginx, PostgreSQL 16+ (atau 14+), SSL Let's Encrypt
- Di server: `git`, **Go 1.26+** (toolchain `go1.27.1`), **Node.js 20+** + `npm`, `rsync`, `curl`
- User **`dianrp`** (sudah ada; proses API/worker jalan sebagai user ini)

### 1. Database

Di aaPanel → PostgreSQL, buat database + user. Catat DSN:

```
postgres://USER:PASSWORD@127.0.0.1:5432/drp_billing?sslmode=disable
```

### 2. Clone repo

```bash
sudo mkdir -p /www/wwwroot
sudo git clone git@github.com:dianrp-space/drp-billing.git /www/wwwroot/billing.dianrp.com
# atau HTTPS:
# sudo git clone https://github.com/dianrp-space/drp-billing.git /www/wwwroot/billing.dianrp.com
sudo chown -R dianrp:dianrp /www/wwwroot/billing.dianrp.com
```

User root yang menjalankan `update.sh` harus bisa `git pull` (deploy key / credential).

### 3. File env

```bash
sudo -u dianrp cp /www/wwwroot/billing.dianrp.com/.env.example /www/wwwroot/billing.dianrp.com/.env
sudo chmod 600 /www/wwwroot/billing.dianrp.com/.env
sudo nano /www/wwwroot/billing.dianrp.com/.env
```

Isi minimal (sesuaikan). Path data relatif ke folder repo:

```bash
APP_ENV=production
HTTP_ADDR=127.0.0.1:8087
DATABASE_URL=postgres://USER:PASSWORD@127.0.0.1:5432/drp_billing?sslmode=disable
JWT_SECRET='<acak panjang, openssl rand -hex 32>'
ENCRYPTION_KEY='<tepat 32 karakter, openssl rand -base64 24 | cut -c1-32>'
CORS_ORIGINS=https://billing.dianrp.com
UPLOAD_DIR=./data/uploads
ROUTER_BACKUP_DIR=./data/router-backups
WHATSAPP_SESSION_DIR=./data/whatsapp
DB_BACKUP_DIR=./data/db-backups
WORKER_ENABLED=true
```

`ENCRYPTION_KEY` wajib **32 byte** (32 karakter). Ganti `JWT_SECRET` dari contoh. Setelah ubah env: `sudo systemctl restart drp-api drp-worker`.

### 4. systemd

```bash
sudo cp /www/wwwroot/billing.dianrp.com/deploy/systemd/drp-api.service /etc/systemd/system/
sudo cp /www/wwwroot/billing.dianrp.com/deploy/systemd/drp-worker.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable drp-api drp-worker
```

Jangan `start` dulu sebelum binary ada (langkah 5). Kalau unit lama masih memakai `/opt` + `/etc/drp-billing`, timpa dengan file di atas lalu `daemon-reload`.

### 5. Build, migrate, start

```bash
sudo bash /www/wwwroot/billing.dianrp.com/deploy/scripts/update.sh
```

Skrip ini: `git pull` → `npm ci` + Vite build → compile Go ke `bin/` → `migrate up` → restart service → `GET /api/health`.

Cek:

```bash
curl -sf http://127.0.0.1:8087/api/health
sudo systemctl status drp-api drp-worker --no-pager
```

### 6. Situs Nginx (aaPanel)

1. Buat website `billing.dianrp.com`, aktifkan SSL Let's Encrypt.
2. Set **website root** ke `/www/wwwroot/billing.dianrp.com/web/dist` (bukan folder git).
3. Gabungkan reverse proxy dari template [`deploy/nginx/drp-billing.conf`](deploy/nginx/drp-billing.conf) (lokasi `/api/`, `/uploads/`, `/events/`, `try_files` SPA, proxy ke **8087**). Reload Nginx.

| Lokasi | Peran |
|--------|--------|
| `/` | Static SPA; fallback `index.html` untuk `/<slug>/...` |
| `/api/` | Proxy ke `127.0.0.1:8087` |
| `/uploads/` | Proxy file branding |
| `/events/` | SSE (buffering off) |

Jangan expose `.git`. Template sudah `deny` path itu.

### 7. Akun pertama

`drpctl` ada di `bin/` setelah `update.sh`. Load env lalu buat platform admin + tenant:

```bash
sudo -u dianrp bash -c '
  set -a
  source /www/wwwroot/billing.dianrp.com/.env
  set +a
  cd /www/wwwroot/billing.dianrp.com
  ./bin/drpctl create-platform-admin \
    --email super@dianrp.com --password "GANTI" --full-name "Platform Admin"
  ./bin/drpctl create-tenant \
    --slug demo --name "ISP Demo" --email admin@demo.local --password "GANTI" --full-name Admin
'
```

| URL | Fungsi |
|-----|--------|
| `https://billing.dianrp.com/login` | Superadmin |
| `https://billing.dianrp.com/<slug>/login` | Admin tenant |
| `https://billing.dianrp.com/<slug>/client/login` | Portal pelanggan |

### 8. Update berikutnya

```bash
sudo bash /www/wwwroot/billing.dianrp.com/deploy/scripts/update.sh
```

Override jika perlu: `APP_DIR=... WEB_ROOT=... BRANCH=main sudo -E bash .../update.sh`.

Rollback: `git checkout` commit sebelumnya lalu jalankan `update.sh` lagi.

### 9. Log

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
    proxy_pass http://127.0.0.1:8087;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 120s;
}

location /uploads/ {
    proxy_pass http://127.0.0.1:8087;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

location /events/ {
    proxy_pass http://127.0.0.1:8087;
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

Sesi WhatsApp produksi: `WHATSAPP_SESSION_DIR=./data/whatsapp` (folder `data/` di repo). Setelah Connect + scan QR, notifikasi invoice/pembayaran memakai sesi tersebut.

## Fitur

Pelanggan, paket, langganan, invoice, pembayaran manual/gateway, isolir otomatis, portal pelanggan, voucher, tiket, MAP FTTH, coverage, monitoring, notifikasi WA/Telegram/email, RADIUS opsional, import pelanggan MySQL legacy.
