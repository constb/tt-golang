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

func TestBalanceDatabase_TopUp(t *testing.T) {
	if os.Getenv("DB_URL") == "" {
		t.Skipf("db tests require database")
		return
	}
	if os.Getenv("ALLOW_ERASE_DATABASE_CONTENT") != "yes" {
		t.Skipf("tests need to be allowed to erase database content")
		return
	}

	for {
		dir, _ := os.Getwd()
		if len(dir) <= 1 {
			t.Skipf("project root folder")
			return
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

	type args struct {
		userId       string
		currency     string
		value        string
		merchantData string
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
		{"zero top-up", args{"kwa", "USD", "0.00", ""}, false, assert.Error, "", decimal.Zero},
		{"invalid value", args{"kwa", "USD", "20.0.0", ""}, false, assert.Error, "", decimal.Zero},
		{"bad user id", args{"", "USD", "20.00", `{"test":true}`}, false, assert.Error, "", decimal.Zero},
		{"invalid currency", args{"kwa", "xxx", "20.00", ""}, false, assert.Error, "", decimal.Zero},
		// actual top-up
		{"good top-up", args{"kwa", "USD", "20.00", `{"test":true}`}, true, assert.NoError, "USD", decimal.NewFromInt(20)},
		{"second top-up", args{"kwa", "USD", "30.00", ``}, true, assert.NoError, "USD", decimal.NewFromInt(50)},
		{"another currency top-up", args{"kwa", "TRY", "500.00", ``}, true, assert.NoError, "USD", decimal.NewFromFloat(76.85)},
		{"another user top-up", args{"meow", "TRY", "200.00", ``}, true, assert.NoError, "TRY", decimal.NewFromInt(200)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.TopUp(context.TODO(), tt.args.userId, tt.args.currency, tt.args.value, tt.args.merchantData)
			if !tt.wantErr(t, err, fmt.Sprintf("TopUp(%v, %v, %v, %v, %v)", "ctx", tt.args.userId, tt.args.currency, tt.args.value, tt.args.merchantData)) {
				return
			}
			if tt.wantID {
				assert.NotEqualf(t, 0, got, "TopUp(%v, %v, %v, %v, %v)", "ctx", tt.args.userId, tt.args.currency, tt.args.value, tt.args.merchantData)

				var gotCurrency string
				var gotBalance decimal.Decimal
				row := db.db.QueryRow(context.TODO(), `SELECT currency, current_value FROM balance WHERE user_id = $1`, tt.args.userId)
				_ = row.Scan(&gotCurrency, &gotBalance)
				assert.Equalf(t, tt.wantCurrency, gotCurrency, "currency %s/%s", tt.wantCurrency, gotCurrency)
				assert.Truef(t, tt.wantBalance.Equal(gotBalance), "balance %s/%s", tt.wantBalance.String(), gotBalance.String())

				var txRecipient, txCurrency string
				var txValue decimal.Decimal
				row = db.db.QueryRow(context.TODO(), `SELECT recipient_id, transaction_currency, transaction_value FROM "transaction" WHERE id = $1`, got.Int64())
				_ = row.Scan(&txRecipient, &txCurrency, &txValue)
				assert.Equalf(t, tt.args.userId, txRecipient, "transaction recipient")
				assert.Equalf(t, tt.args.currency, txCurrency, "transaction currency")
				argValue, _ := decimal.NewFromString(tt.args.value)
				assert.Equalf(t, argValue, txValue, "transaction value")
			}
		})
	}
}
