package proto

import "fmt"

func (m *Error) Error() string {
	if m == nil {
		return ""
	}
	switch e := m.OneError.(type) {
	case *Error_BadParameter:
		return fmt.Sprintf("bad parameter %s", e.BadParameter.Name)
	case *Error_Unauthorized:
		return "unauthorized"
	case *Error_NotEnoughMoney:
		return "not enough money"
	case *Error_UserNotFound:
		return "user not found"
	case *Error_InvalidCurrency:
		return fmt.Sprintf("invalid currency %s", e.InvalidCurrency.Currency)
	case *Error_InvalidState:
		return "order is in invalid state"
	default:
		return m.String()
	}
}

func NewBadParameterError(name string) *Error {
	return &Error{OneError: &Error_BadParameter{&BadParameterError{Name: name}}}
}

func NewUnauthorizedError() *Error {
	return &Error{OneError: &Error_Unauthorized{&UnauthorizedError{}}}
}

func NewNotEnoughMoneyError() *Error {
	return &Error{OneError: &Error_NotEnoughMoney{&NotEnoughMoneyError{}}}
}

func NewUserNotFoundError() *Error {
	return &Error{OneError: &Error_UserNotFound{&UserNotFoundError{}}}
}

func NewInvalidCurrencyError(currency string) *Error {
	return &Error{OneError: &Error_InvalidCurrency{&InvalidCurrencyError{Currency: currency}}}
}

func NewInvalidStateError() *Error {
	return &Error{OneError: &Error_InvalidState{&InvalidStateError{}}}
}
