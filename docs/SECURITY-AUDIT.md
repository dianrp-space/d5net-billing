# Laporan Audit Keamanan — d5net-billing

Tanggal audit awal: 2026-09-22. Dokumen ini adalah checklist hidup:
setiap item yang selesai diimplementasi dicentang `[x]` beserta tanggal +
referensi file. Kerjakan berurutan dari P0 ke P2.

## P0 — Wajib sebelum production

- [x] **P0-1 — Rate-limit auth + batas panjang password** (brute-force & CPU-DoS Argon2). ✅ 2026-09-22.
  Implementasi: `internal/httpx/ratelimit.go` (middleware, login 10/menit/IP,
  refresh 30/menit/IP, 429 + `Retry-After`), dipasang di `internal/httpx/server.go`;
  `MaxPasswordLength=128` di `internal/auth/password.go` (ditolak sebelum Argon2);
  test `internal/httpx/ratelimit_test.go`. `go build ./...` +
  `go test ./internal/httpx/ ./internal/auth/` hijau.
- [x] **P0-2 — Logout + revoke sesi** (token curian tetap hidup). ✅ 2026-09-22.
  Implementasi: `POST /api/auth/logout` idempotent (revoke refresh + clear cookie) di
  `internal/handlers/handlers.go`; `RevokeUserRefreshTokens` di `internal/store/users.go`;
  deteksi reuse refresh token (revoke semua sesi + audit `auth.refresh_reuse`);
  ganti password sendiri / reset admin / nonaktif / hapus user → cabut semua sesi
  (`handlers.go`, `settings.go`); logout frontend panggil endpoint (`UserMenu.tsx`);
  label audit baru di `AuditLogPage.tsx`. `go build`, `go vet`, `go test` hijau.
- [x] **P0-3 — Kunci default wajib ditolak saat start**. ✅ 2026-09-22.
  Implementasi: `validateSecrets()` di `internal/config/config.go` (tolak start bila
  `JWT_SECRET` < 32 char / nilai contoh, `ENCRYPTION_KEY` != 32 char / nilai contoh);
  test `internal/config/config_test.go`; secret `.env` lokal dirotasi ke nilai acak.
  Catatan: semua sesi lama (JWT) invalid setelah ganti `JWT_SECRET` — user harus login ulang.
- [x] **P0-4 — Hapus webhook signature soft-fail**. ✅ 2026-09-22.
  Implementasi: `internal/handlers/handlers.go` — signature invalid selalu 401 di semua
  env (cabang `softFail` development dihapus).
- [x] **P0-5 — Amankan `/uploads/*` + larang SVG aktif**. ✅ 2026-09-22 (Stored XSS).
  Implementasi: upload SVG ditolak via ekstensi maupun isi (`internal/upload/image.go`,
  test `image_test.go`); `/uploads/*` diserve dengan `X-Content-Type-Options: nosniff`
  + `Content-Security-Policy: sandbox` (`settings.go:MountStaticAndUploads`) sehingga
  sisa `.svg` lama di disk tidak bisa eksekusi script bila dibuka langsung.
  Tindak lanjut manual: upload ulang `map-odp.svg` + `map-customer.svg` sebagai PNG
  (upload pengganti otomatis menghapus sibling `.svg` lama). Gating auth per objek
  sensitif (KTP/dokumen) dilanjutkan di P1-1.
- [x] **P0-6 — Security headers (CSP, HSTS, X-Frame-Options, nosniff)**. ✅ 2026-09-22.
  Implementasi: `SecurityHeadersMiddleware` di `internal/httpx/security.go`
  (nosniff, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`,
  CSP `default-src 'none'` kecuali `/uploads/*` yang punya sandbox sendiri),
  dipasang paling luar di `server.go`; HSTS + header duplikat untuk location
  ber-`add_header` sendiri di `deploy/nginx/d5net-billing.conf`.
  CSP sengaja tidak dipasang untuk SPA (risiko inline script Vite).
  Test `internal/httpx/security_test.go`.

## P1 — Anti bocor data antar-tenant

- [ ] **P1-1 — Perluas RLS + test IDOR otomatis + gating auth file sensitif**.
  Lokasi: `migrations/`, `internal/store/*`, `internal/handlers/*`.
  Rencana: RLS untuk tabel sensitif tersisa, semua query baca tulis filter `tenant_id`,
  dokumen/foto sensitif di `/uploads` lewat handler auth (branding publik tetap publik),
  test: token tenant A akses objek tenant B → 401/404.
- [ ] **P1-2 — RBAC deny-by-default di semua mutasi sensitif**.
  Lokasi: `internal/handlers/*`, `internal/handlers/handlers.go:538`.
  Rencana: `rolePermissionsFromCtx` di backup restore, roles, integrasi, router.
- [ ] **P1-3 — CSRF + ketatkan CORS**.
  Lokasi: `internal/httpx/server.go:89-108`, `internal/handlers/handlers.go:219-223`.
  Rencana: cek `Origin/Referer` di `/api/auth/*`, hapus auto-allow LAN di production.
- [ ] **P1-4 — Audit akses `/events/stream`**.
  Lokasi: `cmd/api/main.go:86,165`, `internal/monitor/*`.
  Rencana: pastikan SSE butuh auth + filter per tenant, atau dokumentasikan bila sengaja publik.

## P2 — Hardening

- [ ] **P2-1 — 2FA wajib untuk admin + rate-limit TOTP + backup codes**.
- [ ] **P2-2 — Password policy (max, denylist) + idle-timeout sesi**.
  (Sebagian idle-timeout: `JWT_ACCESS_TTL`/`JWT_REFRESH_TTL` di `.env`.)
- [ ] **P2-3 — Backup restore: re-auth/2FA + audit**.
- [ ] **P2-4 — Batas body global + timeout webhook + audit-log sensitif lengkap**.

## Riwayat pengerjaan

- 2026-09-22: audit awal, file ini dibuat. Belum ada item selesai.
- 2026-09-22: P0-1 selesai (rate-limit auth + batas password).
- 2026-09-22: P0-2 selesai (logout + revoke sesi + deteksi reuse).
- 2026-09-22: P0-3 selesai (validasi secret + rotasi .env lokal).
- 2026-09-22: P0-4 selesai (webhook signature selalu diverifikasi).
- 2026-09-22: P0-5 selesai (tolak SVG + header aman /uploads).
- 2026-09-22: P0-6 selesai (security headers Go + nginx). Semua P0 selesai.
