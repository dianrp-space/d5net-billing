package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dianrp-space/d5net-billing/internal/envfile"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// stripPoolRuntimeParams menghapus parameter pool_* dari RuntimeParams.
// pgxpool.ParseConfig memahami pool_max_conns dkk untuk sisi klien, tapi
// membiarkannya di ConnConfig.RuntimeParams sehingga ikut dikirim ke server
// saat startup → FATAL: unrecognized configuration parameter (SQLSTATE 42704).
func stripPoolRuntimeParams(cfg *pgx.ConnConfig) {
	for k := range cfg.RuntimeParams {
		if strings.HasPrefix(k, "pool_") {
			delete(cfg.RuntimeParams, k)
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate [up|down|status]")
	}
	envfile.Load(".env")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL required")
	}
	// Parse via pgxpool agar parameter khusus pool (pool_max_conns, dll)
	// tidak diteruskan ke server sebagai runtime parameter (FATAL 42704).
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatal(err)
	}
	stripPoolRuntimeParams(poolCfg.ConnConfig)
	db := stdlib.OpenDB(*poolCfg.ConnConfig)
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}
	dir := "migrations"
	if v := os.Getenv("MIGRATIONS_DIR"); v != "" {
		dir = v
	}
	cmd := os.Args[1]
	ctx := context.Background()
	switch cmd {
	case "up":
		err = goose.UpContext(ctx, db, dir)
	case "down":
		err = goose.DownContext(ctx, db, dir)
	case "status":
		err = goose.StatusContext(ctx, db, dir)
	default:
		log.Fatal(fmt.Sprintf("unknown command: %s", cmd))
	}
	if err != nil {
		log.Fatal(err)
	}
}
