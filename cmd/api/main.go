package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dianrp/drp-billing/internal/auth"
	"github.com/dianrp/drp-billing/internal/billing"
	"github.com/dianrp/drp-billing/internal/config"
	"github.com/dianrp/drp-billing/internal/db"
	"github.com/dianrp/drp-billing/internal/handlers"
	"github.com/dianrp/drp-billing/internal/httpx"
	"github.com/dianrp/drp-billing/internal/monitor"
	"github.com/dianrp/drp-billing/internal/notify"
	"github.com/dianrp/drp-billing/internal/payment"
	"github.com/dianrp/drp-billing/internal/provisioner"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/tenant"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	database, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect db", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	st := store.New(database.Pool)
	tokens := auth.NewTokenService(cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL)
	encryptor, err := auth.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		slog.Error("encryptor", "err", err)
		os.Exit(1)
	}

	billingEngine := billing.New(st)
	notifySvc := notify.NewService(st)
	payments := payment.NewRegistryFromEnv(cfg.MidtransServerKey, cfg.XenditSecretKey, cfg.TripayPrivateKey)
	provReg := provisioner.NewRegistry(st, encryptor)

	srv := httpx.NewServer(cfg.CORSOrigins, tenant.Middleware(tokens), requireAPIAuth)

	deps := &handlers.Deps{
		Store: st, Tokens: tokens, Encryptor: encryptor,
		Billing: billingEngine, Notify: notifySvc, Payments: payments,
		Provisioner: provReg, Config: cfg,
	}
	handlers.RegisterAll(srv.API, deps)
	handlers.MountStaticAndUploads(srv.Router, deps)
	srv.Router.Get("/events/stream", monitor.SSEHandler(st))

	// Live OpenAPI from huma (not a placeholder stub).
	srv.Router.Get("/api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		b, err := json.Marshal(srv.API.OpenAPI())
		if err != nil {
			http.Error(w, `{"error":"openapi marshal failed"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(b)
	})

	server := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      srv.Router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("starting api server", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	slog.Info("api server stopped")
}

func requireAPIAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		public := path == "/health" ||
			path == "/api/health" ||
			strings.HasPrefix(path, "/api/auth") ||
			strings.HasPrefix(path, "/api/public") ||
			strings.HasPrefix(path, "/api/portal") ||
			strings.HasPrefix(path, "/api/webhooks") ||
			strings.HasPrefix(path, "/uploads/") ||
			path == "/api/openapi.json" ||
			strings.HasPrefix(path, "/docs") ||
			strings.HasPrefix(path, "/openapi") ||
			path == "/events/stream"
		if strings.HasPrefix(path, "/api/") && !public {
			if _, ok := tenant.FromContext(r.Context()); !ok {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
