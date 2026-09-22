package httpx

import (
	"net/http"
	"strings"
)

// SecurityHeadersMiddleware menambah header pertahanan untuk semua respons API.
// API hanya mengembalikan JSON/SSE (HTML captive isolir jalan di server
// terpisah), jadi CSP ketat aman dipakai di sini. HSTS TIDAK di-set dari Go
// karena Go berjalan sebagai HTTP polos di belakang Nginx — HSTS di-set di
// deploy/nginx/d5net-billing.conf (lapis TLS).
// P0-6 audit keamanan 2026-09-22.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		// /uploads/* sudah punya CSP sandbox sendiri (lihat MountStaticAndUploads)
		// agar header tidak dobel; sisanya kunci total (tidak ada HTML).
		if !strings.HasPrefix(r.URL.Path, "/uploads/") {
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		}
		next.ServeHTTP(w, r)
	})
}
