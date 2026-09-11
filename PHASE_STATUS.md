# Status fase (update 2026-09-04)

Semua fase **sudah diisi implementasi yang bisa dijalankan** (bukan lagi kerangka kosong). Beberapa item plan asli tetap opsional/produksi (River, Timescale, Playwright penuh, maroto).

| Fase | Status | Yang sekarang ada di kode |
|------|--------|---------------------------|
| 1 Fondasi | **siap pakai** | goose, chi+huma, JWT/argon2id/TOTP, RLS+SET LOCAL, OpenAPI live, auth gate `/api/*`, sqlcgen ping, worker ticker + `job_runs` |
| UI shell | **siap pakai** | Tailwind v4 + **shadcn/ui (Radix)** (`button`/`input`/`dialog`/`alert-dialog`/`select`/`table`/…), `AppDialogProvider` (confirm/alert ganti `window.confirm`), tema NeedMCP `natural-tone`, wrapper `web/src/ui.tsx` |
| 2 RouterOS | **siap pakai** | CRUD+AES+test, PPP/hotspot/DHCP/queue, circuit+dial cache, audit, **IP Pool** API+UI (CRUD + assignment, relasi router) |
| 3 Billing | **siap pakai** | prorata, denda, resume saat bayar, isolir+address-list, PDF, portal tagihan+riwayat, usage API |
| 4 Payment/notif | **siap pakai** | Manual + Duitku (HMAC webhook), webhook bayar→resume+journal, template DB, dunning H-7..H+3 idempotent |
| 5 Monitoring | **siap pakai** | poller resource CPU/mem, session, alert ODP, SSE `?tenant_id=`, panel alert dashboard |
| 6 Operasional | **siap pakai** | voucher+QR, tiket, WO+check-in, ODP+peta OSM, lead pipeline, portal teknisi |
| 7 Akunting | **siap pakai** | COA seed, journal on pay, expense, P&L, cashflow, churn, CSV/XLSX, reseller, email laporan tgl 1 jam 8 |
| 8 Lanjutan | **siap pakai** | RADIUS radcheck+Disconnect UDP CoA, backup, reconcile dry-run, bulk-push preview, importer MySQL, outbound webhook |
| Testing/CI | **siap pakai** | `go test -race`, vitest, build amd64+arm64 api+worker, checksum |

## URL single-provider
- `/` landing · `/login` login pelanggan → `/client/dashboard` · `/admin/login` login admin → `/admin/dashboard` · `/isolir` portal isolir
- Tanpa slug, tanpa platform owner; provider tunggal di-resolve otomatis dari DB

## UUID primary keys (2026-09-04)
- Semua PK/FK aplikasi memakai **UUID** (migrasi `005_uuid_primary_keys.sql`)
- Tabel FreeRADIUS (`radcheck`/`radreply`/`radacct`) tetap BIGSERIAL
- Setelah pull: `make migrate-up` lalu login ulang (JWT lama invalid)

## Cluster / POP (2026-09-04)
- CRUD `/api/clusters` (tabel `sites`) — banyak POP per tenant
- Relasi: `routers.site_id` / `customers.cluster_id` → cluster
- Kode pelanggan: pola default `{prefix}-{yyyymm}{seq}` (mis. `D5N-2026090001`), prefix & pola bisa dikustom per cluster; override manual saat create
- Migrasi `006_clusters.sql`

## Plan offers per cluster (2026-09-04)
- Tabel `plan_cluster_offers` (migrasi `007`) — harga/ketersediaan paket per cluster
- API `/api/plan-offers` + sync profile PPP/hotspot ke semua router di cluster
- Tagihan memakai harga offer (fallback harga dasar paket)
- Form langganan filter paket & router sesuai cluster pelanggan

## FTTH map + customer CRUD (2026-09-04)
- ODP terhubung ke cluster (`cluster_id`, migrasi `009`); halaman ODP/FTTH pakai tab per cluster + peta center ke lat/long POP
- Modal: tidak tutup saat klik luar; header bisa digeser
- Relasi langganan ↔ port ODP (`odp_ports`, migrasi `010`): cek sisa slot, assign saat buat langganan

## Setelah pull
```bash
make migrate-up   # termasuk 005..010
make dev
```
