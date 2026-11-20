// Package domain contains pure ledger journal invariants. It does not post or persist financial transactions.
package domain

import (
	"errors"
	"fmt"
	"math"
)

// Side describes a debit or credit entry in a journal.
type Side string

const (
	Debit  Side = "debit"
	Credit Side = "credit"
)

// Entry is a signed-side, positive amount in a single currency's minor units.
type Entry struct {
	AccountID   string
	Currency    string
	Side        Side
	AmountMinor int64
}

var ErrInvalidJournal = errors.New("invalid journal")

// ValidateJournal checks local journal-entry arithmetic only; account status, balance floor,
// idempotency, persistence, reversal, and authorization require separate DB-backed contracts.
func ValidateJournal(entries []Entry) error {
	if len(entries) < 2 {
		return fmt.Errorf("%w: at least two entries required", ErrInvalidJournal)
	}
	currency := ""
	var debitTotal, creditTotal int64
	for i, e := range entries {
		if e.AccountID == "" || e.Currency == "" || e.AmountMinor <= 0 {
			return fmt.Errorf("%w: missing account/currency or non-positive amount at index %d", ErrInvalidJournal, i)
		}
		if currency == "" {
			currency = e.Currency
		} else if e.Currency != currency {
			return fmt.Errorf("%w: mismatched currency at index %d", ErrInvalidJournal, i)
		}
		switch e.Side {
		case Debit:
			if e.AmountMinor > math.MaxInt64-debitTotal {
				return fmt.Errorf("%w: debit sum overflow", ErrInvalidJournal)
			}
			debitTotal += e.AmountMinor
		case Credit:
			if e.AmountMinor > math.MaxInt64-creditTotal {
				return fmt.Errorf("%w: credit sum overflow", ErrInvalidJournal)
			}
			creditTotal += e.AmountMinor
		default:
			return fmt.Errorf("%w: invalid side at index %d", ErrInvalidJournal, i)
		}
	}
	if debitTotal != creditTotal {
		return fmt.Errorf("%w: debits and credits differ", ErrInvalidJournal)
	}
	return nil
}
