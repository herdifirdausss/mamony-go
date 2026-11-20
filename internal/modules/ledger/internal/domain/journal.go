// Package domain holds pure ledger shape rules, not transaction posting or persistence.
package domain

import (
	"errors"
	"fmt"
	"math"
)

type Side string
const ( Debit Side = "debit"; Credit Side = "credit" )

type Entry struct {
	AccountID string
	Currency string
	Side Side
	AmountMinor int64
}

var ErrInvalidJournal = errors.New("invalid journal")

// ValidateJournal checks only positive minor units, single currency and debit/credit equality.
func ValidateJournal(entries []Entry) error {
	if len(entries)<2 {return fmt.Errorf("%w: need at least two entries",ErrInvalidJournal)}
	currency := ""
	var debitTotal,creditTotal int64
	for i,e := range entries {
		if e.AccountID=="" || e.Currency=="" || e.AmountMinor<=0 {return fmt.Errorf("%w: invalid entry %d",ErrInvalidJournal,i)}
		if currency=="" {currency=e.Currency} else if currency!=e.Currency {return fmt.Errorf("%w: mixed currencies",ErrInvalidJournal)}
		switch e.Side {
		case Debit:
			if e.AmountMinor > math.MaxInt64-debitTotal {return fmt.Errorf("%w: debit overflow",ErrInvalidJournal)}
			debitTotal+=e.AmountMinor
		case Credit:
			if e.AmountMinor > math.MaxInt64-creditTotal {return fmt.Errorf("%w: credit overflow",ErrInvalidJournal)}
			creditTotal+=e.AmountMinor
		default: return fmt.Errorf("%w: unknown side at %d",ErrInvalidJournal,i)
		}
	}
	if debitTotal != creditTotal {return fmt.Errorf("%w: unbalanced",ErrInvalidJournal)}
	return nil
}
