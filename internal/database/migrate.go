package database

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigrations(dbUrl string) error {
	if strings.HasPrefix(dbUrl, "postgres://") {
		dbUrl = "pgx://" + dbUrl[11:]
	}
	m, err := migrate.New("file://migrations", dbUrl)
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
