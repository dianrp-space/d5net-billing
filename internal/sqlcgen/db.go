package sqlcgen

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Queries is the sqlc-style accessor. Generated output lives here after `sqlc generate`.
type Queries struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Queries {
	return &Queries{db: db}
}

func (q *Queries) Ping(ctx context.Context) error {
	_, err := q.db.Exec(ctx, `SELECT 1`)
	return err
}
