package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bwmarrin/snowflake"
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

type UserTransactionItem struct {
	Currency           string
	Value              decimal.Decimal
	UserCurrencyValue  decimal.Decimal
	IsTopUpTransaction bool
	OrderId            string
	ItemId             string
	CreatedAt          time.Time
}

func (d *BalanceDatabase) FetchUserTransactions(
	ctx context.Context,
	userID string,
	limit int,
	before snowflake.ID,
	minDate, maxDate time.Time,
) (
	items []UserTransactionItem,
	nextBefore snowflake.ID,
	total int64,
	err error,
) {
	if userID == "" {
		return nil, 0, 0, proto.NewBadParameterError("user id")
	}

	var hasMinDate, hasMaxDate, hasBefore int
	if !minDate.IsZero() {
		hasMinDate = 1
	}
	if !maxDate.IsZero() {
		hasMaxDate = 1
	}
	if before != 0 {
		hasBefore = 1
	}

	rows, err := d.db.Query(ctx, `
SELECT id,
       transaction_currency,
       transaction_value,
       sender_id,
       sender_value,
       recipient_id,
       recipient_value,
       (order_data ->> 'order_id'),
       (order_data ->> 'item_id'),
       created_at
FROM "transaction"
WHERE (sender_id = $1 OR recipient_id = $1)
  AND ($2 = 0 OR created_at >= $3)
  AND ($4 = 0 OR created_at <= $5)
  AND ($6 = 0 OR id <= $7)
ORDER BY id DESC
LIMIT $8`,
		userID, hasMinDate, minDate, hasMaxDate, maxDate, hasBefore, before, limit+1,
	)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("load user tx: %w", err)
	}
	defer rows.Close()

	items = make([]UserTransactionItem, 0, limit)
	for rows.Next() {
		next := UserTransactionItem{}
		var txID snowflake.ID
		var senderID, recipientID, orderID, itemID *string
		var txValue, senderValue, recipientValue decimal.NullDecimal
		err = rows.Scan(&txID, &next.Currency, &txValue, &senderID, &senderValue, &recipientID, &recipientValue, &orderID, &itemID, &next.CreatedAt)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("scan user tx: %w", err)
		}
		next.Value = txValue.Decimal
		if senderID != nil && *senderID == userID {
			next.UserCurrencyValue = senderValue.Decimal
		} else /*if *recipientID == userID*/ {
			next.UserCurrencyValue = recipientValue.Decimal
			next.IsTopUpTransaction = true
		}
		if orderID != nil {
			next.OrderId = *orderID
		}
		if itemID != nil {
			next.ItemId = *itemID
		}
		if len(items) < limit {
			items = append(items, next)
		} else {
			nextBefore = txID
		}
	}

	if err = d.db.QueryRow(ctx, `
SELECT COUNT(*)
FROM "transaction"
WHERE (sender_id = $1 OR recipient_id = $1)
  AND ($2 = 0 OR created_at >= $3)
  AND ($4 = 0 OR created_at <= $5)`,
		userID, hasMinDate, minDate, hasMaxDate, maxDate,
	).Scan(&total); err != nil {
		return nil, 0, 0, fmt.Errorf("count user tx: %w", err)
	}

	return
}

type StatisticsCallbacks struct {
	OnCurrencies func(currencies []string)
	OnRecord     func(item string, values map[string]decimal.Decimal)
	OnError      func(err error)
}

func (d *BalanceDatabase) FetchStatistics(ctx context.Context, year, month int, callbacks StatisticsCallbacks) {
	// x) WRAP IN TRANSACTION (avoid inconsistent data with balance and reservations)
	conn, err := d.db.Acquire(ctx)
	if err != nil {
		callbacks.OnError(fmt.Errorf("acquire connection: %w", err))
		return
	}
	defer conn.Release()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		callbacks.OnError(fmt.Errorf("begin tx: %w", err))
		return
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		} else {
			_ = tx.Commit(ctx)
		}
	}()

	// 1) LOAD LIST OF CURRENCIES USED IN TRANSACTIONS
	var rows pgx.Rows
	var currencies []string
	rows, err = tx.Query(ctx, `
select distinct transaction_currency
from "transaction"
where date_trunc('month', "created_at") = make_date($1, $2, 1)
  and (order_data ->> 'item_id') is not null`, year, month)
	if err != nil {
		callbacks.OnError(fmt.Errorf("load currencies: %w", err))
		return
	}

	for rows.Next() {
		var val string
		err = rows.Scan(&val)
		if err != nil {
			rows.Close()
			callbacks.OnError(fmt.Errorf("load currencies: %w", err))
			return
		}
		currencies = append(currencies, val)
	}
	rows.Close()

	callbacks.OnCurrencies(currencies)

	// 2) READ STATISTICS DATA
	var currentItem string
	var data map[string]decimal.Decimal
	rows, err = tx.Query(ctx, `
select order_data ->> 'item_id' as item_id, transaction_currency, sum(transaction_value)
from "transaction"
where date_trunc('month', "created_at") = make_date($1, $2, 1)
  and (order_data ->> 'item_id') is not null
group by item_id, transaction_currency
order by item_id asc, transaction_currency asc`, year, month)
	if err != nil {
		callbacks.OnError(fmt.Errorf("load statistics: %w", err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var item string
		var currency string
		var value decimal.Decimal
		err = rows.Scan(&item, &currency, &value)
		if err != nil {
			callbacks.OnError(fmt.Errorf("load statistics: %w", err))
			return
		}
		if item != currentItem {
			if currentItem != "" {
				callbacks.OnRecord(currentItem, data)
			}
			currentItem = item
			data = make(map[string]decimal.Decimal)
		}
		data[currency] = value
	}
	// flush last
	callbacks.OnRecord(currentItem, data)
}
