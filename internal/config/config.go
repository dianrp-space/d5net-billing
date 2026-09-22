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
	// IsolirHTTPAddr is the plain-HTTP captive listener that serves the isolir
	// page for ANY host/path. RouterOS dst-nat redirects isolir-pool tcp/80 to
	// this address (typically IP_PUBLIK:8090). Empty disables the listener.
	IsolirHTTPAddr  string        `env:"ISOLIR_HTTP_ADDR" envDefault:"0.0.0.0:8090"`
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

// IsolirHTTPPort returns just the port of IsolirHTTPAddr (e.g. "8090").
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
	if err := cfg.validateSecrets(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// weakSecrets adalah nilai contoh/default yang tidak boleh dipakai production.
// Jangan tambahkan secret asli ke daftar ini.
var weakSecrets = map[string]bool{
	"change-me": true,
	"change-me-to-random-64-char-string-in-production": true,
	"01234567890123456789012345678901":                true,
	"00000000000000000000000000000000":                true,
}

// validateSecrets menolak start bila kunci signing/enkripsi lemah. Tanpa ini,
// instalasi yang lupa mengganti contoh .env akan menandatangani JWT dengan
// kunci yang bisa ditebak siapa saja (pemalsuan token = bobol total).
func (c *Config) validateSecrets() error {
	s := strings.TrimSpace(c.JWTSecret)
	if len(s) < 32 {
		return fmt.Errorf("JWT_SECRET wajib minimal 32 karakter acak (hasilkan via: openssl rand -hex 32)")
	}
	if weakSecrets[strings.ToLower(s)] || weakSecrets[s] {
		return fmt.Errorf("JWT_SECRET masih memakai nilai contoh/default — ganti dengan string acak")
	}
	k := strings.TrimSpace(c.EncryptionKey)
	if len(k) != 32 {
		// NewEncryptor juga menolak, tapi gagal di sini dengan pesan yang jelas.
		return fmt.Errorf("ENCRYPTION_KEY wajib tepat 32 karakter (hasilkan via: openssl rand -hex 16)")
	}
	if weakSecrets[k] {
		return fmt.Errorf("ENCRYPTION_KEY masih memakai nilai contoh/default — ganti dengan string acak")
	}
	return nil
}
