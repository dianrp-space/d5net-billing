package config

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/dianrp-space/d5net-billing/internal/envfile"
)

type Config struct {
	AppEnv   string `env:"APP_ENV" envDefault:"development"`
	HTTPAddr string `env:"HTTP_ADDR" envDefault:"0.0.0.0:8088"`
	// IsolirHTTPAddr is where the Go captive listener binds.
	// Production (nginx/aaPanel): 127.0.0.1:8090 — public via nginx :80 default_server.
	// Dev without nginx: 0.0.0.0:8090. Empty disables the listener.
	IsolirHTTPAddr  string        `env:"ISOLIR_HTTP_ADDR" envDefault:"127.0.0.1:8090"`
	DatabaseURL     string        `env:"DATABASE_URL,required"`
	JWTSecret       string        `env:"JWT_SECRET,required"`
	JWTAccessTTL    time.Duration `env:"JWT_ACCESS_TTL" envDefault:"15m"`
	JWTRefreshTTL   time.Duration `env:"JWT_REFRESH_TTL" envDefault:"720h"`
	EncryptionKey   string        `env:"ENCRYPTION_KEY,required"`
	CORSOrigins     []string      `env:"CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:5173"`
	UploadDir       string        `env:"UPLOAD_DIR" envDefault:"./data/uploads"`
	RouterBackupDir string        `env:"ROUTER_BACKUP_DIR" envDefault:"./data/router-backups"`
	DBBackupDir     string        `env:"DB_BACKUP_DIR" envDefault:"./data/db-backups"`
	WorkerEnabled   bool          `env:"WORKER_ENABLED" envDefault:"true"`
}

// IsolirHTTPPort returns the listen port of IsolirHTTPAddr (internal). This is
// NOT necessarily the RouterOS dst-nat to-ports (often 80 behind nginx).
func (c *Config) IsolirHTTPPort() string {
	addr := strings.TrimSpace(c.IsolirHTTPAddr)
	if addr == "" {
		return ""
	}
	if _, port, err := net.SplitHostPort(addr); err == nil && port != "" {
		return port
	}
	return strings.TrimPrefix(addr, ":")
}

func Load() (*Config, error) {
	envfile.Load(".env")
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
