package proto

import (
	"reflect"
	"testing"
)

func TestError_Error(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input isError_OneError
		want  string
	}{
		{"empty error", nil, ""},
		{"bad parameter", &Error_BadParameter{BadParameter: &BadParameterError{Name: "kwa"}}, "bad parameter kwa"},
		{"unauthorized", &Error_Unauthorized{Unauthorized: &UnauthorizedError{}}, "unauthorized"},
		{"not enough money", &Error_NotEnoughMoney{NotEnoughMoney: &NotEnoughMoneyError{}}, "not enough money"},
		{"user not found", &Error_UserNotFound{UserNotFound: &UserNotFoundError{}}, "user not found"},
		{"invalid currency", &Error_InvalidCurrency{InvalidCurrency: &InvalidCurrencyError{Currency: "xxx"}}, "invalid currency xxx"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := &Error{OneError: tt.input}
			if got := m.Error(); got != tt.want {
				t.Errorf("Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewBadParameterError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  *Error
	}{
		{"empty name", "", &Error{OneError: &Error_BadParameter{&BadParameterError{Name: ""}}}},
		{"with name", "currency", &Error{OneError: &Error_BadParameter{&BadParameterError{Name: "currency"}}}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NewBadParameterError(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewBadParameterError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewInvalidCurrencyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  *Error
	}{
		{"with currency", "xxx", &Error{OneError: &Error_InvalidCurrency{InvalidCurrency: &InvalidCurrencyError{Currency: "xxx"}}}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NewInvalidCurrencyError(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewInvalidCurrencyError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewNotEnoughMoneyError(t *testing.T) {
	t.Parallel()

	want := &Error{OneError: &Error_NotEnoughMoney{NotEnoughMoney: &NotEnoughMoneyError{}}}
	if got := NewNotEnoughMoneyError(); !reflect.DeepEqual(got, want) {
		t.Errorf("NewNotEnoughMoneyError() = %v, want %v", got, want)
	}
}

func TestNewUnauthorizedError(t *testing.T) {
	t.Parallel()

	want := &Error{OneError: &Error_Unauthorized{Unauthorized: &UnauthorizedError{}}}
	if got := NewUnauthorizedError(); !reflect.DeepEqual(got, want) {
		t.Errorf("NewNotEnoughMoneyError() = %v, want %v", got, want)
	}
}

func TestNewUserNotFoundError(t *testing.T) {
	t.Parallel()

	want := &Error{OneError: &Error_UserNotFound{UserNotFound: &UserNotFoundError{}}}
	if got := NewUserNotFoundError(); !reflect.DeepEqual(got, want) {
		t.Errorf("NewNotEnoughMoneyError() = %v, want %v", got, want)
	}
}
