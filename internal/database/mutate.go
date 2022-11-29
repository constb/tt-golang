package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bwmarrin/snowflake"
	"github.com/constb/tt-golang/internal/proto"
	"github.com/constb/tt-golang/internal/utils"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

func (d *BalanceDatabase) initUserBalance(ctx context.Context, userID string, currency string) error {
	_, err := d.db.Exec(ctx, `INSERT INTO balance (user_id, currency, current_value) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		userID, currency, 0,
	)
	if err != nil {
		return fmt.Errorf("ensure balance: %w", err)
	}
	return nil
}

func (d *BalanceDatabase) TopUp(ctx context.Context, idempotencyKey, userID, currency, value, merchantData string) (snowflake.ID, error) {
	if idempotencyKey == "" {
		return 0, proto.NewBadParameterError("idempotency key")
	}
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
	err = d.initUserBalance(ctx, userID, currency)
	if err != nil {
		return 0, err
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

	// x) IDEMPOTENCY CHECK
	var txID snowflake.ID
	row = tx.QueryRow(ctx, `SELECT id FROM transaction WHERE idempotency_key = $1`, idempotencyKey)
	if err = row.Scan(&txID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("idempotency check: %w", err)
	}
	if txID != 0 {
		// constb: transaction already was processed earlier, continue as if we have applied it now
		return txID, nil
	}

	// 3) CONVERT CURRENCIES IF NEEDED
	var topUpInUserCurrency decimal.Decimal
	if currency == balanceCurrency {
		topUpInUserCurrency = topUpValue
	} else {
		topUpInUserCurrency, err = ConvertCurrency(topUpValue, currency, balanceCurrency)
		if err != nil {
			return 0, fmt.Errorf("currency convert: %w", err)
		}
	}

	balanceNewValue := balanceCurrentValue.Add(topUpInUserCurrency)

	// 4) CREATE TRANSACTION RECORD AND TOP-UP BALANCE
	txID = utils.GenerateID()
	var merchantDataParam any
	if merchantData != "" {
		merchantDataParam = merchantData
	}
	_, err = tx.Exec(ctx, `
INSERT INTO transaction (id, transaction_currency, transaction_value, recipient_id, recipient_currency, recipient_value,
                         recipient_balance_before, recipient_balance_after, merchant_data, idempotency_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		txID.Int64(), currency, topUpValue, userID, balanceCurrency, topUpInUserCurrency,
		balanceCurrentValue, balanceNewValue, merchantDataParam, idempotencyKey,
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

func (d *BalanceDatabase) Reserve(ctx context.Context, userID, currency, value, orderID, itemID string) error {
	if userID == "" {
		return proto.NewBadParameterError("user id")
	}
	if !IsCurrencyValid(currency) {
		return proto.NewBadParameterError("currency")
	}
	reserveValue, err := decimal.NewFromString(value)
	if err != nil || reserveValue.LessThanOrEqual(decimal.Zero) {
		return proto.NewBadParameterError("value")
	}
	if orderID == "" {
		return proto.NewBadParameterError("order id")
	}

	// 1) CREATE ZERO BALANCE IF NOT EXISTS
	err = d.initUserBalance(ctx, userID, currency)
	if err != nil {
		return err
	}

	// x) WRAP IN TRANSACTION
	conn, err := d.db.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
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
		return fmt.Errorf("lock balance: %w", err)
	}

	// 3) SUM EXISTING USER RESERVES
	var userAlreadyReserved decimal.NullDecimal
	row = tx.QueryRow(ctx, "SELECT SUM(user_currency_value) FROM balance_reserve WHERE user_id = $1", userID)
	if err = row.Scan(&userAlreadyReserved); err != nil {
		return fmt.Errorf("read reserve: %w", err)
	}

	// x) IDEMPOTENCY CHECK
	var orderReserve int
	row = tx.QueryRow(ctx, "SELECT COUNT(*) FROM balance_reserve WHERE user_id = $1 AND order_id = $2", userID, orderID)
	if err = row.Scan(&orderReserve); err != nil {
		return fmt.Errorf("read reserve: %w", err)
	}
	if orderReserve > 0 {
		// constb: pretend we have successfully processed this reservation
		// do we need to check other parameters too? like item id? reserved value?
		return nil
	}

	// 4) CONVERT CURRENCIES IF NEEDED
	var reserveInUserCurrency decimal.Decimal
	if currency == balanceCurrency {
		reserveInUserCurrency = reserveValue
	} else {
		reserveInUserCurrency, err = ConvertCurrency(
			// constb: reserve extra 6% to compensate for possible rate changes
			reserveValue.Mul(decimal.NewFromFloat(1.06)),
			currency,
			balanceCurrency,
		)
		if err != nil {
			return fmt.Errorf("currency convert: %w", err)
		}
	}

	// 5) CHECK IF USER HAS ENOUGH MONEY
	if userAlreadyReserved.Valid {
		balanceCurrentValue = balanceCurrentValue.Sub(userAlreadyReserved.Decimal)
	}
	if reserveInUserCurrency.GreaterThan(balanceCurrentValue) {
		return proto.NewNotEnoughMoneyError()
	}

	// 6) CREATE RESERVATION
	_, err = tx.Exec(ctx, `
INSERT INTO balance_reserve (order_id, user_id, item_id, currency, "value", user_currency_value)
VALUES ($1, $2, $3, $4, $5, $6)`,
		orderID, userID, itemID, currency, reserveValue, reserveInUserCurrency,
	)
	if err != nil {
		return fmt.Errorf("save reservation: %w", err)
	}

	return nil
}
