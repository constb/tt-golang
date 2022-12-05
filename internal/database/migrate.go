package database

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgconn"
)

func RunMigrations(dbURL string) error {
	if strings.HasPrefix(dbURL, "postgres://") {
		dbURL = "pgx://" + dbURL[11:]
	}

	attempts := 5
	for attempts > 0 {
		m, err := migrate.New("file://migrations", dbURL)
		if err != nil {
			if !pgconn.SafeToRetry(err) {
				return fmt.Errorf("load migrations: %w", err)
			}
		} else if err = m.Up(); err != nil {
			if errors.Is(err, migrate.ErrNoChange) {
				return nil
			}
			if !pgconn.SafeToRetry(err) {
				return fmt.Errorf("run migrations: %w", err)
			}
		}

		time.Sleep(3 * time.Second)
		attempts--
	}
	return nil
}
