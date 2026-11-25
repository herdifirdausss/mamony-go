package reference

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func loadFixture(t *testing.T, release, data string) Catalog {
	t.Helper()
	c, err := parseCatalog(release, data, fmt.Sprintf("%x", sha256.Sum256([]byte(data))))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCuratedRelease(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.ReleaseID() != "fnd02d-2026-11-25.1" || len(c.currencies) != 5 || len(c.countries) != 5 {
		t.Fatal("unexpected release or coverage")
	}
	for _, want := range goldenDefinitions() {
		code := want.code
		d, err := c.LookupCurrency(code)
		if err != nil || d != want {
			t.Fatalf("wrong definition for %s: %v", code, err)
		}
		if got, err := c.RequireCurrency(code, want.exponent, want.version, want.unitID); err != nil || got != d {
			t.Fatalf("matching definition refused: %v", err)
		}
		if got, err := c.LookupDefinition(want.version); err != nil || got != want {
			t.Fatalf("pinned definition mismatch: %+v %v", got, err)
		}
	}
	for _, code := range []CountryCode{"ID", "JP", "KW", "SG", "US"} {
		if got, err := c.LookupCountry(code); err != nil || got != code {
			t.Fatalf("wrong country: %s, %v", got, err)
		}
	}
}

func TestLookupFailsClosed(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []CurrencyCode{"", "usd", "USD ", " USD", "US", "U1D", "EUR", "XXX", "XAU", "CLF", "US\x00", "ＵＳＤ"} {
		if got, err := c.LookupCurrency(code); !errors.Is(err, ErrUnknownCurrency) || got != (CurrencyDefinition{}) {
			t.Fatalf("unexpected currency result: %q, %v", code, err)
		}
	}
	for _, code := range []CountryCode{"", "us", "US ", "USA", "U1", "GB", "ZZ", "U\x00"} {
		if got, err := c.LookupCountry(code); !errors.Is(err, ErrUnknownCountry) || got != "" {
			t.Fatalf("unexpected country result: %q, %v", code, err)
		}
	}
	for _, tc := range []struct {
		code     CurrencyCode
		exponent uint8
		version  string
		unitID   string
		want     error
	}{
		{"USD", 0, "mamony-currency-v2:USD:1", "mamony-unit-v1:USD:1", ErrCurrencyMismatch},
		{"USD", 2, "", "mamony-unit-v1:USD:1", ErrCurrencyMismatch},
		{"USD", 2, "mamony-currency-v2:JPY:1", "mamony-unit-v1:USD:1", ErrCurrencyMismatch},
		{"USD", 2, "iso4217-minor-v1:USD:2", "mamony-unit-v1:USD:1", ErrCurrencyMismatch},
		{"USD", 2, releaseID, "mamony-unit-v1:USD:1", ErrCurrencyMismatch},
		{"USD", 2, "mamony-currency-v2:USD:1", "", ErrCurrencyMismatch},
		{"USD", 2, "mamony-currency-v2:USD:1", "mamony-unit-v1:USD:2", ErrCurrencyMismatch},
		{"USD", 2, "mamony-currency-v2:USD:2", "mamony-unit-v1:USD:1", ErrCurrencyMismatch},
		{"EUR", 2, "mamony-currency-v2:EUR:1", "mamony-unit-v1:EUR:1", ErrUnknownCurrency},
	} {
		if got, err := c.RequireCurrency(tc.code, tc.exponent, tc.version, tc.unitID); !errors.Is(err, tc.want) || got != (CurrencyDefinition{}) {
			t.Fatalf("unexpected match result: %+v, %v", tc, err)
		}
	}
	for _, id := range []string{"", "iso4217-minor-v1:USD:2", releaseID, "mamony-currency-v2:USD:2", "mamony-currency-v2:usd:1", "mamony-unit-v1:USD:1", "mamony-currency-v2:USD:01"} {
		if got, err := c.LookupDefinition(id); !errors.Is(err, ErrUnknownDefinition) || got != (CurrencyDefinition{}) {
			t.Fatalf("unexpected pinned lookup %q: %+v %v", id, got, err)
		}
	}
	var zero Catalog
	if d, err := zero.LookupDefinition("mamony-currency-v2:USD:1"); !errors.Is(err, ErrUnknownDefinition) || d != (CurrencyDefinition{}) {
		t.Fatal("zero catalog accepted pinned lookup")
	}
	if d, err := zero.LookupCurrency("JPY"); !errors.Is(err, ErrUnknownCurrency) || d.Version() != "" {
		t.Fatal("zero catalog or definition accepted")
	}
	if _, err := zero.LookupCountry("JP"); !errors.Is(err, ErrUnknownCountry) {
		t.Fatal("zero catalog accepted country")
	}
}

func TestInvalidCatalogReturnsNoPartialState(t *testing.T) {
	for _, data := range []string{
		"C,USD,2\nN,US\n", // legacy row without IDs
		"C,USD,2,mamony-currency-v2:USD:1,\nN,US\n",
		"C,USD,2,,mamony-unit-v1:USD:1\nN,US\n",
		"C,USD,2,iso4217-minor-v1:USD:2,mamony-unit-v1:USD:1\nN,US\n",
		"C,USD,2,mamony-currency-v2:JPY:1,mamony-unit-v1:USD:1\nN,US\n",
		"C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:JPY:1\nN,US\n",
		"C,USD,2,mamony-currency-v2:USD:01,mamony-unit-v1:USD:1\nN,US\n",
		"C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:0\nN,US\n",
		"C,USD,2,mamony-currency-v2:USD:18446744073709551616,mamony-unit-v1:USD:1\nN,US\n",
		"", "C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\n", "N,US\n", "C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,US", "C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,US\n\n",
		"C,USD,,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,US\n", "C,USD,N.A.,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,US\n", "C,USD,4,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,US\n", "C,USD,-1,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,US\n",
		"C,USD,02,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,US\n", "C,USD,2,extra\nN,US\n", "C,usd,2,mamony-currency-v2:usd:1,mamony-unit-v1:usd:1\nN,US\n", "C,US,2,mamony-currency-v2:US:1,mamony-unit-v1:US:1\nN,US\n",
		"C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,us\n", "C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,USA\n", "C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,US,\n", "C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nX,US\n",
		"C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nC,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,US\n", "C,USD,1,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nC,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,US\n", "C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,US\nN,US\n",
		"N,US\nC,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\n", "C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\r\nN,US\r\n",
	} {
		c, err := parseCatalog("test", data, fmt.Sprintf("%x", sha256.Sum256([]byte(data))))
		if !errors.Is(err, ErrInvalidCatalog) || c.release != "" || c.currencies != nil || c.countries != nil {
			t.Fatalf("invalid data yielded partial catalog: %q, %v", data, err)
		}
	}
	for _, tc := range []struct{ release, checksum string }{{"", dataSHA256}, {releaseID, "wrong"}} {
		if c, err := parseCatalog(tc.release, curatedData, tc.checksum); !errors.Is(err, ErrInvalidCatalog) || c.release != "" || c.currencies != nil || c.countries != nil {
			t.Fatal("invalid manifest accepted")
		}
	}
	if _, err := parseCatalog(releaseID, curatedData+"N,ZZ\n", dataSHA256); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatal("changed bytes accepted with old checksum")
	}
}

// goldenDefinitions is independent test evidence, never a runtime data source.
func goldenDefinitions() []CurrencyDefinition {
	return []CurrencyDefinition{
		{"IDR", 2, "mamony-currency-v2:IDR:1", "mamony-unit-v1:IDR:1"},
		{"JPY", 0, "mamony-currency-v2:JPY:1", "mamony-unit-v1:JPY:1"},
		{"KWD", 3, "mamony-currency-v2:KWD:1", "mamony-unit-v1:KWD:1"},
		{"SGD", 2, "mamony-currency-v2:SGD:1", "mamony-unit-v1:SGD:1"},
		{"USD", 2, "mamony-currency-v2:USD:1", "mamony-unit-v1:USD:1"},
	}
}

// baselineCompatible checks reviewed, validated snapshots in tests only.
// This initial freeze does not allow changes to already admitted tuples.
func baselineCompatible(previous, next Catalog) bool {
	if previous.release == "" || next.release == "" {
		return false
	}
	for code, want := range previous.currencies {
		got, err := next.LookupCurrency(code)
		pinned, pinErr := next.LookupDefinition(want.version)
		if err != nil || pinErr != nil || got != want || pinned != want {
			return false
		}
	}
	return true
}

func TestDefinitionIdentityIndependentOfRelease(t *testing.T) {
	first := loadFixture(t, "test-v1", "C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,US\n")
	second := loadFixture(t, "test-v2", "C,JPY,0,mamony-currency-v2:JPY:1,mamony-unit-v1:JPY:1\nC,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,JP\nN,US\n")
	a, err := first.LookupCurrency("USD")
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.LookupCurrency("USD")
	if err != nil || a != b || first.ReleaseID() == second.ReleaseID() || !baselineCompatible(first, second) {
		t.Fatal("unchanged definition not retained across releases")
	}
	for _, data := range []string{
		"C,USD,3,mamony-currency-v2:USD:1,mamony-unit-v1:USD:1\nN,US\n",
		"C,USD,2,mamony-currency-v2:USD:1,mamony-unit-v1:USD:2\nN,US\n",
		"C,USD,2,mamony-currency-v2:USD:2,mamony-unit-v1:USD:2\nN,US\n",
		"C,USD,2,mamony-currency-v2:USD:2,mamony-unit-v1:USD:1\nN,US\n",
		"C,JPY,0,mamony-currency-v2:JPY:1,mamony-unit-v1:JPY:1\nN,JP\n",
	} {
		next := loadFixture(t, "test-v3", data)
		if baselineCompatible(first, next) {
			t.Fatalf("baseline mutation accepted: %q", data)
		}
		if _, err := next.RequireCurrency(a.code, a.exponent, a.version, a.unitID); err == nil {
			t.Fatalf("old tuple accepted by changed catalog: %q", data)
		}
	}
	if baselineCompatible(Catalog{}, second) || baselineCompatible(first, Catalog{}) {
		t.Fatal("unvalidated baseline accepted")
	}
}

func TestCuratedBaselineRetentionAndProvenance(t *testing.T) {
	current, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	golden := Catalog{release: "golden", currencies: make(map[CurrencyCode]CurrencyDefinition)}
	var old strings.Builder
	for _, d := range goldenDefinitions() {
		golden.currencies[d.code] = d
		fmt.Fprintf(&old, "C,%s,%d\n", d.code, d.exponent)
	}
	for _, code := range []string{"ID", "JP", "KW", "SG", "US"} {
		fmt.Fprintf(&old, "N,%s\n", code)
	}
	if !baselineCompatible(golden, current) {
		t.Fatal("reviewed baseline tuples changed")
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(old.String()))) != "7cd0ffb8aa27b279931d861ca6dea52256272c512a47b5e0013ca1049e306e3c" {
		t.Fatal("original curated provenance drift")
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(curatedData))) != "9c335b6f3362735bde0523de78153e8525c56d0d8ad04e4133155ca6c01cbcf1" {
		t.Fatal("FND-02d reviewed bytes drift")
	}
}

func TestConcurrentReadAndValueIsolation(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	d, err := c.LookupCurrency("USD")
	if err != nil {
		t.Fatal(err)
	}
	d.exponent = 3
	d.unitID = "changed"
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				got, err := c.LookupCurrency("USD")
				pinned, pinErr := c.LookupDefinition("mamony-currency-v2:USD:1")
				if pinErr != nil || pinned != got || err != nil || got.Exponent() != 2 || got.UnitID() != "mamony-unit-v1:USD:1" || got == d {
					t.Error("lookup changed after value copy or concurrent reads")
					return
				}
			}
		}()
	}
	wg.Wait()
}
