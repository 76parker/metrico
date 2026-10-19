package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/golang-migrate/migrate/v4"
	"github.com/jackc/pgx/v5/stdlib"
)

// Migrate применяет все недостающие миграции из указанного каталога.
// Отсутствие новых миграций считается успешным результатом.
func (r *Repository) Migrate(ctx context.Context, migrationDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	path, err := filepath.Abs(migrationDir)
	if err != nil {
		return fmt.Errorf("resolve migration directory: %w", err)
	}

	sourceURL := (&url.URL{
		Scheme: "file",
		Path:   path,
	}).String()

	db := stdlib.OpenDB(*r.pool.Config().ConnConfig)

	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
	if err != nil {
		_ = db.Close()
		return fmt.Errorf("create migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(sourceURL, "pgx5", driver)
	if err != nil {
		_ = driver.Close()
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
