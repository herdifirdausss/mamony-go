package reference

import (
	"crypto/sha256"
	"errors"
	"fmt"
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
	if c.ReleaseID() != "fnd02a-2026-11-25.1" || len(c.currencies) != 5 || len(c.countries) != 5 {
		t.Fatal("unexpected release or coverage")
	}
	for code, exponent := range map[CurrencyCode]uint8{"IDR": 2, "JPY": 0, "KWD": 3, "SGD": 2, "USD": 2} {
		d, err := c.LookupCurrency(code)
		if err != nil || d.Code() != code || d.Exponent() != exponent || d.Version() != fmt.Sprintf("iso4217-minor-v1:%s:%d", code, exponent) {
			t.Fatalf("wrong definition for %s: %v", code, err)
		}
		if got, err := c.RequireCurrency(code, exponent, d.Version()); err != nil || got != d {
			t.Fatalf("matching definition refused: %v", err)
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
		want     error
	}{
		{"USD", 0, "iso4217-minor-v1:USD:2", ErrCurrencyMismatch},
		{"USD", 2, "", ErrCurrencyMismatch},
		{"USD", 2, "iso4217-minor-v1:JPY:0", ErrCurrencyMismatch},
		{"EUR", 2, "iso4217-minor-v1:EUR:2", ErrUnknownCurrency},
	} {
		if got, err := c.RequireCurrency(tc.code, tc.exponent, tc.version); !errors.Is(err, tc.want) || got != (CurrencyDefinition{}) {
			t.Fatalf("unexpected match result: %+v, %v", tc, err)
		}
	}
	var zero Catalog
	if d, err := zero.LookupCurrency("JPY"); !errors.Is(err, ErrUnknownCurrency) || d.Version() != "" {
		t.Fatal("zero catalog or definition accepted")
	}
	if _, err := zero.LookupCountry("JP"); !errors.Is(err, ErrUnknownCountry) {
		t.Fatal("zero catalog accepted country")
	}
}

func TestInvalidCatalogReturnsNoPartialState(t *testing.T) {
	for _, data := range []string{
		"", "C,USD,2\n", "N,US\n", "C,USD,2\nN,US", "C,USD,2\nN,US\n\n",
		"C,USD,\nN,US\n", "C,USD,N.A.\nN,US\n", "C,USD,4\nN,US\n", "C,USD,-1\nN,US\n",
		"C,USD,02\nN,US\n", "C,USD,2,extra\nN,US\n", "C,usd,2\nN,US\n", "C,US,2\nN,US\n",
		"C,USD,2\nN,us\n", "C,USD,2\nN,USA\n", "C,USD,2\nN,US,\n", "C,USD,2\nX,US\n",
		"C,USD,2\nC,USD,2\nN,US\n", "C,USD,1\nC,USD,2\nN,US\n", "C,USD,2\nN,US\nN,US\n",
		"N,US\nC,USD,2\n", "C,USD,2\r\nN,US\r\n",
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

func TestDefinitionIdentityIndependentOfRelease(t *testing.T) {
	first := loadFixture(t, "test-v1", "C,USD,2\nN,US\n")
	second := loadFixture(t, "test-v2", "C,JPY,0\nC,USD,2\nN,JP\nN,US\n")
	changed := loadFixture(t, "test-v3", "C,USD,3\nN,US\n")
	a, err := first.LookupCurrency("USD")
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.LookupCurrency("USD")
	if err != nil {
		t.Fatal(err)
	}
	d, err := changed.LookupCurrency("USD")
	if err != nil {
		t.Fatal(err)
	}
	if first.ReleaseID() == second.ReleaseID() || a != b || a.Version() != b.Version() || a.Version() == d.Version() {
		t.Fatal("release and definition identity are conflated")
	}
	if _, err := changed.RequireCurrency("USD", a.Exponent(), a.Version()); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatal("changed definition accepted under old identity")
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
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				got, err := c.LookupCurrency("USD")
				if err != nil || got.Exponent() != 2 || got == d {
					t.Error("lookup changed after value copy or concurrent reads")
					return
				}
			}
		}()
	}
	wg.Wait()
}
