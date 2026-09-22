package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := SecurityHeadersMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	for k, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Fatalf("%s: got %q, want %q", k, got, want)
		}
	}
	if got := rec.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("missing CSP on api path")
	}

	// /uploads/* memakai CSP sandbox sendiri — jangan dobel.
	req2 := httptest.NewRequest(http.MethodGet, "/uploads/x/logo.webp", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if got := rec2.Header().Get("Content-Security-Policy"); got != "" {
		t.Fatalf("uploads CSP should be left to file server, got %q", got)
	}
}
