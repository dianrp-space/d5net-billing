# Rencana Import Pelanggan (Billing Lama → d5net-billing)

Dokumen ini merangkum rencana migrasi data pelanggan dari aplikasi billing lama ke d5net-billing, berdasarkan file ekspor:

- **Sumber:** `/home/dianrp/Documents/Data_Pelanggan_1-2135_2026-09-13_214030.xlsx`
- **Jumlah baris:** 2135
- **Tipe layanan:** seluruhnya PPPoE
- **Tanggal analisis:** 13 Sep 2026

---

## 1. Ringkasan kondisi data lama

| Status | Jumlah | Keterangan |
|--------|--------|------------|
| Active | 1872 | Pelanggan aktif |
| Expired | 244 | Masa berlangganan habis |
| Berhenti Langganan | 19 | Sudah berhenti |
| **Total** | **2135** | |

### Kualitas data (temuan singkat)

| Cek | Hasil |
|-----|-------|
| `ID Pelanggan` kosong | 1 baris |
| `ID Pelanggan` duplikat | 1 nilai |
| `Nomor Telepon` kosong | 0 |
| `NIK` kosong / `0000000000000000` | ~39 |
| `Email` terisi | 1 saja |
| Username muncul >1 kali | ~674 nilai (perlu dicek sebelum sync Radius) |
| Punya `Nama ODP` | ~2089 baris / ~331 ODP unik |
| `Server` (calon cluster) | 9 unik |
| `Paket` (calon plan) | 59 unik |

---

## 2. Mapping kolom XLSX → d5net-billing

### 2.1 Identitas pelanggan (`customers`)

| Kolom lama | Kolom baru | Catatan |
|------------|------------|---------|
| `ID Pelanggan` | `customer_code` | Pakai kode lama agar upsert aman / bisa di-import ulang |
| `Nama` | `full_name` | Wajib |
| `Nomor Telepon` | `phone` | Wajib; default password portal = hash phone |
| `Email` | `email` | Opsional (hampir kosong di sumber) |
| `Alamat` | `address` | Opsional |
| `Koordinat` (`lat,lng`) | `latitude`, `longitude` | Pecah di koma |
| `NIK` | `identity_type=ktp` + `identity_number` | Skip jika kosong / `000…0` |
| `Server` | `cluster_code` → `cluster_id` | Cluster/site harus sudah ada |
| `Status` | `is_active`, `portal_enabled` | Active → true; Expired / Berhenti → false (atau cabut) |

**Tidak ikut CSV pelanggan:** paket, username/password PPPoE, ODP, IP, hutang.

### 2.2 Template CSV bawaan app

Import UI/API memakai kolom:

```text
customer_code, full_name, phone, email, address, cluster_code,
latitude, longitude, identity_type, identity_number, is_active, portal_enabled
```

- Endpoint: `POST /api/customers/import`
- UI: halaman **Pelanggan** → Import CSV
- Upsert by `customer_code` (unik per tenant)
- `customer_code` kosong → sistem generate otomatis

### 2.3 Relasi yang harus dibuat terpisah

| Data lama | Entitas baru | Cara |
|-----------|--------------|------|
| `Server` | Cluster (`sites`) | Seed manual / admin sebelum import |
| `Paket` + `Harga` | Plan (`plans`) | Seed ~59 plan; parse `Rp. 165.000` → `165000` |
| `Username` / `Password` + paket | Subscription | Pass ke-2 setelah pelanggan + plan ada |
| `Nama ODP` / Port / koordinat ODP | ODP + port | Pass ke-3 (opsional) |
| `Remote Address` / pool | IP assignment | Opsional, setelah pool siap |
| `Hutang` | Invoice / saldo | Opsional; keputusan bisnis dulu |

---

## 3. Usulan mapping Server → cluster_code

Prefix paket di sumber sudah mengisyaratkan wilayah. Usulan kode (boleh disesuaikan):

| Server (lama) | Jumlah | Usulan `cluster_code` | Catatan prefix paket |
|---------------|--------|------------------------|----------------------|
| Belilas | 740 | `BLS` | `BLS-*` |
| Sungai Lilin | 612 | `SLN` | `SLN-*` |
| Tanjung Anom | 273 | `TJA` | `TJA-*` |
| Sri Gunung | 258 | `SGN` | `SGN-*` |
| Permata Ratu | 126 | `PRT` | `PRT-*` |
| Kompos | 56 | `KPS` | `KPS-*` |
| Sepakat | 44 | `SPK` | `SPK-*` |
| Delima | 25 | `RDA` / `DLM` | banyak `RDA-*` |
| Tembilahan | 1 | `TBL` | `TBL-*` |

Pastikan kode cluster di admin **persis** sama (case-insensitive) dengan yang dipakai di CSV.

---

## 4. Tahapan migrasi (disarankan)

Jangan import semua sekaligus. Urutan aman:

### Tahap A — Persiapan master data

1. Buat **9 cluster** sesuai tabel di atas.
2. Buat **~59 plan** dari daftar unik `Paket` + harga.
3. Tandai plan `*-0` / harga Rp 0 sebagai internal/gratis jika perlu.
4. (Opsional) Siapkan ODP unik (~331) jika ingin migrasi port di tahap lanjut.

### Tahap B — Import pelanggan (identitas saja)

1. Generate CSV dari XLSX dengan mapping bagian 2.1.
2. Bersihkan:
   - 1 baris tanpa `ID Pelanggan`
   - 1 kode duplikat
   - NIK dummy
   - nilai `-` → kosong
3. Import via UI Pelanggan / `POST /api/customers/import`.
4. Verifikasi: jumlah baris, sample per cluster, status aktif/nonaktif.

### Tahap C — Subscription (PPPoE / paket)

1. Resolve `customer_code` → `customer_id`.
2. Resolve nama `Paket` → `plan_id`.
3. Buat subscription: username, password, status sesuai Active/Expired/Berhenti.
4. Cek dulu username duplikat (~674) agar tidak bentrok di Radius.

> CSV import pelanggan **tidak** membuat subscription. Ini pass terpisah (script/API `POST /api/subscriptions`).

### Tahap D — ODP / IP / hutang (opsional)

1. Import/seed ODP + port, tautkan ke subscription.
2. Assign IP/pool bila relevan.
3. Putuskan apakah `Hutang` digenerate jadi invoice terbuka atau diabaikan.

---

## 5. Fitur yang sudah ada di app ini

| Fitur | Lokasi / endpoint | Catatan |
|-------|-------------------|---------|
| Export CSV pelanggan | `GET /api/customers/export.csv` | Template + data existing |
| Import CSV pelanggan | `POST /api/customers/import` | Upsert by `customer_code` |
| UI Import/Export | `web/src/pages/CustomersPage.tsx` | Ikon Import / Export |
| Batch aktif/nonaktif | `POST /api/customers/batch-status` | Max 500 ID |
| CLI MySQL lama | `drpctl import-mysql-customers` | Hanya jika DB MySQL `tbl_customers` masih ada; lebih sempit |

**Belum ada:** import XLSX pelanggan langsung. Jalur praktis = XLSX → CSV UTF-8 → Import UI.

---

## 6. Checklist sebelum Go-live import

- [ ] Cluster 9 wilayah sudah dibuat & kode disepakati
- [ ] Plan ~59 sudah dibuat (nama/kode selaras dengan `Paket` lama atau ada tabel mapping)
- [ ] CSV pelanggan sudah di-generate & di-spot-check (10–20 baris)
- [ ] Keputusan status: Expired/Berhenti → nonaktif saja atau `dismantled`
- [ ] Keputusan password portal: tetap default phone, atau reset massal nanti
- [ ] Keputusan username duplikat: rename / skip / merge
- [ ] Backup DB d5net-billing sebelum import besar
- [ ] Uji import di tenant/staging dulu (mis. 50 baris), baru full 2135
- [ ] Setelah pelanggan OK → baru subscription → baru ODP/IP

---

## 7. Deliverable lanjutan (bila diminta)

1. **CSV pelanggan siap import** (mapping Server→cluster, pecah koordinat, bersihkan NIK/`-`)
2. **Daftar unik cluster + paket** (untuk seed admin)
3. **Script/CSV subscription** (setelah plan & cluster siap)
4. (Opsional) CLI/API baca XLSX langsung via `excelize` (dependency sudah ada di repo)

---

## 8. Risiko & mitigasi

| Risiko | Mitigasi |
|--------|----------|
| Cluster/plan belum ada → baris gagal / tanpa cluster | Seed master dulu; validasi `cluster_code` sebelum import |
| Username PPPoE duplikat | Audit & resolve sebelum sync Radius |
| Password portal = phone | Informasikan ke pelanggan / sediakan reset |
| Hutang & expired date tidak ikut CSV | Tahap terpisah + kebijakan bisnis |
| Import ulang tanpa `customer_code` stabil | Selalu isi `ID Pelanggan` lama sebagai `customer_code` |
| Data ODP/IP salah taut | Kerjakan setelah subscription stabil |

---

## 9. Keputusan yang perlu dikonfirmasi

1. Kode cluster final (terutama Delima: `RDA` vs `DLM`).
2. Nama/kode plan: pakai string `Paket` lama apa dibuatkan kode baru + tabel mapping.
3. Status Expired/Berhenti: nonaktif saja, atau tandai cabut (`dismantled_at`).
4. Apakah subscription & ODP ikut migrasi di gelombang pertama, atau hanya identitas dulu.
5. Apakah hutang dari app lama digenerate sebagai invoice di app baru.
