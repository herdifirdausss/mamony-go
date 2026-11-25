// Package reference provides a curated, immutable reference catalog, not a
// capability enablement policy or an authority for payment eligibility.
package reference

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrInvalidCatalog    = errors.New("invalid reference catalog")
	ErrUnknownCurrency   = errors.New("currency not in reference catalog")
	ErrUnknownCountry    = errors.New("country not in reference catalog")
	ErrCurrencyMismatch  = errors.New("currency definition mismatch")
	ErrUnknownDefinition = errors.New("definition not in reference catalog")
)

type CurrencyCode string
type CountryCode string

// CurrencyDefinition is a value; only a successful catalog lookup establishes
// membership in this release. A zero value is not a definition.
type CurrencyDefinition struct {
	code     CurrencyCode
	exponent uint8
	version  string
	unitID   string
}

func (d CurrencyDefinition) Code() CurrencyCode { return d.code }
func (d CurrencyDefinition) Exponent() uint8    { return d.exponent }
func (d CurrencyDefinition) UnitID() string     { return d.unitID }

// Version returns the allocated immutable definition ID, not the release.
func (d CurrencyDefinition) Version() string { return d.version }

// Catalog has no mutation API. A successful Load owns its private maps;
// lookups return values, and copies of Catalog are safe for concurrent reads.
type Catalog struct {
	release    string
	currencies map[CurrencyCode]CurrencyDefinition
	countries  map[CountryCode]struct{}
}

// Load validates the compiled-in release. No filesystem or network is used.
func Load() (Catalog, error) {
	return parseCatalog(releaseID, curatedData, dataSHA256)
}

func (c Catalog) ReleaseID() string { return c.release }

func (c Catalog) LookupCurrency(code CurrencyCode) (CurrencyDefinition, error) {
	if d, ok := c.currencies[code]; ok {
		return d, nil
	}
	return CurrencyDefinition{}, ErrUnknownCurrency
}

// LookupDefinition pins an exact identity in this curated baseline only.
// It does not provide historical coverage or effective-date selection.
func (c Catalog) LookupDefinition(id string) (CurrencyDefinition, error) {
	for _, d := range c.currencies {
		if d.version == id {
			return d, nil
		}
	}
	return CurrencyDefinition{}, ErrUnknownDefinition
}

// RequireCurrency refuses metadata that does not match the selected definition.
func (c Catalog) RequireCurrency(code CurrencyCode, exponent uint8, version, unitID string) (CurrencyDefinition, error) {
	d, err := c.LookupCurrency(code)
	if err != nil {
		return CurrencyDefinition{}, err
	}
	if d.exponent != exponent || d.version != version || d.unitID != unitID {
		return CurrencyDefinition{}, ErrCurrencyMismatch
	}
	return d, nil
}

func (c Catalog) LookupCountry(code CountryCode) (CountryCode, error) {
	if _, ok := c.countries[code]; ok {
		return code, nil
	}
	return "", ErrUnknownCountry
}

func asciiCode(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for i := range len(value) {
		if value[i] < 'A' || value[i] > 'Z' {
			return false
		}
	}
	return true
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

// parseCatalog is deliberately private: callers cannot install a new dataset.
// The checksum detects drift, not a malicious change to both data and checksum.
func parseCatalog(release, data, checksum string) (Catalog, error) {
	if release == "" || fmt.Sprintf("%x", sha256.Sum256([]byte(data))) != checksum || !strings.HasSuffix(data, "\n") {
		return Catalog{}, ErrInvalidCatalog
	}
	c := Catalog{
		release:    release,
		currencies: make(map[CurrencyCode]CurrencyDefinition),
		countries:  make(map[CountryCode]struct{}),
	}
	previous := ""
	for _, line := range strings.Split(strings.TrimSuffix(data, "\n"), "\n") {
		if line <= previous {
			return Catalog{}, ErrInvalidCatalog
		}
		previous = line
		fields := strings.Split(line, ",")
		switch fields[0] {
		case "C":
			if len(fields) != 5 || !asciiCode(fields[1], 3) || len(fields[2]) != 1 || fields[2][0] < '0' || fields[2][0] > '3' ||
				!validAllocatedID(fields[3], "mamony-currency-v2:"+fields[1]+":") ||
				!validAllocatedID(fields[4], "mamony-unit-v1:"+fields[1]+":") {
				return Catalog{}, ErrInvalidCatalog
			}
			code := CurrencyCode(fields[1])
			if _, exists := c.currencies[code]; exists {
				return Catalog{}, ErrInvalidCatalog
			}
			c.currencies[code] = CurrencyDefinition{code: code, exponent: fields[2][0] - '0', version: fields[3], unitID: fields[4]}
		case "N":
			if len(fields) != 2 || !asciiCode(fields[1], 2) {
				return Catalog{}, ErrInvalidCatalog
			}
			c.countries[CountryCode(fields[1])] = struct{}{}
		default:
			return Catalog{}, ErrInvalidCatalog
		}
	}
	if len(c.currencies) == 0 || len(c.countries) == 0 {
		return Catalog{}, ErrInvalidCatalog
	}
	return c, nil
}
