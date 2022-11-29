package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/constb/tt-golang/internal/proto"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

func (d *BalanceDatabase) FetchUserBalance(ctx context.Context, userID string) (currency string, available, reserved decimal.Decimal, err error) {
	if userID == "" {
		return "", decimal.Zero, decimal.Zero, proto.NewBadParameterError("user id")
	}

	// x) WRAP IN TRANSACTION (avoid inconsistent data with balance and reservations)
	conn, err := d.db.Acquire(ctx)
	if err != nil {
		return "", decimal.Zero, decimal.Zero, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", decimal.Zero, decimal.Zero, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		} else {
			_ = tx.Commit(ctx)
		}
	}()

	// 1) GET CURRENT BALANCE
	row := tx.QueryRow(ctx, `SELECT currency, current_value FROM balance WHERE user_id = $1`, userID)
	if err = row.Scan(&currency, &available); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = proto.NewUserNotFoundError()
		} else {
			err = fmt.Errorf("read balance: %w", err)
		}
		return "", decimal.Zero, decimal.Zero, err
	}

	// 2) GET RESERVATIONS
	var reservedRow decimal.Decimal
	var rows pgx.Rows
	rows, err = tx.Query(ctx, `SELECT user_currency_value FROM balance_reserve WHERE user_id = $1`, userID)
	if err != nil {
		return "", decimal.Zero, decimal.Zero, fmt.Errorf("read reservations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err = rows.Scan(&reservedRow); err != nil {
			return "", decimal.Zero, decimal.Zero, fmt.Errorf("read reservations: %w", err)
		}
		if reservedRow.GreaterThan(decimal.Zero) {
			reserved = reserved.Add(reservedRow)
		}
	}

	if reserved.GreaterThan(decimal.Zero) {
		available = available.Sub(reserved)
	}

	return
}
