package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func balanceDatabaseCleanup(t *testing.T) (*BalanceDatabase, bool) {
	if os.Getenv("DB_URL") == "" {
		t.Skipf("db tests require database")
		return nil, false
	}
	if os.Getenv("ALLOW_ERASE_DATABASE_CONTENT") != "yes" {
		t.Skipf("tests need to be allowed to erase database content")
		return nil, false
	}

	for {
		dir, _ := os.Getwd()
		if len(dir) <= 1 {
			t.Skipf("project root folder")
			return nil, false
		}
		if strings.HasSuffix(dir, "tt-golang") {
			break
		}
		_ = os.Chdir("..")
	}
	db, err := NewDatabaseConnection()
	if err != nil {
		t.Fatal(err)
	}
	//goland:noinspection SqlWithoutWhere
	_, _ = db.db.Exec(context.TODO(), `DELETE FROM "transaction"`)
	//goland:noinspection SqlWithoutWhere
	_, _ = db.db.Exec(context.TODO(), `DELETE FROM "balance_reserve"`)
	//goland:noinspection SqlWithoutWhere
	_, _ = db.db.Exec(context.TODO(), `DELETE FROM "balance"`)

	return db, true
}

func TestBalanceDatabase_TopUp(t *testing.T) {
	db, ok := balanceDatabaseCleanup(t)
	if !ok {
		return
	}

	type args struct {
		idempotencyKey string
		userID         string
		currency       string
		value          string
		merchantData   string
	}
	tests := []struct {
		name         string
		args         args
		wantID       bool
		wantErr      assert.ErrorAssertionFunc
		wantCurrency string
		wantBalance  decimal.Decimal
	}{
		// validations
		{"no idempotency key", args{"", "kwa", "USD", "0.00", ""}, false, assert.Error, "", decimal.Zero},
		{"zero top-up", args{"id1", "kwa", "USD", "0.00", ""}, false, assert.Error, "", decimal.Zero},
		{"invalid value", args{"id1", "kwa", "USD", "20.0.0", ""}, false, assert.Error, "", decimal.Zero},
		{"bad user id", args{"id1", "", "USD", "20.00", `{"test":true}`}, false, assert.Error, "", decimal.Zero},
		{"invalid currency", args{"id1", "kwa", "xxx", "20.00", ""}, false, assert.Error, "", decimal.Zero},
		// actual top-up
		{"good top-up", args{"id2", "kwa", "USD", "20.00", `{"test":true}`}, true, assert.NoError, "USD", decimal.NewFromInt(20)},
		{"second top-up", args{"id3", "kwa", "USD", "30.00", ``}, true, assert.NoError, "USD", decimal.NewFromInt(50)},
		{"another currency top-up", args{"id4", "kwa", "TRY", "500.00", ``}, true, assert.NoError, "USD", decimal.NewFromFloat(76.85)},
		{"another user top-up", args{"id5", "meow", "TRY", "200.00", ``}, true, assert.NoError, "TRY", decimal.NewFromInt(200)},
		{"duplicate top-up", args{"id5", "meow", "TRY", "200.00", ``}, true, assert.NoError, "TRY", decimal.NewFromInt(200)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.TopUp(context.TODO(), tt.args.idempotencyKey, tt.args.userID, tt.args.currency, tt.args.value, tt.args.merchantData)
			if !tt.wantErr(t, err, fmt.Sprintf("TopUp(%v, %v, %v, %v, %v)", "ctx", tt.args.userID, tt.args.currency, tt.args.value, tt.args.merchantData)) {
				return
			}
			if tt.wantID {
				assert.NotEqualf(t, 0, got, "TopUp(%v, %v, %v, %v, %v)", "ctx", tt.args.userID, tt.args.currency, tt.args.value, tt.args.merchantData)

				var gotCurrency string
				var gotBalance decimal.Decimal
				row := db.db.QueryRow(context.TODO(), `SELECT currency, current_value FROM balance WHERE user_id = $1`, tt.args.userID)
				_ = row.Scan(&gotCurrency, &gotBalance)
				assert.Equalf(t, tt.wantCurrency, gotCurrency, "currency %s/%s", tt.wantCurrency, gotCurrency)
				assert.Truef(t, tt.wantBalance.Equal(gotBalance), "balance %s/%s", tt.wantBalance.String(), gotBalance.String())

				var txRecipient, txCurrency string
				var txValue decimal.Decimal
				row = db.db.QueryRow(context.TODO(), `SELECT recipient_id, transaction_currency, transaction_value FROM "transaction" WHERE id = $1`, got.Int64())
				_ = row.Scan(&txRecipient, &txCurrency, &txValue)
				assert.Equalf(t, tt.args.userID, txRecipient, "transaction recipient")
				assert.Equalf(t, tt.args.currency, txCurrency, "transaction currency")
				argValue, _ := decimal.NewFromString(tt.args.value)
				assert.Equalf(t, argValue, txValue, "transaction value")
			}
		})
	}
}

func TestBalanceDatabase_Reserve(t *testing.T) {
	db, ok := balanceDatabaseCleanup(t)
	if !ok {
		return
	}

	_, _ = db.TopUp(context.TODO(), "reserve_Test_1", "miguel", "EUR", "50.00", "")
	_, _ = db.TopUp(context.TODO(), "reserve_Test_2", "orlando", "EUR", "200.00", "")

	errNoMoneyError := assert.ErrorAssertionFunc(func(t assert.TestingT, err error, i ...interface{}) bool {
		return assert.ErrorContainsf(t, err, "not enough money", "not a money error %v", err)
	})

	type args struct {
		userID   string
		currency string
		value    string
		orderID  string
		itemID   string
	}
	tests := []struct {
		name        string
		args        args
		wantErr     assert.ErrorAssertionFunc
		wantReserve decimal.Decimal
	}{
		{"no user id", args{"", "EUR", "100.00", "order001", ""}, assert.Error, decimal.Zero},
		{"no order id", args{"miguel", "EUR", "100.00", "", ""}, assert.Error, decimal.Zero},

		{"success, same currency", args{"orlando", "EUR", "100.00", "order002", "item"}, assert.NoError, decimal.NewFromFloat(100)},
		{"success, duplicate", args{"orlando", "EUR", "100.00", "order002", "item"}, assert.NoError, decimal.NewFromFloat(100)},
		{"success, diff currency", args{"orlando", "USD", "50.00", "order003", "item"}, assert.NoError, decimal.NewFromFloat(151.23)},
		{"fail, diff currency, no extra 6%", args{"orlando", "USD", "50.00", "order004", "item"}, errNoMoneyError, decimal.NewFromFloat(151.23)},
		{"fail, no money", args{"miguel", "EUR", "100.00", "order005", "item"}, errNoMoneyError, decimal.Zero},

		{"fail, no user", args{"sammy", "EUR", "100.00", "order006", ""}, errNoMoneyError, decimal.Zero},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.Reserve(context.TODO(), tt.args.userID, tt.args.currency, tt.args.value, tt.args.orderID, tt.args.itemID)
			tt.wantErr(t, err, fmt.Sprintf("Reserve(%v, %v, %v, %v, %v, %v)", "ctx", tt.args.userID, tt.args.currency, tt.args.value, tt.args.orderID, tt.args.itemID))
			if tt.args.userID != "" {
				_, _, reserve, err := db.FetchUserBalance(context.TODO(), tt.args.userID)
				assert.NoErrorf(t, err, "FetchUserBalance(%v)", tt.args.userID)
				assert.Truef(t, reserve.Equal(tt.wantReserve), "reserve got: %v want: %v", reserve.String(), tt.wantReserve.String())
			}
		})
	}
}

func TestBalanceDatabase_CommitReservation(t *testing.T) {
	db, ok := balanceDatabaseCleanup(t)
	if !ok {
		return
	}

	_, _ = db.TopUp(context.TODO(), "reserve_Test_1", "miguel", "EUR", "50.00", "")
	_, _ = db.TopUp(context.TODO(), "reserve_Test_2", "orlando", "EUR", "200.00", "")

	errNoMoneyError := assert.ErrorAssertionFunc(func(t assert.TestingT, err error, i ...interface{}) bool {
		return assert.ErrorContainsf(t, err, "not enough money", "not a money error %v", err)
	})

	type args struct {
		userID   string
		currency string
		value    string
		orderID  string
		itemID   string
	}
	tests := []struct {
		name        string
		reserve     bool
		args        args
		wantErr     assert.ErrorAssertionFunc
		wantBalance decimal.Decimal
		wantReserve decimal.Decimal
	}{
		{"success, same currency, no reserve", false, args{"orlando", "EUR", "10.00", "order1", ""}, assert.NoError, decimal.NewFromInt(190), decimal.Zero},
		{"success, duplicate, no reserve", false, args{"orlando", "EUR", "10.00", "order1", ""}, assert.NoError, decimal.NewFromInt(190), decimal.Zero},
		{"success, same currency, with reserve", true, args{"orlando", "EUR", "10.00", "order2", ""}, assert.NoError, decimal.NewFromInt(180), decimal.Zero},
		{"success, diff currency, no reserve", false, args{"orlando", "TRY", "50.00", "order3", ""}, assert.NoError, decimal.NewFromFloat(177.4), decimal.Zero},
		{"success, diff currency, with reserve", true, args{"orlando", "TRY", "50.00", "order4", ""}, assert.NoError, decimal.NewFromFloat(174.8), decimal.Zero},

		{"fail, not enough money", false, args{"miguel", "EUR", "100.00", "order5", ""}, errNoMoneyError, decimal.NewFromInt(50), decimal.Zero},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.reserve {
				_, _, reserve, err := db.FetchUserBalance(context.TODO(), tt.args.userID)
				assert.NoErrorf(t, err, "FetchUserBalance(%v)", tt.args.userID)
				assert.Falsef(t, reserve.GreaterThan(decimal.Zero), "reserve got: %v want: %v", reserve.String(), "0")

				err = db.Reserve(context.TODO(), tt.args.userID, tt.args.currency, tt.args.value, tt.args.orderID, tt.args.itemID)
				assert.NoErrorf(t, err, "Reserve(%v, %v, %v, %v, %v, %v)", "ctx", tt.args.userID, tt.args.currency, tt.args.value, tt.args.orderID, tt.args.itemID)

				_, _, reserve, err = db.FetchUserBalance(context.TODO(), tt.args.userID)
				assert.NoErrorf(t, err, "FetchUserBalance(%v)", tt.args.userID)
				assert.Truef(t, reserve.GreaterThan(decimal.Zero), "reserve got: %v want: %v", reserve.String(), "> 0")
			}
			_, err := db.CommitReservation(context.TODO(), tt.args.userID, tt.args.currency, tt.args.value, tt.args.orderID, tt.args.itemID)
			tt.wantErr(t, err, fmt.Sprintf("CommitReservation(%v, %v, %v, %v, %v, %v)", "ctx", tt.args.userID, tt.args.currency, tt.args.value, tt.args.orderID, tt.args.itemID))
			if tt.args.userID != "" {
				_, available, reserve, err := db.FetchUserBalance(context.TODO(), tt.args.userID)
				assert.NoErrorf(t, err, "FetchUserBalance(%v)", tt.args.userID)
				assert.Truef(t, available.Equal(tt.wantBalance), "available got: %v want: %v", available.String(), tt.wantBalance.String())
				assert.Truef(t, reserve.Equal(tt.wantReserve), "reserve got: %v want: %v", reserve.String(), tt.wantReserve.String())
			}
		})
	}

	t.Run("overdraft scenario", func(t *testing.T) {
		loadRatesFromStub()
		t.Cleanup(loadRatesFromStub)
		userID := "miguel"
		currency := "USD"
		value := "50.00"
		orderID := "orderX"
		itemID := "item"
		firstUsdRate := decimal.NewFromFloat(1.1)
		firstReserve, _ := decimal.NewFromString(value)
		firstReserve = firstReserve.Mul(decimal.NewFromFloat(1.06)).Div(firstUsdRate).RoundBank(2)
		secondUsdRate := decimal.NewFromFloat(0.9)
		secondCommit, _ := decimal.NewFromString(value)
		secondCommit = secondCommit.Div(secondUsdRate).RoundBank(2)

		// ensure no reserve
		_, _, reserve, err := db.FetchUserBalance(context.TODO(), userID)
		assert.NoErrorf(t, err, "FetchUserBalance(%v)", userID)
		assert.Falsef(t, reserve.GreaterThan(decimal.Zero), "reserve got: %v want: %v", reserve.String(), "0")

		// reserve with first currency rate + 6%
		rates["USD"] = firstUsdRate

		err = db.Reserve(context.TODO(), userID, currency, value, orderID, itemID)
		assert.NoErrorf(t, err, "Reserve(%v, %v, %v, %v, %v, %v)", "ctx", userID, currency, value, orderID, itemID)

		// ensure reserved
		_, available, reserve, err := db.FetchUserBalance(context.TODO(), userID)
		assert.NoErrorf(t, err, "FetchUserBalance(%v)", userID)
		assert.Truef(t, available.Equal(decimal.NewFromInt(50).Sub(firstReserve)), "available got: %v want: %v", available.String(), decimal.NewFromInt(50).Sub(firstReserve).String())
		assert.Truef(t, reserve.Equal(firstReserve), "reserve got: %v want: %v", reserve.String(), firstReserve.String())

		// commit with second currency rate precisely, rate makes user go above balance, cause overdraft
		rates["USD"] = secondUsdRate
		txID, err := db.CommitReservation(context.TODO(), userID, currency, value, orderID, itemID)
		assert.NoErrorf(t, err, "CommitReservation(%v, %v, %v, %v, %v, %v)", "ctx", userID, currency, value, orderID, itemID)
		assert.Greaterf(t, txID.Int64(), int64(0), "txID %v > 0", txID)

		// ensure overdraft is allowed, balance becomes negative
		_, available, reserve, err = db.FetchUserBalance(context.TODO(), userID)
		assert.NoErrorf(t, err, "FetchUserBalance(%v)", userID)
		assert.Truef(t, available.Equal(decimal.NewFromInt(50).Sub(secondCommit)), "available got: %v want: %v", available.String(), decimal.NewFromInt(50).Sub(secondCommit).String())
		assert.Truef(t, reserve.Equal(decimal.Zero), "reserve got: %v want: %v", reserve.String(), "0")
	})
}
