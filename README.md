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
5. Tambah situs Nginx, tempel `deploy/nginx/drp-billing.conf`, aktifkan SSL Let's Encrypt.
6. `systemctl enable --now drp-api drp-worker`

RouterOS: enable API (`/ip service enable api`), port default **8728** (TLS **8729**). Alamat bisa IP atau domain.

## Fitur

Pelanggan, paket, langganan, invoice, pembayaran manual/gateway, isolir otomatis, portal pelanggan, voucher, tiket, work order, ODP/FTTH, monitoring, notifikasi WA/Telegram/email, RADIUS opsional, import pelanggan MySQL legacy.
