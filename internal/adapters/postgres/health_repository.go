package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type healthRepo struct {
	pool *pgxpool.Pool
}

// Ping проверяет доступность PostgreSQL с ограничениями переданного контекста.
func (r *healthRepo) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}
