package httpx

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Batas rate-limit untuk endpoint autentikasi (anti brute-force + anti CPU-DoS
// Argon2 di /api/auth/login). Jendela fixed-window per IP per endpoint.
const (
	authRateWindow = time.Minute
	// Login (password + TOTP) sangat mahal (Argon2) → batas ketat.
	authLoginPerMinute = 10
	// Refresh murah (HMAC verify) tapi tetap dibatasi agar tidak jadi oracle.
	authRefreshPerMinute = 30
)

// authRateKey mengelompokkan hit per IP + endpoint agar satu IP yang
// menyerang satu akun tidak menghabiskan kuota endpoint lain.
func authRateKey(r *http.Request) (key, path string, limit int, ok bool) {
	if r.Method != http.MethodPost {
		return "", "", 0, false
	}
	switch r.URL.Path {
	case "/api/auth/login", "/api/portal/login":
		return rateKey(r, "login"), r.URL.Path, authLoginPerMinute, true
	case "/api/auth/refresh":
		return rateKey(r, "refresh"), r.URL.Path, authRefreshPerMinute, true
	default:
		return "", "", 0, false
	}
}

func rateKey(r *http.Request, scope string) string {
	return clientIPString(r) + "|" + scope
}

func clientIPString(r *http.Request) string {
	if ip := IPFromContext(r.Context()); ip != nil {
		return ip.String()
	}
	host := strings.TrimSpace(r.RemoteAddr)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	if host != "" {
		return host
	}
	return "unknown"
}

// RateLimitAuthMiddleware membatasi request ke endpoint auth per IP.
// Melebihi batas → 429 + header Retry-After (detik). Endpoint lain lolos.
func RateLimitAuthMiddleware(next http.Handler) http.Handler {
	lim := newAuthRateLimiter()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, _, limit, ok := authRateKey(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		if retryAfter := lim.allow(key, limit, authRateWindow); retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"too many requests, try again later"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type authRateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newAuthRateLimiter() *authRateLimiter {
	return &authRateLimiter{hits: make(map[string][]time.Time)}
}

// allow mencatat 1 hit dan mengembalikan 0 bila masih dalam kuota,
// atau sisa detik jendela bila sudah melebihi batas.
func (l *authRateLimiter) allow(key string, limit int, window time.Duration) (retryAfter int) {
	now := time.Now()
	cutoff := now.Add(-window)
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		// Jendela dihitung dari hit tertua agar klien tahu kapan boleh retry.
		wait := kept[0].Add(window).Sub(now)
		if wait < time.Second {
			wait = time.Second
		}
		l.hits[key] = kept
		return int(wait.Seconds()) + 1
	}
	l.hits[key] = append(kept, now)
	// Bersihkan oportunistik agar map tidak tumbuh tanpa batas.
	if len(l.hits) > 10000 {
		for k, v := range l.hits {
			if len(v) == 0 {
				delete(l.hits, k)
			}
		}
	}
	return 0
}
