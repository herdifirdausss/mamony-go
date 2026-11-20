package domain

import (
	"errors"
	"math"
	"testing"
)

func TestValidateJournal(t *testing.T) {
	t.Parallel()
	valid := []Entry{
		{AccountID: "a", Currency: "IDR", Side: Debit, AmountMinor: 100},
		{AccountID: "b", Currency: "IDR", Side: Credit, AmountMinor: 100},
	}
	tests := []struct {
		name    string
		entries []Entry
		ok      bool
	}{
		{"balanced", valid, true},
		{"missing entries", nil, false},
		{"unbalanced", []Entry{{"a", "IDR", Debit, 100}, {"b", "IDR", Credit, 99}}, false},
		{"wrong currency", []Entry{{"a", "USD", Debit, 100}, {"b", "IDR", Credit, 100}}, false},
		{"negative", []Entry{{"a", "IDR", Debit, -10}, {"b", "IDR", Credit, -10}}, false},
		{"zero", []Entry{{"a", "IDR", Debit, 0}, {"b", "IDR", Credit, 0}}, false},
		{"invalid side", []Entry{{"a", "IDR", "other", 100}, {"b", "IDR", Credit, 100}}, false},
		{"overflow", []Entry{{"a", "IDR", Debit, math.MaxInt64}, {"a", "IDR", Debit, 1}, {"b", "IDR", Credit, 1}}, false},
		{"multiple entries", []Entry{{"a", "IDR", Debit, 40}, {"a", "IDR", Debit, 60}, {"b", "IDR", Credit, 100}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateJournal(tc.entries)
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalidJournal) {
				t.Fatalf("want ErrInvalidJournal; got %v", err)
			}
		})
	}
}
