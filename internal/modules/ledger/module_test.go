package ledger

import (
	"errors"
	"sync"
	"testing"
)

func usdDefinition() CurrencyDefinition {
	return CurrencyDefinition{Code: "USD", Exponent: 2, Version: "mamony-currency-v2:USD:1", UnitID: "mamony-unit-v1:USD:1"}
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
		{"conflict", []CurrencyDefinition{usd, {Code: "USD", Exponent: 3, Version: "mamony-currency-v2:USD:2", UnitID: "mamony-unit-v1:USD:1"}}},
		{"extra", []CurrencyDefinition{usd, {Code: "JPY", Version: "mamony-currency-v2:JPY:1", UnitID: "mamony-unit-v1:JPY:1"}}},
		{"disabled", []CurrencyDefinition{{Code: "JPY", Version: "mamony-currency-v2:JPY:1", UnitID: "mamony-unit-v1:JPY:1"}}},
		{"lowercase", []CurrencyDefinition{{Code: "usd", Exponent: 2, Version: "mamony-currency-v2:usd:1", UnitID: "mamony-unit-v1:usd:1"}}},
		{"invalid exponent", []CurrencyDefinition{{Code: "USD", Exponent: 4, Version: "mamony-currency-v2:USD:1", UnitID: "mamony-unit-v1:USD:1"}}},
		{"missing version", []CurrencyDefinition{{Code: "USD", Exponent: 2}}},
		{"definition code mismatch", []CurrencyDefinition{{Code: "USD", Exponent: 2, Version: "mamony-currency-v2:JPY:1", UnitID: usd.UnitID}}},
		{"unit code mismatch", []CurrencyDefinition{{Code: "USD", Exponent: 2, Version: usd.Version, UnitID: "mamony-unit-v1:JPY:1"}}},
		{"missing unit", []CurrencyDefinition{{Code: "USD", Exponent: 2, Version: usd.Version}}},
		{"legacy v1", []CurrencyDefinition{{Code: "USD", Exponent: 2, Version: "iso4217-minor-v1:USD:2", UnitID: usd.UnitID}}},
		{"malformed definition", []CurrencyDefinition{{Code: "USD", Exponent: 2, Version: "mamony-currency-v2:USD:01", UnitID: usd.UnitID}}},
		{"malformed unit", []CurrencyDefinition{{Code: "USD", Exponent: 2, Version: usd.Version, UnitID: "mamony-unit-v1:USD:0"}}},
		{"release not definition", []CurrencyDefinition{{Code: "USD", Exponent: 2, Version: "fnd02d-2026-11-25.1", UnitID: usd.UnitID}}},
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
		{CurrencyDefinition{Code: "JPY", Version: "mamony-currency-v2:JPY:1", UnitID: "mamony-unit-v1:JPY:1"}, ErrCurrencyNotEnabled},
		{CurrencyDefinition{Code: "USD", Exponent: 0, Version: usd.Version, UnitID: usd.UnitID}, ErrCurrencyMismatch},
		{CurrencyDefinition{Code: "USD", Exponent: 3, Version: "mamony-currency-v2:USD:2", UnitID: "mamony-unit-v1:USD:1"}, ErrCurrencyMismatch},
		{CurrencyDefinition{Code: "USD", Exponent: 2, Version: "iso4217-minor-v1:USD:2", UnitID: usd.UnitID}, ErrCurrencyMismatch},
		{CurrencyDefinition{Code: "USD", Exponent: 2, Version: "iso4217-minor-v2:USD:2", UnitID: usd.UnitID}, ErrCurrencyMismatch},
		{CurrencyDefinition{Code: "USD", Exponent: 3, Version: usd.Version, UnitID: usd.UnitID}, ErrCurrencyMismatch},
		{CurrencyDefinition{Code: "USD", Exponent: 2, Version: usd.Version, UnitID: "mamony-unit-v1:USD:2"}, ErrCurrencyMismatch},
		{CurrencyDefinition{Code: "USD", Exponent: 2, Version: "mamony-currency-v2:USD:2", UnitID: usd.UnitID}, ErrCurrencyMismatch},
		{CurrencyDefinition{Code: "USD", Exponent: 2, Version: usd.Version}, ErrCurrencyMismatch},
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
	input[0].UnitID = "changed"
	returned, err := a.LookupCurrency("USD")
	if err != nil {
		t.Fatal(err)
	}
	returned.Version = "changed"
	returned.UnitID = "changed"
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

// TestCurrencyAdmissionTrustBoundary documents that structural validation is
// not proof of catalog membership; only the composition root supplies authority.
func TestCurrencyAdmissionTrustBoundary(t *testing.T) {
	d := usdDefinition()
	d.Exponent = 3
	a, err := NewCurrencyAdmission([]CurrencyDefinition{d})
	if err != nil {
		t.Fatal("shape-valid metadata should not be interpreted from identity", err)
	}
	if _, err := a.RequireCurrency(usdDefinition()); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatal("mismatched tuple accepted")
	}
}
