package app

import (
	"fmt"

	"github.com/herdifirdausss/mamony-go/internal/modules/ledger"
	"github.com/herdifirdausss/mamony-go/internal/platform/reference"
)

// newLedgerCurrencyAdmission is an unmounted synthetic composition helper.
// It is intentionally not called by Run, any command, or a worker.
func newLedgerCurrencyAdmission() (ledger.CurrencyAdmission, error) {
	catalog, err := reference.Load()
	if err != nil {
		return ledger.CurrencyAdmission{}, fmt.Errorf("load currency reference: %w", err)
	}
	return admitLedgerCurrency(catalog)
}

func admitLedgerCurrency(catalog reference.Catalog) (ledger.CurrencyAdmission, error) {
	d, err := catalog.LookupCurrency(reference.CurrencyCode(ledger.InitialCurrencyCode))
	if err != nil {
		return ledger.CurrencyAdmission{}, fmt.Errorf("resolve ledger currency: %w", err)
	}
	return ledger.NewCurrencyAdmission([]ledger.CurrencyDefinition{{
		Code:     string(d.Code()),
		Exponent: d.Exponent(),
		Version:  d.Version(),
		UnitID:   d.UnitID(),
	}})
}
