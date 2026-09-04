package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dianrp/drp-billing/internal/auth"
	"github.com/dianrp/drp-billing/internal/billing"
	"github.com/dianrp/drp-billing/internal/config"
	"github.com/dianrp/drp-billing/internal/db"
	"github.com/dianrp/drp-billing/internal/job"
	"github.com/dianrp/drp-billing/internal/monitor"
	"github.com/dianrp/drp-billing/internal/notify"
	"github.com/dianrp/drp-billing/internal/provisioner"
	"github.com/dianrp/drp-billing/internal/store"
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
	encryptor, err := auth.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		slog.Error("encryptor", "err", err)
		os.Exit(1)
	}

	billingEngine := billing.New(st)
	notifySvc := notify.NewService(st)
	poller := monitor.NewPoller(st, encryptor, 0)
	provReg := provisioner.NewRegistry(st, encryptor)
	worker := job.NewWorker(st, billingEngine, notifySvc, poller, provReg)

	go worker.Run(ctx)

	slog.Info("worker started")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	cancel()
	slog.Info("worker stopped")
}
