package database

import (
	"context"
	"os"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BalanceDatabase struct {
	db *pgxpool.Pool
	// TODO: add caching
}

func NewDatabaseConnection() (*BalanceDatabase, error) {
	url := os.Getenv("DB_URL")
	if url == "" {
		panic("DB_URL is not defined")
	}

	if err := RunMigrations(url); err != nil {
		return nil, err
	}

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}

	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		pgxdecimal.Register(conn.TypeMap())
		return nil
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}

	return &BalanceDatabase{pool}, nil
}
