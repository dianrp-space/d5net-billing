<p align="center">
  <img src="web/public/d5net.webp" alt="D5Net — better connecting all" width="360" />
</p>

# d5net-billing

Aplikasi billing ISP fullstack untuk jaringan MikroTik (PPPoE/Hotspot), dirancang untuk
**satu provider/ISP** (single-provider, bukan marketplace multi-ISP / platform owner).
Arsitektur data multi-tenant (tabel `tenant`) agar tiap provider punya admin,
pelanggan, router, dan pengaturan sendiri.

- **Backend:** Go (chi + huma + pgx) — API HTTP + worker terpisah
- **Frontend:** React 19 + Vite + Tailwind + ECharts
- **Database:** PostgreSQL (migrasi goose, query di-generate sqlc)
- **Deployment:** Linux + aaPanel + Nginx + systemd (tanpa Docker Compose)

---

## Tech Stack

### Backend (Go)

| Kategori | Pustaka | Fungsi |
|----------|---------|--------|
| Router & API | `go-chi/chi/v5`, `danielgtaylor/huma/v2` | HTTP routing + OpenAPI/Huma schema handler |
| Database | `jackc/pgx/v5`, `pressly/goose/v3`, `sqlc` | Pool PostgreSQL, migrasi SQL, query ter-generasi ke `internal/sqlcgen` |
| Auth | `golang-jwt/jwt/v5`, `pquerna/otp` | JWT access/refresh, TOTP (2FA) |
| MikroTik | `go-routeros/routeros/v3` | API RouterOS (provisioning PPPoE/Hotspot, isolir, IP pool) |
| Legacy import | `go-sql-driver/mysql` | Impor pelanggan dari MySQL legacy |
| Utilitas | `google/uuid`, `golang.org/x/crypto`, `skip2/go-qrcode`, `xuri/excelize/v2`, `HugoSmits86/nativewebp`, `golang.org/x/image` | UUID, enkripsi, QR code, Excel import/export, pemrosesan gambar WebP (branding) |
| Config | `caarlos0/env/v11` + `.env` loader | Pembacaan variabel lingkungan |

Worker (`d5net-billing-worker`) berjalan sebagai proses terpisah: menjalankan siklus
billing, isolir, dunning, pemrosesan antrean notifikasi, rekonsiliasi router, dan laporan
bulanan.

### Frontend (TypeScript/React)

| Kategori | Pustaka |
|----------|---------|
| Framework | React 19, Vite 6 |
| Routing | `@tanstack/react-router` (type-safe) |
| Data/state | `@tanstack/react-query` v5 |
| Styling | Tailwind CSS v4 (`@tailwindcss/vite`), Radix UI primitives, lucide-react |
| Charts | `echarts` + `echarts-for-react` |
| Forms | `react-hook-form` + `zod` |
| UX | `sweetalert2`, `@dnd-kit/*` (drag-and-drop: leads/kanban) |
| Test | `vitest` + `jsdom` (unit), Playwright (e2e) |

---

## Project Layout

| Path | Isi |
|------|-----|
| `cmd/api` | Entrypoint API HTTP (`d5net-billing-api`) |
| `cmd/worker` | Entrypoint worker (`d5net-billing-worker`) |
| `cmd/migrate` | CLI migrasi (`drp-migrate`) |
| `cmd/drpctl` | CLI admin (`drpctl`): `create-tenant`, dsb. |
| `internal/` | Domain: `handlers`, `store`, `billing`, `payment`, `notify`, `job`, `provision`, `monitor`, `auth`, `config`, `sqlcgen`, dll. |
| `migrations/` | 46 migrasi SQL (goose) |
| `queries/` | Query SQL sumber untuk sqlc |
| `web/` | Frontend React (Vite) |
| `deploy/` | Template systemd, Nginx, skrip `update.sh`/`rollback.sh`/`upgrade.sh` |
| `docs/` | Catatan teknis (`routeros.md`), audit & checklist keamanan (`SECURITY-AUDIT.md`) |

---

## Prasyarat (global)

- **Go 1.26+** (toolchain `go1.27.1` otomatis ditarik)
- **Node.js 20+** + `npm`
- **PostgreSQL 16+ / 18**, `git`, `rsync`, `curl`
- (Produksi) aaPanel dengan Nginx + PostgreSQL milik aaPanel, SSL Let's Encrypt

PostgreSQL 18 tidak butuh extension `pgcrypto` untuk UUID (`gen_random_uuid()` sudah di
core sejak PG 13). `citext` (email case-insensitive) tetap disarankan dari paket contrib:

```bash
sudo apt-get install -y postgresql-18-contrib
# di database: CREATE EXTENSION IF NOT EXISTS citext;
```

---

## Pengembangan

```bash
cp .env.example .env
# set DATABASE_URL, JWT_SECRET, ENCRYPTION_KEY (tepat 32 karakter)
make migrate-up
make run-api
# terminal lain:
cd web && npm install && npm run dev
```

Vite listen di `0.0.0.0:5173`. PC lain di Wi‑Fi `192.168.100.0/24` bisa buka
`http://<IP-laptop>:5173` (contoh `http://192.168.100.67:5173`). API default
`0.0.0.0:8088`; frontend mem-proxy `/api`, `/uploads`, `/events`.

Pastikan firewall mengizinkan port **5173** (dan **8088** jika API dipanggil langsung):

```bash
sudo ufw allow from 192.168.100.0/24 to any port 5173 proto tcp
sudo ufw allow from 192.168.100.0/24 to any port 8088 proto tcp
```

Skrip cepat `make dev` menjalankan `scripts/dev.sh` (API + Web bersama).

Buat provider + admin pertama (satu per tenant):

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

Go **tidak hot-reload**. Setelah ubah kode backend, restart proses API/worker (di produksi:
`systemctl restart d5net-billing-api d5net-billing-worker`).

---

## Produksi (aaPanel + systemd)

Semua file aplikasi ada di **satu folder situs**, dijalankan user **`dianrp`** (bukan user
sistem `drp` baru). Nginx root hanya `web/dist`, jadi `.env`, `data/`, `bin/`, dan `.git`
tidak ikut ter-serve.

Yang tetap di luar webroot hanya unit systemd (`/etc/systemd/system/`) — systemd memang
wajib baca unit dari situ — dan PostgreSQL/Nginx milik aaPanel.

`deploy/scripts/install.sh` adalah sisa alur tarball lama. **Jangan dipakai.** Pakai
langkah di bawah + [`deploy/scripts/update.sh`](deploy/scripts/update.sh).

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

Unit systemd memakai `ProtectSystem=strict` dan hanya boleh tulis ke `data/`. Log proses
lewat `journalctl` (bukan `/var/log/d5net-billing`).

### Prasyarat

- aaPanel: Nginx, PostgreSQL 16+ / 18, SSL Let's Encrypt
- Di server: `git`, **Go 1.26+** (toolchain `go1.27.1`), **Node.js 20+** + `npm` (nvm OK), `rsync`, `curl`
- User **`dianrp`** (sudah ada; proses API/worker jalan sebagai user ini)

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

User **`dianrp`** yang punya akses git (SSH key / credential). `update.sh` memakai `sudo`
hanya untuk systemd; `git pull` dijalankan sebagai `dianrp`.

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

Isi minimal (lihat juga **Variabel Lingkungan** di bawah). Path data relatif ke folder repo:

```bash
APP_ENV=production
HTTP_ADDR=127.0.0.1:8088
DATABASE_URL=postgres://USER:PASSWORD@127.0.0.1:5432/d5net_billing?sslmode=disable&pool_max_conns=20
JWT_SECRET='<acak panjang, openssl rand -hex 32>'
ENCRYPTION_KEY='<tepat 32 karakter, openssl rand -base64 24 | cut -c1-32>'
CORS_ORIGINS=https://delimanet.dianrp.com
UPLOAD_DIR=./data/uploads
ROUTER_BACKUP_DIR=./data/router-backups
DB_BACKUP_DIR=./data/db-backups
WORKER_ENABLED=true
```

`ENCRYPTION_KEY` wajib **32 byte** (32 karakter). Ganti `JWT_SECRET` dari contoh.
Setelah ubah env: `sudo systemctl restart d5net-billing-api d5net-billing-worker`.

### 4. systemd

```bash
sudo cp /www/wwwroot/delimanet.dianrp.com/deploy/systemd/d5net-billing-api.service /etc/systemd/system/
sudo cp /www/wwwroot/delimanet.dianrp.com/deploy/systemd/d5net-billing-worker.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable d5net-billing-api d5net-billing-worker
```

Jangan `start` dulu sebelum binary ada (langkah 5). Kalau unit lama masih memakai `/opt` +
`/etc/drp-billing`, timpa dengan file di atas lalu `daemon-reload`.

### 5. Build, migrate, start

```bash
sudo bash /www/wwwroot/delimanet.dianrp.com/deploy/scripts/update.sh
```

Skrip ini aman diulang: `git pull` sebagai `dianrp`, **skip `npm ci`** jika
`package-lock.json` tidak berubah, skip Vite/Go build jika sumber tidak berubah, `migrate
up`, restart systemd hanya jika binary/frontend berubah. Paksa:
`FORCE_NPM=1 FORCE_WEB=1 FORCE_GO=1 FORCE_RESTART=1`.

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

---

## Variabel Lingkungan

| Variabel | Default | Keterangan |
|----------|---------|------------|
| `APP_ENV` | `development` | `development` / `production` |
| `HTTP_ADDR` | `0.0.0.0:8088` | Bind address API |
| `DATABASE_URL` | — (wajib) | DSN PostgreSQL (`pool_max_conns` didukung) |
| `JWT_SECRET` | — (wajib) | Kunci tanda tangan JWT (acak panjang) |
| `JWT_ACCESS_TTL` | `15m` (prod `8h` di example) | Masa berlaku token akses |
| `JWT_REFRESH_TTL` | `720h` | Masa berlaku refresh token |
| `ENCRYPTION_KEY` | — (wajib) | **Tepat 32 karakter** untuk enkripsi secret (router, gateway) |
| `CORS_ORIGINS` | `http://localhost:5173` | Daftar origin dipisah koma |
| `UPLOAD_DIR` | `./data/uploads` | Direktori upload branding |
| `ROUTER_BACKUP_DIR` | `./data/router-backups` | Backup konfig router |
| `DB_BACKUP_DIR` | `./data/db-backups` | Backup/restore JSON + dump DB |
| `WORKER_ENABLED` | `true` | Jalankan siklus worker (billing/isolir/notif) |
| `TELEGRAM_BOT_TOKEN` | — | Fallback bot Telegram global (bisa di-override di Integrasi) |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USER` / `SMTP_PASS` / `SMTP_FROM` | — | Fallback SMTP global (bisa di-override di Integrasi → Email) |

---

## API & OpenAPI

API dibangun dengan **Huma**; spesifikasi OpenAPI live tersedia di:

```
GET /api/openapi.json
```

Endpoint publik (tanpa auth tenant): `/api/health`, `/api/auth/*`, `/api/public/*`,
`/api/portal/*`, `/api/webhooks/*`, `/uploads/*`, `/events/stream`, `/docs`,
`/api/openapi.json`. Sisa `/api/*` membutuhkan token tenant (middleware `tenant`).

---

## Integrasi (menu admin → Integrasi)

### Payment Gateway (PG)

Di menu **Integrasi → Payment Gateway** tersedia:

- **Manual** — pembayaran tunai / transfer bank dikonfirmasi admin (tanpa webhook).
- **Duitku** (POP API) — VA (bank), e-wallet, retail (alfamart/indomaret), QRIS. Mendukung
  kredensial terpisah **sandbox & produksi** yang disimpan bersamaan, mode biaya admin
  (`fee_mode` = `customer` menambahkan MDR ke tagihan, atau `merchant`), serta
  `fee_flat` + `fee_percent`. Webhook HMAC diverifikasi di
  `POST /api/webhooks/payment/duitku`.
- **DOKU** — dua mode:
  - **Checkout / halaman bayar** (Client ID + Secret Key) untuk link bayar.
  - **QRIS Direct** (Direct API) memakai RSA private key (PEM) + Merchant ID, Terminal ID,
    Kode pos; menghasilkan gambar QR yang dikirim via WhatsApp. Notifikasi QRIS
    diverifikasi dengan konfirmasi status aktif ke DOKU sebelum tagihan dilunasi.
  - Per-channel fee dapat diatur (`channels` map). Webhook di
    `POST /api/webhooks/payment/doku`.

Daftarkan Notification URL berikut di dashboard masing-masing PSP:

```
https://delimanet.dianrp.com/api/webhooks/payment/duitku
https://delimanet.dianrp.com/api/webhooks/payment/doku
```

Pembayaran otomatis juga mendukung **wallet/top-up pelanggan**: saat tagihan terbit, saldo
pelanggan dicoba auto-bayar; jika kurang, dikirim notifikasi saldo kurang.

#### DOKU (Checkout + QRIS Direct)

Di **Integrasi → Payment Gateway → DOKU** isi:

- **Client ID** & **Secret Key** — dari DOKU Back Office (wajib, untuk link bayar/checkout)
- **RSA private key (PEM)** — generate sendiri (`openssl genrsa -out private.key 2048`), upload public key-nya ke dashboard DOKU (wajib untuk QRIS langsung)
- **Merchant ID, Terminal ID, Kode pos** — ID QRIS dari DOKU setelah registrasi QRIS disetujui (wajib untuk QRIS langsung)
- **Sandbox** — aktifkan saat uji coba (`api-sandbox.doku.com`)

Alur: portal & bot `/link` memakai halaman bayar DOKU; bot `/qris` memakai QRIS Direct
(gambar QR dikirim via WA). Daftarkan Notification URL ini di dashboard DOKU (per channel):

```
https://delimanet.dianrp.com/api/webhooks/payment/doku
```

Notifikasi QRIS diverifikasi dengan konfirmasi status aktif ke DOKU sebelum tagihan dilunasi.

### Messaging Gateway (MSG)

Di menu **Integrasi → Messaging Gateway**:

- **WhatsApp** — gateway eksternal [go-whatsapp-web-multidevice (GOWA)](https://github.com/aldinokemal/go-whatsapp-web-multidevice): isi **base URL** + **Basic Auth**. WhatsApp tidak lagi memakai sesi in-process; login/scan QR dilakukan di dashboard gateway.
  - **Multi-nomor (redundansi):** pada satu gateway/base URL yang sama, bisa didaftarkan lebih dari satu device (nomor). Tombol **Muat device dari gateway** mengisi daftar dari `GET /app/devices`; tiap device dipilih lewat header `X-Device-Id`. Saat mengirim, sistem mencoba nomor secara berurutan dan lanjut ke nomor berikutnya bila gagal (**failover**). **Cek koneksi** menampilkan status per nomor. Bila daftar dikosongkan, dipakai device default gateway.
- **Telegram** — bot token + chat ID (opsional, ada juga fallback env `TELEGRAM_BOT_TOKEN`).
- **Email (SMTP)** — host, port, user, password, from, from name. Ada juga fallback env `SMTP_*`.

WhatsApp diarahkan ke gateway GOWA (login/scan QR di dashboard gateway), lalu isi base URL +
Basic Auth.

#### Bot WhatsApp pelanggan (`wabot`)

Di **Integrasi → Messaging Gateway → WhatsApp**, aktifkan **Bot WhatsApp pelanggan** dan
pilih **nomor bot** (device ID). Bot hanya membalas bila pengirim adalah pelanggan aktif dan
perintahnya cocok:

| Perintah | Aksi |
|----------|------|
| `/tagihan` | Kirim PDF tagihan berjalan sebagai dokumen |
| `/link` | Kirim link bayar online (Duitku) |
| `/qris` | Kirim gambar QR bayar langsung; fallback ke link bila QR tidak tersedia |

Perintah lain **tidak dibalas**. Bila tidak ada tagihan berjalan, bot membalas info
("sudah lunas" / "belum terbit" / "nomor belum terdaftar").

Agar pesan masuk diteruskan ke aplikasi, di dashboard gateway arahkan webhook nomor bot ke:

```
https://delimanet.dianrp.com/api/webhooks/whatsapp
```

dengan filter event `message` (payload native GOWA: `event`, `device_id`, `session_id`,
`payload.from/body`). Balasan selalu dikirim dari nomor bot yang dipilih.

### Backup / Restore

Menu **Integrasi → Backup / Restore** (atau `DB_BACKUP_DIR`): export/import JSON data
aplikasi + dump PostgreSQL.

---

## Notifikasi (Notif)

Layanan `notify` (`internal/notify`) mengirim pesan lewat tiga channel — **WhatsApp**,
**Telegram**, **Email** — dengan antrean persisten di tabel `notification_queue`
(`status` = `pending`/`sent`/`failed`, `scheduled_at`, `attempts`, `batch_id`). Worker
memproses antrean tiap siklus (`ProcessPending`, batch = `NotifyBatchSize`, default 100).

### Template & katalog event

Template tersimpan di `notification_templates` (channel + event + `is_active`). Bila tidak
ada template aktif, dipakai default bawaan. Variabel `{{customer_name}}`, `{{plan_name}}`,
`{{item_name}}`, `{{invoice_number}}`, `{{amount}}`, `{{due_date}}`, `{{phone}}`,
`{{balance}}` diisi otomatis.

| Event | Label | Channel default | Pemicu |
|-------|-------|----------------|--------|
| `invoice_issued` | Tagihan baru (manual) | whatsapp | Admin menerbitkan tagihan manual |
| `invoice_reminder` | Pengingat tagihan (dunning) | whatsapp | Offset hari di menu Cronjob |
| `payment_confirmation` | Konfirmasi pembayaran | whatsapp | Pembayaran diterima (incl. auto-pay wallet) |
| `wallet_topup` | Topup saldo berhasil | whatsapp | Saldo pelanggan bertambah |
| `wallet_insufficient` | Saldo kurang saat tagihan terbit | whatsapp | Tagihan terbit tapi saldo tidak cukup |
| `broadcast` | Broadcast manual | whatsapp, telegram, email | Tab Broadcast (pesan massal) |

### Broadcast

Tab **Broadcast** mengirim pesan massal ke banyak penerima dengan **jeda terstaggered**
(`delaySeconds` 1–60 detik) untuk rate-limit gateway. Tiap broadcast punya `batch_id` yang
dapat dipantau progresnya (total / pending / sent / failed) secara live.

### Dunning (pengingat berjenjang)

Worker mengirim `invoice_reminder` berdasarkan **offset hari** (`DunningOffsets`, mis.
`-3, 0, 3, 7`) yang diatur di menu **Cronjob/Jobs**. Setiap kombinasi invoice+offset hanya
dikirim sekali (`ClaimJob`).

### Laporan bulanan & retensi

- **Laporan bisnis bulanan** dikirim via email ke email tenant pada hari/jam yang diatur
  (`MonthlyReport*` di Jobs), berisi pelanggan aktif, langganan aktif, tagihan belum lunas,
  dan pendapatan bulan berjalan.
- **Retensi log notifikasi**: baris final (`sent`/`failed`) yang lebih tua dari
  `NotifLogRetentionDays` dihapus otomatis (sekali sehari). Antrean `pending` tidak pernah
  dihapus.

### Run manual (Jalankan sekarang)

Tombol **Jalankan sekarang** di menu Cronjob menjalankan siklus worker segera tanpa menunggu
interval, dengan cakupan yang bisa dipilih:

- **Sertakan sampling router** — poll semua router aktif (sesi, CPU/memori, traffic);
  interval rutin tidak diganggu (tidak dobel).
- **Paksa reconcile + laporan** — jalankan reconcile mingguan / laporan bulanan walau di luar
  jadwal hari/jam; `ClaimJob` tetap mencegah eksekusi ganda (maks. 1x sehari / 1x sebulan).

### Ops / alert

Kejadian operasional (isolir, drift rekonsiliasi router) dikirim ke **Telegram ops** tenant
dan dicatat sebagai alert (`alerts`) yang bisa dilihat di lonceng notifikasi admin.

---

## Fitur (ringkasan)

- **Pelanggan & penagihan:** pelanggan, paket/plan, langganan (subscription), invoice,
  diskon paket, pembayaran manual & gateway.
- **Invoice per tenant:** teks perusahaan, cara bayar, catatan kaki, serta **warna aksen**
  dan **warna stempel LUNAS** yang bisa diatur dari Format Invoice (PDF + pratinjau ikut).
- **Wallet:** saldo pelanggan, top-up, auto-bayar tagihan dari saldo.
- **Isolir otomatis:** suspend/resume otomatis di MikroTik saat jatuh tempo / lunas; profil
  isolir + IP pool + portal redirect.
- **Provisioning:** MikroTik RouterOS (PPPoE/Hotspot) via API + **RADIUS CoA** opsional;
  IPAM (pools & alokasi IP).
- **Portal:** landing + info bayar, login pelanggan, dashboard, portal isolir.
- **Payment Gateway:** manual, Duitku, DOKU (checkout + QRIS direct).
- **Messaging:** WhatsApp (GOWA multi-nomor failover), Telegram, Email SMTP; bot WA
  pelanggan (`/tagihan`, `/link`, `/qris`).
- **Notifikasi:** template per event, broadcast massal, dunning, laporan bulanan, retensi,
  run manual dengan cakupan (sampling router, paksa terjadwal).
- **Monitoring:** poller sesi/CPU router + **SSE** live (`/events/stream`); live traffic
  pelanggan (rx = download, grafik 3 menit); alert & lonceng.
- **Peta & coverage:** FTTH map, ODP, rute kabel, coverage area (marker POP magenta).
- **Operasional:** tiket, voucher, leads (kanban), SLA report, audit log.
- **Keamanan:** rate-limit login, logout + revoke sesi (termasuk deteksi pakai-ulang refresh
  token), tolak kunci default saat start, verifikasi signature webhook, tolak upload SVG,
  security headers — lihat [`docs/SECURITY-AUDIT.md`](docs/SECURITY-AUDIT.md).
- **Lainnya:** impor pelanggan MySQL legacy, backup/restore JSON + DB, branding upload,
  multi-tema (dark/light), widget Chatwoot.

---

## MikroTik / RouterOS

Enable API: `/ip service enable api` — port **8728** (TLS **8729**). Isi alamat, user,
password di menu Router. Interval poller API (sesi/CPU) diatur di **Cronjob**, terpisah dari
interval tagihan/isolir. Lihat [`docs/routeros.md`](docs/routeros.md) untuk setup user
terbatas dan URL portal isolir.

---

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

---

## Testing

```bash
# Backend (race detector)
make test
# atau: go test ./... -race -count=1

# Frontend unit
cd web && npm test          # vitest run --pool=forks

# Frontend e2e
cd web && npm run e2e       # playwright test
```

---

## Lisensi

Lihat [LICENSE](LICENSE).
