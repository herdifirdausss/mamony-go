// Package ledger exposes the unmounted synthetic Ledger currency admission boundary.
package ledger

import (
	"errors"
	"strconv"
	"strings"

	"github.com/herdifirdausss/mamony-go/internal/modules/ledger/internal/domain"
)

// InitialCurrencyCode is Ledger's synthetic enablement policy, not a catalog entry.
const InitialCurrencyCode = "USD"

var (
	ErrInvalidCurrencyConfiguration = errors.New("invalid ledger currency configuration")
	ErrCurrencyNotEnabled           = errors.New("currency not enabled for ledger")
	ErrCurrencyMismatch             = errors.New("ledger currency definition mismatch")
)

// CurrencyDefinition carries values from trusted application composition.
// It is not proof of source authenticity or financial authorization.
type CurrencyDefinition struct {
	Code     string
	Exponent uint8
	Version  string
	UnitID   string
}

// CurrencyAdmission owns one immutable enabled definition and its private domain value.
// It has no posting operations, mutable registry, or runtime activation behavior.
type CurrencyAdmission struct {
	definition CurrencyDefinition
	currency   domain.CurrencyRef
}

// validAllocatedID validates local identity syntax, not source authority.
func validAllocatedID(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	revision := strings.TrimPrefix(value, prefix)
	n, err := strconv.ParseUint(revision, 10, 64)
	return err == nil && n != 0 && strconv.FormatUint(n, 10) == revision
}

// NewCurrencyAdmission accepts exactly the initial synthetic USD definition.
// The composition root is responsible for obtaining authoritative metadata.
func NewCurrencyAdmission(definitions []CurrencyDefinition) (CurrencyAdmission, error) {
	if len(definitions) != 1 {
		return CurrencyAdmission{}, ErrInvalidCurrencyConfiguration
	}
	d := definitions[0]
	if d.Code != InitialCurrencyCode {
		return CurrencyAdmission{}, ErrInvalidCurrencyConfiguration
	}
	currency, err := domain.NewCurrencyRef(d.Code, d.Version, d.Exponent)
	if err != nil || !validAllocatedID(d.Version, "mamony-currency-v2:"+d.Code+":") ||
		!validAllocatedID(d.UnitID, "mamony-unit-v1:"+d.Code+":") {
		return CurrencyAdmission{}, ErrInvalidCurrencyConfiguration
	}
	return CurrencyAdmission{definition: d, currency: currency}, nil
}

// LookupCurrency returns an admitted definition by value; zero admission fails closed.
func (a CurrencyAdmission) LookupCurrency(code string) (CurrencyDefinition, error) {
	if a.definition.Code == "" || code != a.definition.Code {
		return CurrencyDefinition{}, ErrCurrencyNotEnabled
	}
	return a.definition, nil
}

// RequireCurrency compares supplied metadata with the already admitted definition.
// It never installs or substitutes a caller-supplied definition.
func (a CurrencyAdmission) RequireCurrency(candidate CurrencyDefinition) (CurrencyDefinition, error) {
	d, err := a.LookupCurrency(candidate.Code)
	if err != nil {
		return CurrencyDefinition{}, err
	}
	currency, err := domain.NewCurrencyRef(candidate.Code, candidate.Version, candidate.Exponent)
	if err != nil || currency != a.currency || candidate != d {
		return CurrencyDefinition{}, ErrCurrencyMismatch
	}
	return d, nil
}
