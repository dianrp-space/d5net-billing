package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/dianrp/drp-billing/internal/envfile"
	"github.com/pressly/goose/v3"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate [up|down|status]")
	}
	envfile.Load(".env")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
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
