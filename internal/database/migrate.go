package database

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigrations(dbURL string) error {
	if strings.HasPrefix(dbURL, "postgres://") {
		dbURL = "pgx://" + dbURL[11:]
	}
	m, err := migrate.New("file://migrations", dbURL)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	if err = m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			return nil
		}
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
