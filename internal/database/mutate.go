package database

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bwmarrin/snowflake"
	"github.com/constb/tt-golang/internal/proto"
	"github.com/constb/tt-golang/internal/utils"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

func (d *BalanceDatabase) TopUp(ctx context.Context, userID, currency, value, merchantData string) (snowflake.ID, error) {
	if userID == "" {
		return 0, proto.NewBadParameterError("user id")
	}
	if !IsCurrencyValid(currency) {
		return 0, proto.NewBadParameterError("currency")
	}
	topUpValue, err := decimal.NewFromString(value)
	if err != nil || topUpValue.LessThanOrEqual(decimal.Zero) {
		return 0, proto.NewBadParameterError("value")
	}
	if merchantData != "" && !json.Valid([]byte(merchantData)) {
		return 0, proto.NewBadParameterError("merchant data")
	}

	// 1) CREATE ZERO BALANCE IF NOT EXISTS
	_, err = d.db.Exec(ctx, `INSERT INTO balance (user_id, currency, current_value) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		userID, currency, 0,
	)
	if err != nil {
		return 0, fmt.Errorf("ensure balance: %w", err)
	}

	// x) WRAP IN TRANSACTION
	conn, err := d.db.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		} else {
			_ = tx.Commit(ctx)
		}
	}()

	// 2) LOAD BALANCE AND LOCK FOR UPDATE
	var balanceCurrency string
	var balanceCurrentValue decimal.Decimal

	row := tx.QueryRow(ctx, "SELECT currency, current_value FROM balance WHERE user_id = $1 FOR UPDATE", userID)
	if err = row.Scan(&balanceCurrency, &balanceCurrentValue); err != nil {
		return 0, fmt.Errorf("lock balance: %w", err)
	}

	// 3) CONVERT CURRENCIES IF NEEDED
	var topUpInUserCurrency decimal.Decimal
	if currency == balanceCurrency {
		topUpInUserCurrency = topUpValue
	} else {
		topUpInUserCurrency, err = ConvertCurrency(topUpValue, currency, balanceCurrency)
		if err != nil {
			return 0, fmt.Errorf("top-up currency convert: %w", err)
		}
	}

	balanceNewValue := balanceCurrentValue.Add(topUpInUserCurrency)

	// 4) CREATE TRANSACTION RECORD AND TOP-UP BALANCE
	txID := utils.GenerateID()
	var merchantDataParam any
	if merchantData != "" {
		merchantDataParam = merchantData
	}
	_, err = tx.Exec(ctx, `
INSERT INTO transaction (id, transaction_currency, transaction_value, recipient_id, recipient_currency, recipient_value,
                         recipient_balance_before, recipient_balance_after, merchant_data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		txID.Int64(), currency, topUpValue, userID, balanceCurrency, topUpInUserCurrency,
		balanceCurrentValue, balanceNewValue, merchantDataParam,
	)
	if err != nil {
		return 0, fmt.Errorf("save user tx: %w", err)
	}
	_, err = tx.Exec(ctx, `UPDATE balance SET current_value = $2 WHERE user_id = $1`,
		userID, balanceNewValue,
	)
	if err != nil {
		return 0, fmt.Errorf("update balance: %w", err)
	}

	return txID, nil
}
