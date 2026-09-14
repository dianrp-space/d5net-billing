<p align="center">
  <img src="web/public/d5net.webp" alt="D5Net — better connecting all" width="360" />
</p>

# d5net-billing

Aplikasi billing ISP fullstack untuk jaringan MikroTik (PPPoE/Hotspot), single-provider
(tanpa multi-tenant / platform owner).

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

Vite listen di `0.0.0.0:5173`. PC lain di Wi‑Fi `192.168.100.0/24` bisa buka `http://<IP-laptop>:5173` (contoh `http://192.168.100.67:5173`). API default `0.0.0.0:8088`; frontend mem-proxy `/api`, `/uploads`, `/events`.

Pastikan firewall mengizinkan port **5173** (dan **8088** jika API dipanggil langsung):

```bash
sudo ufw allow from 192.168.100.0/24 to any port 5173 proto tcp
sudo ufw allow from 192.168.100.0/24 to any port 8088 proto tcp
```

Buat provider + admin pertama:

```bash
go run ./cmd/drpctl create-tenant --slug delimanet --name "Delima Net" --email admin@delimanet.id --password rahasia123 --full-name Admin
```

| Path | Fungsi |
|------|--------|
| `/` | Landing + info pembayaran tagihan |
| `/login` | Login pelanggan (portal) |
| `/client/dashboard` | Portal pelanggan |
| `/admin/login` | Login admin (akses manual via URL) |
| `/admin/dashboard` | Panel admin |
| `/isolir` | Portal isolir (login bayar tagihan) |

Go **tidak hot-reload**. Setelah ubah kode backend, restart proses API/worker (di produksi: `systemctl restart d5net-billing-api d5net-billing-worker`).

## Produksi (aaPanel + systemd)

Semua file aplikasi ada di **satu folder situs**, dijalankan user **`dianrp`** (bukan user sistem `drp` baru). Nginx root hanya `web/dist`, jadi `.env`, `data/`, `bin/`, dan `.git` tidak ikut ter-serve.

Yang tetap di luar webroot hanya unit systemd (`/etc/systemd/system/`) — systemd memang wajib baca unit dari situ — dan PostgreSQL/Nginx milik aaPanel.

`deploy/scripts/install.sh` adalah sisa alur tarball lama. **Jangan dipakai.** Pakai langkah di bawah + [`deploy/scripts/update.sh`](deploy/scripts/update.sh).

### Layout

| Path | Isi |
|------|-----|
| `/www/wwwroot/delimanet.dianrp.com` | Clone git (working dir proses) |
| `web/dist/` | Root situs aaPanel / Nginx (`index.html`) |
| `bin/` | `d5net-billing-api`, `d5net-billing-worker`, `drp-migrate`, `drpctl` |
| `.env` | Secret produksi (chmod 600, gitignored) |
| `data/` | Upload, backup router, backup DB |
| `/etc/systemd/system/d5net-billing-api.service` | Unit API — [`deploy/systemd/d5net-billing-api.service`](deploy/systemd/d5net-billing-api.service) |
| `/etc/systemd/system/d5net-billing-worker.service` | Unit worker — [`deploy/systemd/d5net-billing-worker.service`](deploy/systemd/d5net-billing-worker.service) |

Unit systemd memakai `ProtectSystem=strict` dan hanya boleh tulis ke `data/`. Log proses lewat `journalctl` (bukan `/var/log/d5net-billing`).

### Prasyarat

- aaPanel: Nginx, PostgreSQL 16+ / 18, SSL Let's Encrypt
- Di server: `git`, **Go 1.26+** (toolchain `go1.27.1`), **Node.js 20+** + `npm` (nvm OK), `rsync`, `curl`
- User **`dianrp`** (sudah ada; proses API/worker jalan sebagai user ini)

PostgreSQL 18 **tidak butuh** extension `pgcrypto` untuk UUID (`gen_random_uuid()` sudah di core sejak PG 13). Error `extension "pgcrypto" is not available` artinya paket contrib belum terpasang. Migrasi sudah mengabaikan pgcrypto jika tidak ada. `citext` (email case-insensitive) tetap lebih baik dari paket contrib:

```bash
# Debian/Ubuntu / aaPanel
sudo apt-get install -y postgresql-18-contrib
# lalu di database:
#   CREATE EXTENSION IF NOT EXISTS citext;
```

### 1. Database

Di aaPanel → PostgreSQL, buat database + user. Catat DSN:

```
postgres://USER:PASSWORD@127.0.0.1:5432/d5net_billing?sslmode=disable
```

### 2. Clone repo

```bash
sudo mkdir -p /www/wwwroot
sudo git clone git@github.com:dianrp-space/d5net-billing.git /www/wwwroot/delimanet.dianrp.com
# atau HTTPS:
# sudo git clone https://github.com/dianrp-space/d5net-billing.git /www/wwwroot/delimanet.dianrp.com
sudo chown -R dianrp:dianrp /www/wwwroot/delimanet.dianrp.com
```

User **`dianrp`** yang punya akses git (SSH key / credential). `update.sh` memakai `sudo` hanya untuk systemd; `git pull` dijalankan sebagai `dianrp`.

Kalau skrip lama masih `git pull` sebagai root, tarik dulu tanpa sudo:

```bash
cd /www/wwwroot/delimanet.dianrp.com
git pull
sudo bash deploy/scripts/update.sh
```

### 3. File env

```bash
sudo -u dianrp cp /www/wwwroot/delimanet.dianrp.com/.env.example /www/wwwroot/delimanet.dianrp.com/.env
sudo chmod 600 /www/wwwroot/delimanet.dianrp.com/.env
sudo nano /www/wwwroot/delimanet.dianrp.com/.env
```

Isi minimal (sesuaikan). Path data relatif ke folder repo:

```bash
APP_ENV=production
HTTP_ADDR=127.0.0.1:8088
ISOLIR_HTTP_ADDR=127.0.0.1:8090
DATABASE_URL=postgres://USER:PASSWORD@127.0.0.1:5432/d5net_billing?sslmode=disable&pool_max_conns=20
JWT_SECRET='<acak panjang, openssl rand -hex 32>'
ENCRYPTION_KEY='<tepat 32 karakter, openssl rand -base64 24 | cut -c1-32>'
CORS_ORIGINS=https://delimanet.dianrp.com
UPLOAD_DIR=./data/uploads
ROUTER_BACKUP_DIR=./data/router-backups
DB_BACKUP_DIR=./data/db-backups
WORKER_ENABLED=true
```

`ENCRYPTION_KEY` wajib **32 byte** (32 karakter). Ganti `JWT_SECRET` dari contoh. Setelah ubah env: `sudo systemctl restart d5net-billing-api d5net-billing-worker`.

`ISOLIR_HTTP_ADDR` bind localhost; nginx `listen 80 default_server` mem-proxy ke situ (lihat template nginx). Di Settings → Template Isolir, **Port DST-NAT publik** = `80`.

### 4. systemd

```bash
sudo cp /www/wwwroot/delimanet.dianrp.com/deploy/systemd/d5net-billing-api.service /etc/systemd/system/
sudo cp /www/wwwroot/delimanet.dianrp.com/deploy/systemd/d5net-billing-worker.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable d5net-billing-api d5net-billing-worker
```

Jangan `start` dulu sebelum binary ada (langkah 5). Kalau unit lama masih memakai `/opt` + `/etc/drp-billing`, timpa dengan file di atas lalu `daemon-reload`.

### 5. Build, migrate, start

```bash
sudo bash /www/wwwroot/delimanet.dianrp.com/deploy/scripts/update.sh
```

Skrip ini aman diulang: `git pull` sebagai `dianrp`, **skip `npm ci`** jika `package-lock.json` tidak berubah, skip Vite/Go build jika sumber tidak berubah, `migrate up`, restart systemd hanya jika binary/frontend berubah. Paksa: `FORCE_NPM=1 FORCE_WEB=1 FORCE_GO=1 FORCE_RESTART=1`.

Cek:

```bash
curl -sf http://127.0.0.1:8088/api/health
sudo systemctl status d5net-billing-api d5net-billing-worker --no-pager
```

### 6. Situs Nginx (aaPanel)

1. Buat website `delimanet.dianrp.com`, aktifkan SSL Let's Encrypt.
2. Set **website root** ke `/www/wwwroot/delimanet.dianrp.com/web/dist` (bukan folder git).
3. Gabungkan reverse proxy dari template [`deploy/nginx/d5net-billing.conf`](deploy/nginx/d5net-billing.conf) (lokasi `/api/`, `/uploads/`, `/events/`, `try_files` SPA, proxy ke **8088**). Reload Nginx.

| Lokasi | Peran |
|--------|--------|
| `/` | Static SPA; fallback `index.html` untuk `/admin/*`, `/client/*`, `/login`, `/isolir` |
| `/api/` | Proxy ke `127.0.0.1:8088` |
| `/uploads/` | Proxy file branding |
| `/events/` | SSE (buffering off) |

Jangan expose `.git`. Template sudah `deny` path itu.

### 7. Akun pertama

`drpctl` ada di `bin/` setelah `update.sh`. Load env lalu buat provider + admin:

```bash
sudo -u dianrp bash -c '
  set -a
  source /www/wwwroot/delimanet.dianrp.com/.env
  set +a
  cd /www/wwwroot/delimanet.dianrp.com
  ./bin/drpctl create-tenant \
    --slug delimanet --name "Delima Net" --email admin@delimanet.id --password "GANTI" --full-name Admin
'
```

| URL | Fungsi |
|-----|--------|
| `https://delimanet.dianrp.com/` | Landing + info pembayaran |
| `https://delimanet.dianrp.com/login` | Login pelanggan |
| `https://delimanet.dianrp.com/client/dashboard` | Portal pelanggan |
| `https://delimanet.dianrp.com/admin/login` | Login admin (akses manual via URL) |
| `https://delimanet.dianrp.com/admin/dashboard` | Panel admin |

### 8. Update berikutnya

```bash
sudo bash /www/wwwroot/delimanet.dianrp.com/deploy/scripts/update.sh
```

Override jika perlu: `APP_DIR=... WEB_ROOT=... BRANCH=main sudo -E bash .../update.sh`.

Rollback: `git checkout` commit sebelumnya lalu jalankan `update.sh` lagi.

### 9. Log

```bash
sudo journalctl -u d5net-billing-api -u d5net-billing-worker -f
```

Log Nginx: `/www/wwwlogs/delimanet.dianrp.com.*.log` (sesuai template).

### MikroTik

Enable API: `/ip service enable api` — port **8728** (TLS **8729**). Isi alamat, user, password di menu Router. Interval poller API (sesi/CPU) diatur di **Cronjob**, terpisah dari interval tagihan/isolir.

## Nginx: cuplikan

Template penuh: [`deploy/nginx/d5net-billing.conf`](deploy/nginx/d5net-billing.conf).

```nginx
root /www/wwwroot/delimanet.dianrp.com/web/dist;
index index.html;
client_max_body_size 20m;

location / {
    try_files $uri $uri/ /index.html;
}

location /api/ {
    proxy_pass http://127.0.0.1:8088;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 120s;
}

location /uploads/ {
    proxy_pass http://127.0.0.1:8088;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

location /events/ {
    proxy_pass http://127.0.0.1:8088;
    proxy_http_version 1.1;
    proxy_set_header Connection '';
    proxy_buffering off;
    proxy_cache off;
    chunked_transfer_encoding off;
}
```

## Integrasi (menu admin)

Di menu **Integrasi**:

- **Payment Gateway** — Duitku (VA, e-wallet, retail, QRIS) dan DOKU (halaman bayar + QRIS langsung via Direct API); pembayaran manual/tunai/transfer tetap tersedia

### DOKU (Checkout + QRIS Direct)

Di **Integrasi → Payment Gateway → DOKU** isi:

- **Client ID** & **Secret Key** — dari DOKU Back Office (wajib, untuk link bayar/checkout)
- **RSA private key (PEM)** — generate sendiri (`openssl genrsa -out private.key 2048`), upload public key-nya ke dashboard DOKU (wajib untuk QRIS langsung)
- **Merchant ID, Terminal ID, Kode pos** — ID QRIS dari DOKU setelah registrasi QRIS disetujui (wajib untuk QRIS langsung)
- **Sandbox** — aktifkan saat uji coba (`api-sandbox.doku.com`)

Alur: portal & bot `/link` memakai halaman bayar DOKU; bot `/qris` memakai QRIS Direct (gambar QR dikirim via WA). Daftarkan Notification URL ini di dashboard DOKU (per channel):

```
https://delimanet.dianrp.com/api/webhooks/payment/doku
```

Notifikasi QRIS diverifikasi dengan konfirmasi status aktif ke DOKU sebelum tagihan dilunasi.
- **Messaging Gateway** — tab **WhatsApp** (gateway eksternal GOWA: base URL + Basic Auth, multi-nomor) dan tab **Telegram** (bot token + chat ID ops)
- **Backup / Restore** — export/import JSON data aplikasi (dir `DB_BACKUP_DIR`)

WhatsApp tidak lagi memakai sesi in-process: arahkan tab WhatsApp di Integrasi ke gateway [go-whatsapp-web-multidevice](https://github.com/aldinokemal/go-whatsapp-web-multidevice) (login/scan QR di dashboard gateway), lalu isi base URL + Basic Auth.

**Multi-nomor (redundansi):** pada satu gateway/base URL yang sama, bisa didaftarkan lebih dari satu device (nomor). Tombol **Muat device dari gateway** mengisi daftar dari `GET /app/devices`; tiap device dipilih lewat header `X-Device-Id`. Saat mengirim, sistem mencoba nomor secara berurutan dan lanjut ke nomor berikutnya bila gagal (failover). **Cek koneksi** menampilkan status per nomor. Bila daftar dikosongkan, dipakai device default gateway.

### Bot WhatsApp pelanggan (`wabot`)

Di **Integrasi → Messaging Gateway → WhatsApp**, aktifkan **Bot WhatsApp pelanggan** dan pilih **nomor bot** (device ID). Bot hanya membalas bila pengirim adalah pelanggan aktif dan perintahnya cocok:

| Perintah | Aksi |
|----------|------|
| `/tagihan` | Kirim PDF tagihan berjalan sebagai dokumen |
| `/link` | Kirim link bayar online (Duitku) |
| `/qris` | Kirim gambar QR bayar langsung; fallback ke link bila QR tidak tersedia |

Perintah lain **tidak dibalas**. Bila tidak ada tagihan berjalan, bot membalas info ("sudah lunas" / "belum terbit" / "nomor belum terdaftar").

Agar pesan masuk diteruskan ke aplikasi, di dashboard gateway arahkan webhook nomor bot ke:

```
https://delimanet.dianrp.com/api/webhooks/whatsapp
```

dengan filter event `message` (payload native GOWA: `event`, `device_id`, `session_id`, `payload.from/body`). Balasan selalu dikirim dari nomor bot yang dipilih.

## Fitur

Pelanggan, paket, langganan, invoice, pembayaran manual/gateway, isolir otomatis, portal pelanggan, voucher, tiket, MAP FTTH, coverage, monitoring, notifikasi WA/Telegram/email, RADIUS opsional, import pelanggan MySQL legacy.
