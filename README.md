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
| `/admin/<slug>/login` | Login admin tenant |
| `/client/<slug>/login` | Portal pelanggan tenant |

Contoh: `/admin/demo/login`, `/client/demo/login`

## Produksi (aaPanel)

1. Buat database PostgreSQL di aaPanel.
2. Salin `.env.example` ke `/etc/drp-billing/drp-billing.env` (chmod 600).
3. Build: `make release VERSION=1.0.0`
4. Jalankan `deploy/scripts/install.sh 1.0.0`
5. Tambah situs Nginx (lihat di bawah), aktifkan SSL Let's Encrypt.
6. `systemctl enable --now drp-api drp-worker`

RouterOS: enable API (`/ip service enable api`), port default **8728** (TLS **8729**). Alamat bisa IP atau domain.

## Nginx: static frontend + reverse proxy

Template lengkap: [`deploy/nginx/drp-billing.conf`](deploy/nginx/drp-billing.conf).

| Lokasi | Peran |
|--------|--------|
| `/` | Static SPA (`web/dist` → `root`); `try_files` fallback ke `index.html` untuk `/admin/...` dan `/client/...` |
| `/api/` | Reverse proxy ke API Go (`127.0.0.1:8080`) |
| `/uploads/` | Reverse proxy file branding (logo/favicon) ke API |
| `/events/` | Reverse proxy SSE monitoring (buffering off) |

Contoh inti (HTTPS):

```nginx
root /www/wwwroot/drp-billing/web;
index index.html;
client_max_body_size 20m;

# Frontend SPA
location / {
    try_files $uri $uri/ /index.html;
}

# API
location /api/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 120s;
}

# Upload branding (logo/favicon)
location /uploads/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

# SSE realtime
location /events/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Connection '';
    proxy_buffering off;
    proxy_cache off;
    chunked_transfer_encoding off;
}
```

Catatan aaPanel:

- Arahkan `root` ke folder hasil build frontend (isi `index.html` + assets), biasanya salinan dari `web/dist`.
- Pastikan API listen di `HTTP_ADDR` (default `127.0.0.1:8080`) dan `UPLOAD_DIR` writable oleh user service.
- Redirect HTTP→HTTPS dan path sertifikat Let's Encrypt ikut template di `deploy/nginx/drp-billing.conf`.

## Integrasi (admin tenant)

Di menu **Integrasi**:

- **Payment Gateway** — DRP Payment (QRIS), kredensial per tenant atau env `DRP_PAYMENT_*`
- **Messaging Gateway** — tab **WhatsApp** (pairing QR via [whatsmeow](https://github.com/tulir/whatsmeow), ke pelanggan) dan tab **Telegram** (bot token + chat ID **ops tenant** saja)
- **Backup / Restore** — tenant: export/import JSON data tenant; platform/owner: `pg_dump` / `psql` penuh (dir `DB_BACKUP_DIR`)

Sesi WhatsApp disimpan di `WHATSAPP_SESSION_DIR` (default `./data/whatsapp`). Setelah Connect + scan QR, notifikasi invoice/pembayaran memakai sesi tersebut.

## Fitur

Pelanggan, paket, langganan, invoice, pembayaran manual/gateway, isolir otomatis, portal pelanggan, voucher, tiket, work order, ODP/FTTH, monitoring, notifikasi WA/Telegram/email, RADIUS opsional, import pelanggan MySQL legacy.
