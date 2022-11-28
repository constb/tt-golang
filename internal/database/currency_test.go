package database

import (
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestConvertCurrency(t *testing.T) {
	loadRatesFromStub()
	t.Parallel()

	type args struct {
		value decimal.Decimal
		from  string
		to    string
	}
	tests := []struct {
		name    string
		args    args
		want    decimal.Decimal
		wantErr assert.ErrorAssertionFunc
	}{
		{"zero value", args{decimal.Zero, "EUR", "USD"}, decimal.Zero, assert.NoError},
		{"conversion", args{decimal.NewFromInt(500), "TRY", "USD"}, decimal.NewFromFloat(26.85), assert.NoError},

		{"bad from", args{decimal.Zero, "xxx", "USD"}, decimal.Zero, assert.Error},
		{"bad to", args{decimal.Zero, "EUR", "xxx"}, decimal.Zero, assert.Error},
		{"same currency", args{decimal.Zero, "EUR", "EUR"}, decimal.Zero, assert.Error},
		{"negative value", args{decimal.NewFromInt(-1), "EUR", "USD"}, decimal.Zero, assert.Error},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ConvertCurrency(tt.args.value, tt.args.from, tt.args.to)
			if !tt.wantErr(t, err, fmt.Sprintf("ConvertCurrency(%v, %v, %v)", tt.args.value, tt.args.from, tt.args.to)) {
				return
			}
			assert.Equalf(t, tt.want.StringFixed(2), got.StringFixed(2), "ConvertCurrency(%v, %v, %v)", tt.args.value, tt.args.from, tt.args.to)
		})
	}
}

func TestIsCurrencyValid(t *testing.T) {
	t.Parallel()
	type args struct {
		currency string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"dollar", args{"USD"}, true},
		{"euro", args{"EUR"}, true},
		{"ruble", args{"RUB"}, true},
		{"bad", args{"xxx"}, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equalf(t, tt.want, IsCurrencyValid(tt.args.currency), "IsCurrencyValid(%v)", tt.args.currency)
		})
	}
}
