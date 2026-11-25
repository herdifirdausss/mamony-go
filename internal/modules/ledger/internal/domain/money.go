package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var ErrInvalid = errors.New("invalid ledger input")

const maxIdentifierBytes = 128

func validIdentifier(value string) bool {
	return len(value) > 0 && len(value) <= maxIdentifierBytes &&
		utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

// CurrencyRef is a structural value, not evidence of reference-data authority.
type CurrencyRef struct {
	code, version string
	exponent      uint8
}

func NewCurrencyRef(code, version string, exponent uint8) (CurrencyRef, error) {
	c := CurrencyRef{code: code, version: version, exponent: exponent}
	if !c.valid() {
		return CurrencyRef{}, ErrInvalid
	}
	return c, nil
}

func (c CurrencyRef) valid() bool {
	if len(c.code) != 3 || !validIdentifier(c.version) || c.exponent > 3 {
		return false
	}
	for i := range len(c.code) {
		if c.code[i] < 'A' || c.code[i] > 'Z' {
			return false
		}
	}
	return true
}

// Money represents integer minor units, including signed values for calculations.
// Entry validation separately requires a positive amount.
type Money struct {
	currency CurrencyRef
	minor    int64
}

func NewMoney(currency CurrencyRef, minor int64) (Money, error) {
	if !currency.valid() {
		return Money{}, ErrInvalid
	}
	return Money{currency: currency, minor: minor}, nil
}
