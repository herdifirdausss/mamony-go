package app

import (
	"errors"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/modules/ledger"
	"github.com/herdifirdausss/mamony-go/internal/platform/reference"
)

func TestLedgerReferenceComposition(t *testing.T) {
	catalog, err := reference.Load()
	if err != nil {
		t.Fatal(err)
	}
	a, err := newLedgerCurrencyAdmission()
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := catalog.LookupCurrency("USD")
	if err != nil {
		t.Fatal(err)
	}
	want := ledger.CurrencyDefinition{Code: string(canonical.Code()), Exponent: canonical.Exponent(), Version: canonical.Version(), UnitID: canonical.UnitID()}
	if got, err := a.LookupCurrency("USD"); err != nil || got != want || got.Exponent != 2 || got.Version != "mamony-currency-v2:USD:1" || got.UnitID != "mamony-unit-v1:USD:1" {
		t.Fatalf("composition did not use canonical USD: %+v, %v", got, err)
	}
	if got, err := a.RequireCurrency(want); err != nil || got != want {
		t.Fatalf("canonical definition refused: %+v, %v", got, err)
	}
	for _, code := range []reference.CurrencyCode{"JPY", "KWD", "IDR", "SGD"} {
		d, err := catalog.LookupCurrency(code)
		if err != nil {
			t.Fatalf("expected known reference %s: %v", code, err)
		}
		if got, err := a.RequireCurrency(ledger.CurrencyDefinition{Code: string(d.Code()), Exponent: d.Exponent(), Version: d.Version(), UnitID: d.UnitID()}); !errors.Is(err, ledger.ErrCurrencyNotEnabled) || got != (ledger.CurrencyDefinition{}) {
			t.Fatalf("known but disabled currency admitted: %s, %v", code, err)
		}
	}
	for _, code := range []string{"EUR", "usd", "USD ", "XXX", ""} {
		if _, err := catalog.LookupCurrency(reference.CurrencyCode(code)); !errors.Is(err, reference.ErrUnknownCurrency) {
			t.Fatalf("unexpected catalog acceptance: %q, %v", code, err)
		}
		if _, err := a.LookupCurrency(code); !errors.Is(err, ledger.ErrCurrencyNotEnabled) {
			t.Fatalf("unexpected admission: %q, %v", code, err)
		}
	}
	pinned, err := catalog.LookupDefinition(want.Version)
	if err != nil || pinned != canonical {
		t.Fatal("pinned canonical USD differs from current definition")
	}
	wrong := want
	wrong.UnitID = "mamony-unit-v1:USD:2"
	if _, err := a.RequireCurrency(wrong); !errors.Is(err, ledger.ErrCurrencyMismatch) {
		t.Fatal("same-exponent different unit accepted")
	}
	wrong = want
	wrong.Version = "iso4217-minor-v1:USD:2"
	if _, err := a.RequireCurrency(wrong); !errors.Is(err, ledger.ErrCurrencyMismatch) {
		t.Fatal("legacy identity accepted")
	}
	// The admitted identity is a definition version, not the release ID.
	want.Version = catalog.ReleaseID()
	if _, err := a.RequireCurrency(want); !errors.Is(err, ledger.ErrCurrencyMismatch) {
		t.Fatal("release substituted for definition identity")
	}
}

func TestLedgerReferenceCompositionMissingCatalog(t *testing.T) {
	a, err := admitLedgerCurrency(reference.Catalog{})
	if !errors.Is(err, reference.ErrUnknownCurrency) || a != (ledger.CurrencyAdmission{}) {
		t.Fatalf("missing canonical USD did not fail closed: %v", err)
	}
}
