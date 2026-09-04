package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type Server struct {
	Router chi.Router
	API    huma.API
}

func NewServer(origins []string, extra ...func(http.Handler) http.Handler) *Server {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Tenant-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	for _, mw := range extra {
		r.Use(mw)
	}

	config := huma.DefaultConfig("drp-billing API", "1.0.0")
	config.Info.Description = "ISP Billing API for MikroTik RouterOS"
	api := humachi.New(r, config)

	return &Server{Router: r, API: api}
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
