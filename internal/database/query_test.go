package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestBalanceDatabase_FetchUserBalance(t *testing.T) {
	if os.Getenv("DB_URL") == "" {
		t.Skipf("db tests require database")
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

	// CREATE TEST DATASET

	initData := [][]any{
		// balance
		{"andy", "USD", "20.00"},
		{"danny", "USD", "5.00"},
		{"jenna", "EUR", "200.00"},
		// balance_reserve
		{"xy", "danny", "", "USD", "6.00", "6.00"},
		{"xz", "jenna", "", "TRY", "500.00", "25.95"},
	}
	_, _ = db.db.CopyFrom(context.TODO(), pgx.Identifier{"balance"}, []string{"user_id", "currency", "current_value"}, pgx.CopyFromRows(initData[0:3]))
	_, _ = db.db.CopyFrom(context.TODO(), pgx.Identifier{"balance_reserve"}, []string{"order_id", "user_id", "item_id", "currency", "value", "user_currency_value"}, pgx.CopyFromRows(initData[3:5]))
	t.Cleanup(func() {
		_, _ = db.db.Exec(context.TODO(), `DELETE FROM balance_reserve WHERE user_id IN($1, $2)`, "danny", "jenna")
		_, _ = db.db.Exec(context.TODO(), `DELETE FROM balance WHERE user_id IN ($1, $2, $3)`, "andy", "danny", "jenna")
	})

	errUserNotFoundError := assert.ErrorAssertionFunc(func(t assert.TestingT, err error, i ...interface{}) bool {
		return assert.ErrorContainsf(t, err, "user not found", "not a user error %v", err)
	})

	type args struct {
		userID string
	}
	tests := []struct {
		name          string
		args          args
		wantCurrency  string
		wantAvailable decimal.Decimal
		wantReserved  decimal.Decimal
		wantErr       assert.ErrorAssertionFunc
	}{
		{"nominal balance", args{"andy"}, "USD", decimal.NewFromInt(20), decimal.Zero, assert.NoError},
		{"overdraft", args{"danny"}, "USD", decimal.NewFromInt(-1), decimal.NewFromInt(6), assert.NoError},
		{"reserve", args{"jenna"}, "EUR", decimal.NewFromFloat(174.05), decimal.NewFromFloat(25.95), assert.NoError},
		{"default", args{"manuela"}, "", decimal.Zero, decimal.Zero, errUserNotFoundError},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotCurrency, gotAvailable, gotReserved, err := db.FetchUserBalance(context.TODO(), tt.args.userID)
			if !tt.wantErr(t, err, fmt.Sprintf("FetchUserBalance(%v, %v)", "ctx", tt.args.userID)) {
				return
			}
			assert.Equalf(t, tt.wantCurrency, gotCurrency, "FetchUserBalance(%v, %v)", "ctx", tt.args.userID)
			assert.Equalf(t, tt.wantAvailable.StringFixed(2), gotAvailable.StringFixed(2), "FetchUserBalance(%v, %v)", "ctx", tt.args.userID)
			assert.Equalf(t, tt.wantReserved.StringFixed(2), gotReserved.StringFixed(2), "FetchUserBalance(%v, %v)", "ctx", tt.args.userID)
		})
	}
}
