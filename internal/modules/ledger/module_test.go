package ledger

import (
	"errors"
	"sync"
	"testing"
)

func usdDefinition() CurrencyDefinition {
	return CurrencyDefinition{Code: "USD", Exponent: 2, Version: "iso4217-minor-v1:USD:2"}
}

func TestCurrencyAdmissionConfiguration(t *testing.T) {
	usd := usdDefinition()
	for _, tc := range []struct {
		name        string
		definitions []CurrencyDefinition
	}{
		{"missing", nil},
		{"zero", []CurrencyDefinition{{}}},
		{"duplicate", []CurrencyDefinition{usd, usd}},
		{"conflict", []CurrencyDefinition{usd, {Code: "USD", Exponent: 3, Version: "iso4217-minor-v1:USD:3"}}},
		{"extra", []CurrencyDefinition{usd, {Code: "JPY", Version: "iso4217-minor-v1:JPY:0"}}},
		{"disabled", []CurrencyDefinition{{Code: "JPY", Version: "iso4217-minor-v1:JPY:0"}}},
		{"lowercase", []CurrencyDefinition{{Code: "usd", Exponent: 2, Version: "iso4217-minor-v1:usd:2"}}},
		{"invalid exponent", []CurrencyDefinition{{Code: "USD", Exponent: 4, Version: "iso4217-minor-v1:USD:4"}}},
		{"missing version", []CurrencyDefinition{{Code: "USD", Exponent: 2}}},
		{"mismatch", []CurrencyDefinition{{Code: "USD", Exponent: 3, Version: usd.Version}}},
		{"release not definition", []CurrencyDefinition{{Code: "USD", Exponent: 2, Version: "fnd02a-2026-11-25.1"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, err := NewCurrencyAdmission(tc.definitions)
			if !errors.Is(err, ErrInvalidCurrencyConfiguration) || a != (CurrencyAdmission{}) {
				t.Fatalf("expected zero admission and configuration error: %v", err)
			}
		})
	}
}

func TestCurrencyAdmissionLookupAndMatch(t *testing.T) {
	usd := usdDefinition()
	a, err := NewCurrencyAdmission([]CurrencyDefinition{usd})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := a.LookupCurrency("USD"); err != nil || got != usd {
		t.Fatalf("USD lookup: %+v, %v", got, err)
	}
	if got, err := a.RequireCurrency(usd); err != nil || got != usd {
		t.Fatalf("USD exact match: %+v, %v", got, err)
	}
	for _, code := range []string{"JPY", "KWD", "IDR", "SGD", "EUR", "XXX", "", "usd", "USD ", " USD", "US", "U1D", "US\x00", "ＵＳＤ"} {
		if got, err := a.LookupCurrency(code); !errors.Is(err, ErrCurrencyNotEnabled) || got != (CurrencyDefinition{}) {
			t.Fatalf("unexpected lookup %q: %+v, %v", code, got, err)
		}
	}
	for _, tc := range []struct {
		candidate CurrencyDefinition
		want      error
	}{
		{CurrencyDefinition{}, ErrCurrencyNotEnabled},
		{CurrencyDefinition{Code: "JPY", Version: "iso4217-minor-v1:JPY:0"}, ErrCurrencyNotEnabled},
		{CurrencyDefinition{Code: "USD", Exponent: 0, Version: usd.Version}, ErrCurrencyMismatch},
		{CurrencyDefinition{Code: "USD", Exponent: 3, Version: "iso4217-minor-v1:USD:3"}, ErrCurrencyMismatch},
		{CurrencyDefinition{Code: "USD", Exponent: 2, Version: "iso4217-minor-v2:USD:2"}, ErrCurrencyMismatch},
		{CurrencyDefinition{Code: "USD", Exponent: 2}, ErrCurrencyMismatch},
	} {
		if got, err := a.RequireCurrency(tc.candidate); !errors.Is(err, tc.want) || got != (CurrencyDefinition{}) {
			t.Fatalf("unexpected match: %+v, %v", got, err)
		}
	}
	var zero CurrencyAdmission
	if got, err := zero.LookupCurrency("USD"); !errors.Is(err, ErrCurrencyNotEnabled) || got != (CurrencyDefinition{}) {
		t.Fatal("zero admission accepted USD")
	}
	if _, err := zero.RequireCurrency(usd); !errors.Is(err, ErrCurrencyNotEnabled) {
		t.Fatal("zero admission accepted definition")
	}
}

func TestCurrencyAdmissionImmutableConcurrentReads(t *testing.T) {
	usd := usdDefinition()
	input := []CurrencyDefinition{usd}
	a, err := NewCurrencyAdmission(input)
	if err != nil {
		t.Fatal(err)
	}
	input[0].Exponent = 3
	returned, err := a.LookupCurrency("USD")
	if err != nil {
		t.Fatal(err)
	}
	returned.Version = "changed"
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				got, err := a.RequireCurrency(usd)
				if err != nil || got != usd || got == returned {
					t.Error("admission changed after value mutation or concurrent read")
					return
				}
			}
		}()
	}
	wg.Wait()
}
