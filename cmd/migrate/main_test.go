package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStripPoolRuntimeParams(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://u:p@127.0.0.1:5432/db?sslmode=disable&pool_max_conns=20&pool_min_conns=2")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConns != 20 {
		t.Fatalf("MaxConns tidak terbaca: %d", cfg.MaxConns)
	}
	stripPoolRuntimeParams(cfg.ConnConfig)
	// RuntimeParams inilah yang dikirim ke server saat startup handshake.
	for k := range cfg.ConnConfig.RuntimeParams {
		if len(k) >= 5 && k[:5] == "pool_" {
			t.Fatalf("param pool lolos ke server: %s", k)
		}
	}
}
