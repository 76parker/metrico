// Package postgres реализует хранение метрик и проверку доступности PostgreSQL.
package postgres

import (
	"github.com/76Parker/metrico/internal/adapters/postgres/pgen"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository хранит метрики в PostgreSQL, проверяет соединение и применяет миграции.
type Repository struct {
	pool *pgxpool.Pool
	db   *pgen.Queries
	*healthRepo
	*metricsRepo
}

// NewRepository создаёт адаптер поверх переданного пула соединений.
// Вызывающий код отвечает за проверку и закрытие пула.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:        pool,
		db:          pgen.New(pool),
		healthRepo:  &healthRepo{pool: pool},
		metricsRepo: &metricsRepo{q: pgen.New(pool)},
	}
}
