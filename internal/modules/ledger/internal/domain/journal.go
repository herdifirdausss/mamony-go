package domain

import (
	"math/big"
	"slices"
)

const MaxEntries = 64

type Entry struct {
	Account   AccountID
	Direction Direction
	Amount    Money
}

// JournalDraft retains the submitted entry order and owns a copy of its slice.
type JournalDraft struct {
	book     BookID
	currency CurrencyRef
	entries  []Entry
}

func NewJournalDraft(book BookID, currency CurrencyRef, entries []Entry) (JournalDraft, error) {
	if len(entries) < 2 || len(entries) > MaxEntries {
		return JournalDraft{}, ErrInvalid
	}
	j := JournalDraft{book: book, currency: currency, entries: slices.Clone(entries)}
	if err := j.validate(); err != nil {
		return JournalDraft{}, err
	}
	return j, nil
}

func (j JournalDraft) validate() error {
	if !validIdentifier(string(j.book)) || !j.currency.valid() ||
		len(j.entries) < 2 || len(j.entries) > MaxEntries {
		return ErrInvalid
	}
	var debit, credit big.Int
	for _, e := range j.entries {
		if !validIdentifier(string(e.Account)) || e.Amount.currency != j.currency ||
			e.Amount.minor <= 0 {
			return ErrInvalid
		}
		switch e.Direction {
		case Debit:
			debit.Add(&debit, big.NewInt(e.Amount.minor))
		case Credit:
			credit.Add(&credit, big.NewInt(e.Amount.minor))
		default:
			return ErrInvalid
		}
	}
	if debit.Cmp(&credit) != 0 {
		return ErrInvalid
	}
	return nil
}

func (j JournalDraft) Entries() []Entry {
	return slices.Clone(j.entries)
}
