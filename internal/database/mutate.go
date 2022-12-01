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
	"github.com/jackc/pgx/v5/pgconn"
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
	var orderReserve, orderTx int
	row = tx.QueryRow(ctx, "SELECT COUNT(*) FROM balance_reserve WHERE user_id = $1 AND order_id = $2", userID, orderID)
	if err = row.Scan(&orderReserve); err != nil {
		return fmt.Errorf("read reserve: %w", err)
	}
	if orderReserve > 0 {
		// constb: pretend we have successfully processed this reservation
		// do we need to check other parameters too? like item id? reserved value?
		return nil
	}
	row = tx.QueryRow(ctx, "SELECT COUNT(*) FROM transaction WHERE order_data->>'order_id' = $1", orderID)
	if err = row.Scan(&orderTx); err != nil {
		return fmt.Errorf("read tx: %w", err)
	}
	if orderTx > 0 {
		// constb: reserving money for already committed transaction? definitely an error!
		// important: set err to Rollback transaction
		err = proto.NewInvalidStateError()
		return err
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

func (d *BalanceDatabase) CommitReservation(ctx context.Context, userID, currency, value, orderID, itemID string) (snowflake.ID, error) {
	if userID == "" {
		return 0, proto.NewBadParameterError("user id")
	}
	if !IsCurrencyValid(currency) {
		return 0, proto.NewBadParameterError("currency")
	}
	commitValue, err := decimal.NewFromString(value)
	if err != nil || commitValue.LessThanOrEqual(decimal.Zero) {
		return 0, proto.NewBadParameterError("value")
	}
	if orderID == "" {
		return 0, proto.NewBadParameterError("order id")
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
	row = tx.QueryRow(ctx, "SELECT id FROM transaction WHERE order_data->>'order_id' = $1", orderID)
	if err = row.Scan(&txID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("locate transaction: %w", err)
	}
	if txID > 0 {
		// constb: pretend we have successfully committed this reservation
		// do we need to check other parameters too? like user id? currency and value?
		return txID, nil
	}

	// 3) DELETE RESERVATION IF WAS PREVIOUSLY MADE
	var res pgconn.CommandTag
	res, err = tx.Exec(ctx, `DELETE FROM balance_reserve WHERE order_id = $1`, orderID)
	if err != nil {
		return 0, fmt.Errorf("delete reservation: %w", err)
	}
	previouslyReserved := res.RowsAffected() > 0

	// 4) CALCULATE ACTUAL TRANSACTION VALUE
	var commitInUserCurrency decimal.Decimal
	if currency == balanceCurrency {
		commitInUserCurrency = commitValue
	} else {
		commitInUserCurrency, err = ConvertCurrency(commitValue, currency, balanceCurrency)
		if err != nil {
			return 0, fmt.Errorf("currency convert: %w", err)
		}
	}

	balanceNewValue := balanceCurrentValue.Sub(commitInUserCurrency)

	if balanceNewValue.LessThan(decimal.Zero) && (currency == balanceCurrency || !previouslyReserved) {
		// constb only allow overdraft on reservations in different currency, otherwise generate error
		// important: set err to Rollback transaction
		err = proto.NewNotEnoughMoneyError()
		return 0, err
	}

	// 5) CREATE TRANSACTION RECORD AND CHARGE FROM BALANCE
	txID = utils.GenerateID()
	var orderDataParam []byte
	orderDataParam, err = json.Marshal(struct {
		OrderID string `json:"order_id"`
		ItemID  string `json:"item_id,omitempty"`
	}{orderID, itemID})
	_, err = tx.Exec(ctx, `
INSERT INTO transaction (id, transaction_currency, transaction_value, sender_id, sender_currency, sender_value,
                         sender_balance_before, sender_balance_after, order_data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		txID.Int64(), currency, commitValue, userID, balanceCurrency, commitInUserCurrency,
		balanceCurrentValue, balanceNewValue, orderDataParam,
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

func (d *BalanceDatabase) CancelReservation(ctx context.Context, userID, orderID, itemID string) error {
	if userID == "" {
		return proto.NewBadParameterError("user id")
	}
	if orderID == "" {
		return proto.NewBadParameterError("order id")
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

	// 1) LOAD BALANCE AND LOCK FOR UPDATE
	var balanceCurrency string
	var balanceCurrentValue decimal.Decimal

	row := tx.QueryRow(ctx, "SELECT currency, current_value FROM balance WHERE user_id = $1 FOR UPDATE", userID)
	if err = row.Scan(&balanceCurrency, &balanceCurrentValue); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// must set err to trigger Rollback
			err = proto.NewUserNotFoundError()
			return err
		}
		return fmt.Errorf("lock balance: %w", err)
	}

	// 2) FIND RESERVATION
	var reservationUserID string

	row = tx.QueryRow(ctx, "SELECT user_id FROM balance_reserve WHERE order_id = $1", orderID)
	if err = row.Scan(&reservationUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// no reservation
			var orderTxCount int
			row = tx.QueryRow(ctx, "SELECT COUNT(*) FROM transaction WHERE (order_data->>'order_id') = $1", orderID)
			if err = row.Scan(&orderTxCount); err != nil {
				return fmt.Errorf("locate order tx: %w", err)
			}
			// constb: if we don't have a transaction for this order – cancel call is duplicate
			// pretend we have cancelled just now
			if orderTxCount == 0 {
				return nil
			}
			// …otherwise this order is already committed. must set err to trigger Rollback
			err = proto.NewInvalidStateError()
			return err
		}
		return fmt.Errorf("locate reservation: %w", err)
	}
	if reservationUserID != userID {
		// must set err to trigger Rollback
		err = proto.NewBadParameterError("user id")
		return err
	}

	// 3) REMOVE RESERVATION
	_, err = tx.Exec(ctx, "DELETE FROM balance_reserve WHERE order_id = $1", orderID)
	if err != nil {
		return fmt.Errorf("remove reservation: %w", err)
	}

	return nil
}
