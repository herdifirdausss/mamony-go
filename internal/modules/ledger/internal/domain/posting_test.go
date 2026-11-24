package domain

import (
	"errors"
	"math"
	"math/big"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func fixtureCurrency(t *testing.T, code string, exponent uint8) CurrencyRef {
	t.Helper()
	c, err := NewCurrencyRef(code, "fixture-v1", exponent)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func fixtureEntry(t *testing.T, c CurrencyRef, id AccountID, side Direction, amount int64) Entry {
	t.Helper()
	m, err := NewMoney(c, amount)
	if err != nil {
		t.Fatal(err)
	}
	return Entry{Account: id, Direction: side, Amount: m}
}

func fixtureAccount(c CurrencyRef, id AccountID, kind AccountType, balance int64) AccountSnapshot {
	return AccountSnapshot{ID: id, Book: "book", Currency: c, Type: kind, Status: Active, Balance: balance}
}

func TestCurrencyAndIdentifiers(t *testing.T) {
	for _, tc := range []struct {
		code string
		exp  uint8
	}{
		{"USD", 2}, {"JPY", 0}, {"KWD", 3},
	} {
		c := fixtureCurrency(t, tc.code, tc.exp)
		if c.exponent != tc.exp || c.code != tc.code {
			t.Fatal("wrong currency definition")
		}
	}
	for _, tc := range []struct {
		code, version string
		exp           uint8
	}{
		{"usd", "v", 2}, {"US", "v", 2}, {"US1", "v", 2},
		{"USD", "", 2}, {"USD", "v", 4}, {"USD", "a\x00b", 2},
		{"USD", strings.Repeat("x", 129), 2}, {"USD", "\xff", 2},
	} {
		if _, err := NewCurrencyRef(tc.code, tc.version, tc.exp); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid currency accepted: %+v, %v", tc, err)
		}
	}
	if !validIdentifier(strings.Repeat("é", 64)) || validIdentifier(strings.Repeat("é", 65)) {
		t.Fatal("identifier bound must count bytes")
	}
	if _, err := NewMoney(CurrencyRef{}, 0); !errors.Is(err, ErrInvalid) {
		t.Fatal("zero currency accepted")
	}
}

func TestJournalShape(t *testing.T) {
	c := fixtureCurrency(t, "USD", 2)
	jpy := fixtureCurrency(t, "JPY", 0)
	debit := fixtureEntry(t, c, "cash", Debit, 1)
	credit := fixtureEntry(t, c, "capital", Credit, 1)
	pair := []Entry{debit, credit}
	wide := make([]Entry, 0, 64)
	for range 32 {
		wide = append(wide, debit, credit)
	}
	for _, tc := range []struct {
		name    string
		entries []Entry
		valid   bool
	}{
		{"empty", nil, false},
		{"one", pair[:1], false},
		{"two", pair, true},
		{"64", wide, true},
		{"65", append(slices.Clone(wide), debit), false},
		{"unbalanced", []Entry{debit, debit}, false},
		{"zero", []Entry{fixtureEntry(t, c, "cash", Debit, 0), credit}, false},
		{"negative", []Entry{fixtureEntry(t, c, "cash", Debit, -1), credit}, false},
		{"mixed", []Entry{debit, fixtureEntry(t, jpy, "capital", Credit, 1)}, false},
		{"direction", []Entry{fixtureEntry(t, c, "cash", "X", 1), credit}, false},
		{"empty account", []Entry{fixtureEntry(t, c, "", Debit, 1), credit}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewJournalDraft("book", c, tc.entries)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
		})
	}
}

func TestPostingNetAndImmutability(t *testing.T) {
	c := fixtureCurrency(t, "USD", 2)
	accounts := []AccountSnapshot{
		fixtureAccount(c, "b", CustomerAvailable, 100),
		fixtureAccount(c, "a", CustomerAvailable, 0),
	}
	entries := []Entry{
		fixtureEntry(t, c, "b", Debit, 150),
		fixtureEntry(t, c, "b", Credit, 80),
		fixtureEntry(t, c, "a", Credit, 70),
	}
	j, err := NewJournalDraft("book", c, entries)
	if err != nil {
		t.Fatal(err)
	}
	original := slices.Clone(entries)
	entries[0] = Entry{}
	before := slices.Clone(accounts)
	p, err := PlanPosting(j, accounts)
	if err != nil {
		t.Fatal(err)
	}
	want := []AccountStep{
		{Account: "a", Before: 0, Delta: 70, After: 70, Sequence: 1},
		{Account: "b", Before: 100, Delta: -70, After: 30, Sequence: 1},
	}
	if !reflect.DeepEqual(p.Steps(), want) || !reflect.DeepEqual(p.Entries(), original) ||
		!reflect.DeepEqual(accounts, before) {
		t.Fatal("wrong plan or mutated inputs")
	}
	p.Steps()[0].After = 999
	p.Entries()[0] = Entry{}
	j.Entries()[0] = Entry{}
	slices.Reverse(accounts)
	again, err := PlanPosting(j, accounts)
	if err != nil || !reflect.DeepEqual(again.Steps(), want) ||
		!reflect.DeepEqual(p.Steps(), want) || !reflect.DeepEqual(p.Entries(), original) {
		t.Fatal("plan aliases output or depends on snapshot order")
	}
}

func TestPostingRejections(t *testing.T) {
	c := fixtureCurrency(t, "USD", 2)
	other, err := NewCurrencyRef("USD", "fixture-v2", 2)
	if err != nil {
		t.Fatal(err)
	}
	base := []AccountSnapshot{
		fixtureAccount(c, "a", CustomerAvailable, 100),
		fixtureAccount(c, "b", CustomerAvailable, 0),
	}
	j, err := NewJournalDraft("book", c, []Entry{
		fixtureEntry(t, c, "a", Debit, 100), fixtureEntry(t, c, "b", Credit, 100),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func([]AccountSnapshot) []AccountSnapshot
		want error
	}{
		{"floor", func(a []AccountSnapshot) []AccountSnapshot { a[0].Balance = 99; return a }, ErrFloor},
		{"range", func(a []AccountSnapshot) []AccountSnapshot { a[1].Balance = math.MaxInt64; return a }, ErrRange},
		{"sequence", func(a []AccountSnapshot) []AccountSnapshot { a[0].Sequence = math.MaxInt64; return a }, ErrRange},
		{"negative snapshot", func(a []AccountSnapshot) []AccountSnapshot { a[0].Balance = -1; return a }, ErrSnapshot},
		{"negative seq", func(a []AccountSnapshot) []AccountSnapshot { a[0].Sequence = -1; return a }, ErrSnapshot},
		{"suspended", func(a []AccountSnapshot) []AccountSnapshot { a[0].Status = Suspended; return a }, ErrState},
		{"closed", func(a []AccountSnapshot) []AccountSnapshot { a[0].Status = Closed; return a }, ErrState},
		{"invalid type", func(a []AccountSnapshot) []AccountSnapshot { a[0].Type = ""; return a }, ErrSnapshot},
		{"wrong book", func(a []AccountSnapshot) []AccountSnapshot { a[0].Book = "other"; return a }, ErrSnapshot},
		{"wrong version", func(a []AccountSnapshot) []AccountSnapshot { a[0].Currency = other; return a }, ErrSnapshot},
		{"missing", func(a []AccountSnapshot) []AccountSnapshot { return a[:1] }, ErrSnapshot},
		{"duplicate", func(a []AccountSnapshot) []AccountSnapshot { a[1] = a[0]; return a }, ErrSnapshot},
		{"unreferenced", func(a []AccountSnapshot) []AccountSnapshot { a[1].ID = "c"; return a }, ErrSnapshot},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.edit(slices.Clone(base))
			before := slices.Clone(a)
			p, err := PlanPosting(j, a)
			if !errors.Is(err, tc.want) || len(p.Steps()) != 0 || len(p.Entries()) != 0 ||
				!reflect.DeepEqual(a, before) {
				t.Fatalf("expected refusal %v without effects, got %v", tc.want, err)
			}
		})
	}
}

func TestPostingWideTotalsAndZeroNet(t *testing.T) {
	c := fixtureCurrency(t, "USD", 2)
	accounts := []AccountSnapshot{
		fixtureAccount(c, "a", CollectionCash, 0),
		fixtureAccount(c, "b", CollectionCash, 0),
		fixtureAccount(c, "c", ContributedCapital, 0),
		fixtureAccount(c, "d", ContributedCapital, 0),
	}
	j, err := NewJournalDraft("book", c, []Entry{
		fixtureEntry(t, c, "a", Debit, math.MaxInt64),
		fixtureEntry(t, c, "b", Debit, math.MaxInt64),
		fixtureEntry(t, c, "c", Credit, math.MaxInt64),
		fixtureEntry(t, c, "d", Credit, math.MaxInt64),
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := PlanPosting(j, accounts)
	if err != nil || len(p.Steps()) != 4 {
		t.Fatalf("wide balanced journal refused: %v", err)
	}
	for _, step := range p.Steps() {
		if step.After != math.MaxInt64 || step.Sequence != 1 {
			t.Fatal("wrong wide journal effect")
		}
	}
	z, err := NewJournalDraft("book", c, []Entry{
		fixtureEntry(t, c, "a", Debit, math.MaxInt64),
		fixtureEntry(t, c, "a", Credit, math.MaxInt64),
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err = PlanPosting(z, accounts[:1])
	if err != nil || !reflect.DeepEqual(p.Steps(), []AccountStep{{Account: "a", Sequence: 1}}) {
		t.Fatalf("wrong zero-net plan: %v", err)
	}
	accounts[0].Status = Suspended
	if _, err := PlanPosting(z, accounts[:1]); !errors.Is(err, ErrState) {
		t.Fatal("zero net bypassed state")
	}
}

// signedOracle reconstructs debit-normal positions independently of the
// production net accumulator. This oracle is for generated valid-shape inputs.
func signedOracle(accounts []AccountSnapshot, entries []Entry) ([]AccountStep, bool) {
	var result []AccountStep
	for _, a := range accounts {
		position := big.NewInt(a.Balance)
		if a.Type != CollectionCash {
			position.Neg(position)
		}
		for _, e := range entries {
			if e.Account != a.ID {
				continue
			}
			if e.Direction == Debit {
				position.Add(position, big.NewInt(e.Amount.minor))
			} else {
				position.Sub(position, big.NewInt(e.Amount.minor))
			}
		}
		if a.Type != CollectionCash {
			position.Neg(position)
		}
		if position.Sign() < 0 || !position.IsInt64() || a.Sequence == math.MaxInt64 {
			return nil, false
		}
		after := position.Int64()
		result = append(result, AccountStep{Account: a.ID, Before: a.Balance, Delta: after - a.Balance, After: after, Sequence: a.Sequence + 1})
	}
	return result, true
}

func TestPostingAgainstIndependentModel(t *testing.T) {
	c := fixtureCurrency(t, "USD", 2)
	rng := rand.New(rand.NewPCG(7, 19))
	accepted, refused := 0, 0
	for i := range 2000 {
		accounts := []AccountSnapshot{
			fixtureAccount(c, "a", CollectionCash, rng.Int64N(1000)),
			fixtureAccount(c, "b", CustomerAvailable, rng.Int64N(1000)),
		}
		entries := make([]Entry, 0, 64)
		for range 1 + rng.IntN(32) {
			amount := 1 + rng.Int64N(100)
			if i%17 == 0 {
				amount = math.MaxInt64
			}
			entries = append(entries, fixtureEntry(t, c, "a", Debit, amount), fixtureEntry(t, c, "b", Credit, amount))
		}
		// Swap some balanced pairs to vary signs, still touching both accounts.
		for k := 0; k < len(entries); k += 2 {
			if rng.IntN(2) == 0 {
				entries[k].Direction, entries[k+1].Direction = Credit, Debit
			}
		}
		j, err := NewJournalDraft("book", c, entries)
		if err != nil {
			t.Fatal(err)
		}
		want, ok := signedOracle(accounts, entries)
		got, err := PlanPosting(j, accounts)
		if (err == nil) != ok || (ok && !reflect.DeepEqual(want, got.Steps())) {
			t.Fatalf("case %d disagrees with oracle: %v", i, err)
		}
		if !ok {
			refused++
			continue
		}
		accepted++
		steps := got.Steps()
		if steps[0].Delta != steps[1].Delta {
			t.Fatal("debit-normal conservation failed")
		}
		permuted := slices.Clone(entries)
		slices.Reverse(permuted)
		other, err := NewJournalDraft("book", c, permuted)
		if err != nil {
			t.Fatal(err)
		}
		p, err := PlanPosting(other, accounts)
		if err != nil || !reflect.DeepEqual(p.Steps(), steps) {
			t.Fatal("entry permutation changed balances")
		}
		for k := range accounts {
			accounts[k].Balance = steps[k].After
			accounts[k].Sequence = steps[k].Sequence
		}
		for k := range permuted {
			if permuted[k].Direction == Debit {
				permuted[k].Direction = Credit
			} else {
				permuted[k].Direction = Debit
			}
		}
		inverse, err := NewJournalDraft("book", c, permuted)
		if err != nil {
			t.Fatal(err)
		}
		back, err := PlanPosting(inverse, accounts)
		if err != nil {
			t.Fatal(err)
		}
		for k, step := range back.Steps() {
			if step.After != steps[k].Before || step.Sequence != 2 {
				t.Fatal("inverse did not restore balance and advance sequence")
			}
		}
	}
	if accepted == 0 || refused == 0 {
		t.Fatal("generator did not exercise both outcomes")
	}
	t.Logf("independent model: %d accepted, %d refused", accepted, refused)
}
