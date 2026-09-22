package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type ctxKey string

const clientIPKey ctxKey = "client-ip"

// ClientIPMiddleware menyimpan IP klien (setelah RealIP) ke ctx untuk audit log.
func ClientIPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIPFromRemoteAddr(r.RemoteAddr)
		if ip != nil {
			r = r.WithContext(context.WithValue(r.Context(), clientIPKey, ip))
		}
		next.ServeHTTP(w, r)
	})
}

func clientIPFromRemoteAddr(remoteAddr string) net.IP {
	host := strings.TrimSpace(remoteAddr)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil {
		return ip
	}
	return nil
}

// IPFromContext mengembalikan IP klien yang disimpan middleware (nil bila tak ada).
func IPFromContext(ctx context.Context) net.IP {
	if ip, ok := ctx.Value(clientIPKey).(net.IP); ok {
		return ip
	}
	return nil
}

type Server struct {
	Router chi.Router
	API    huma.API
}

func NewServer(origins []string, extra ...func(http.Handler) http.Handler) *Server {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(SecurityHeadersMiddleware)
	r.Use(middleware.RealIP)
	r.Use(ClientIPMiddleware)
	r.Use(RateLimitAuthMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowOriginFunc:  originAllowed(origins),
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Tenant-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	for _, mw := range extra {
		r.Use(mw)
	}

	config := huma.DefaultConfig("d5net-billing API", "1.0.0")
	config.Info.Description = "ISP Billing API for MikroTik RouterOS"
	api := humachi.New(r, config)

	return &Server{Router: r, API: api}
}

// originAllowed matches exact CORS_ORIGINS entries, trailing "*" prefixes,
// and LAN hosts on 192.168.100.0/24 (dev access from Wi‑Fi peers).
func originAllowed(allowed []string) func(r *http.Request, origin string) bool {
	return func(_ *http.Request, origin string) bool {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			return true
		}
		for _, a := range allowed {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			if a == "*" || a == origin {
				return true
			}
			if strings.HasSuffix(a, "*") && strings.HasPrefix(origin, strings.TrimSuffix(a, "*")) {
				return true
			}
		}
		return isDevLANOrigin(origin)
	}
}

func isDevLANOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	if !strings.HasPrefix(host, "192.168.100.") {
		return false
	}
	last := strings.TrimPrefix(host, "192.168.100.")
	n, err := strconv.Atoi(last)
	return err == nil && n >= 0 && n <= 255
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ErrorHandler(ctx context.Context, err error) {
	slog.Error("request error", "err", err)
}

type HealthOutput struct {
	Body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
}

func RegisterHealth(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "health",
		Method:      http.MethodGet,
		Path:        "/api/health",
		Summary:     "Health check",
		Tags:        []string{"System"},
	}, func(ctx context.Context, _ *struct{}) (*HealthOutput, error) {
		out := &HealthOutput{}
		out.Body.Status = "ok"
		out.Body.Version = "1.0.0"
		return out, nil
	})
}

func Unauthorized(msg string) error {
	return huma.Error401Unauthorized(msg)
}

func Forbidden(msg string) error {
	return huma.Error403Forbidden(msg)
}

func BadRequest(msg string) error {
	return huma.Error400BadRequest(msg)
}

func NotFound(msg string) error {
	return huma.Error404NotFound(msg)
}

func Internal(err error) error {
	if err != nil {
		slog.Error("internal error", "err", err)
	}
	return huma.Error500InternalServerError("internal error", err)
}

func WrapNotFound(err error, msg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	return NotFound(msg)
}
