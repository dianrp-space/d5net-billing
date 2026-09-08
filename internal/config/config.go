package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/dianrp/drp-billing/internal/envfile"
)

type Config struct {
	AppEnv                     string        `env:"APP_ENV" envDefault:"development"`
	HTTPAddr                   string        `env:"HTTP_ADDR" envDefault:"0.0.0.0:8080"`
	DatabaseURL                string        `env:"DATABASE_URL,required"`
	JWTSecret                  string        `env:"JWT_SECRET,required"`
	JWTAccessTTL               time.Duration `env:"JWT_ACCESS_TTL" envDefault:"15m"`
	JWTRefreshTTL              time.Duration `env:"JWT_REFRESH_TTL" envDefault:"720h"`
	EncryptionKey              string        `env:"ENCRYPTION_KEY,required"`
	CORSOrigins                []string      `env:"CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:5173"`
	UploadDir                  string        `env:"UPLOAD_DIR" envDefault:"/var/lib/drp-billing/uploads"`
	RouterBackupDir            string        `env:"ROUTER_BACKUP_DIR" envDefault:"/var/lib/drp-billing/router-backups"`
	DBBackupDir                string        `env:"DB_BACKUP_DIR" envDefault:"./data/db-backups"`
	WhatsAppSessionDir         string        `env:"WHATSAPP_SESSION_DIR" envDefault:"./data/whatsapp"`
	WorkerEnabled              bool          `env:"WORKER_ENABLED" envDefault:"true"`
	DRPPaymentBaseURL          string        `env:"DRP_PAYMENT_BASE_URL" envDefault:"https://payment.dianrp.com"`
	DRPPaymentAPIKey           string        `env:"DRP_PAYMENT_API_KEY"`
	DRPPaymentWebhookSecret    string        `env:"DRP_PAYMENT_WEBHOOK_SECRET"`
	DRPPaymentExpiresInMinutes int           `env:"DRP_PAYMENT_EXPIRES_IN_MINUTES" envDefault:"15"`
}

func Load() (*Config, error) {
	envfile.Load(".env")
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
