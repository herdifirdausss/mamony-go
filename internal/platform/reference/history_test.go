package reference

import (
	"errors"
	"slices"
	"sync"
	"testing"
)

func historyFixture() ([]historyDefinition, []historyEvent) {
	definitions := []historyDefinition{
		{"mamony-currency-v2:ZZZ:1", "ZZZ", "fixture-unit:1", "fixture-evidence:1", 2},
		{"mamony-currency-v2:ZZZ:2", "ZZZ", "fixture-unit:1", "fixture-evidence:2", 3},
		{"mamony-currency-v2:ZZZ:3", "ZZZ", "fixture-unit:2", "fixture-evidence:3", 3},
		{"mamony-currency-v2:ZZZ:4", "ZZZ", "fixture-unit:2", "fixture-evidence:4", 2},
	}
	events := []historyEvent{
		{definitions[0].id, "2030-01-02", "2030-01-01T10:00:00Z", "fixture-evidence:5", true},
		{definitions[0].id, "2030-02-01", "2030-01-20T10:00:00Z", "fixture-evidence:6", false},
		{definitions[1].id, "2030-02-02", "2030-01-21T10:00:00Z", "fixture-evidence:7", true},
		{definitions[1].id, "2030-03-01", "2030-02-20T10:00:00Z", "fixture-evidence:8", false},
		{definitions[2].id, "2030-03-02", "2030-02-21T10:00:00Z", "fixture-evidence:9", true},
		{definitions[2].id, "2030-04-01", "2030-03-20T10:00:00Z", "fixture-evidence:10", false},
		{definitions[3].id, "2030-04-02", "2030-03-21T10:00:00Z", "fixture-evidence:11", true},
	}
	return definitions, events
}

func mustHistory(t *testing.T, release, accepted string, definitions []historyDefinition, events []historyEvent) currencyHistory {
	t.Helper()
	h, err := newCurrencyHistory(release, accepted, definitions, events)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestHistoryPinnedIdentityAndEffectiveSelection(t *testing.T) {
	d, e := historyFixture()
	h := mustHistory(t, "fixture-release:2", "2030-03-22T00:00:00Z", d, e)
	for _, tc := range []struct {
		date  string
		index int
	}{
		{"2030-01-01", -1}, {"2030-01-02", 0}, {"2030-01-31", 0}, {"2030-02-01", -1},
		{"2030-02-02", 1}, {"2030-03-01", -1}, {"2030-03-02", 2}, {"2030-04-01", -1},
		{"2030-04-02", 3}, {"2031-01-01", 3}, {"2030-02-30", -1}, {"", -1},
	} {
		got, err := h.selectDefinition("ZZZ", tc.date)
		if tc.index < 0 {
			if !errors.Is(err, errNoSelection) || got != (historyDefinition{}) {
				t.Fatalf("unexpected selection %s: %+v %v", tc.date, got, err)
			}
		} else if err != nil || got != d[tc.index] {
			t.Fatalf("wrong selection %s: %+v %v", tc.date, got, err)
		}
	}
	for _, want := range d {
		if got, err := h.lookupDefinition(want.id); err != nil || got != want {
			t.Fatalf("pinned definition changed: %+v %v", got, err)
		}
	}
	for _, id := range []string{"", "iso4217-minor-v1:ZZZ:2", "mamony-currency-v2:ZZZ:99"} {
		if got, err := h.lookupDefinition(id); !errors.Is(err, errUnknownDefinition) || got != (historyDefinition{}) {
			t.Fatal("unknown identity admitted")
		}
	}
	for _, code := range []string{"USD", "zzz", "ZZZ "} {
		if _, err := h.selectDefinition(code, "2030-04-02"); !errors.Is(err, errNoSelection) {
			t.Fatal("unexpected code selection")
		}
	}
}

func TestHistoryRejectsInvalidSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]historyDefinition, []historyEvent) ([]historyDefinition, []historyEvent)
	}{
		{"duplicate definition", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			return append(d, d[0]), e
		}},
		{"conflicting identity", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			x := d[0]
			x.exponent = 3
			return append(d, x), e
		}},
		{"invalid exponent", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			d[0].exponent = 4
			return d, e
		}},
		{"wrong namespace", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			d[0].id = "iso4217-minor-v1:ZZZ:2"
			return d, e
		}},
		{"invalid unit", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			d[0].unit = ""
			return d, e
		}},
		{"unknown definition", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			e[0].definition = "missing"
			return d, e
		}},
		{"missing date", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			e[0].effective = ""
			return d, e
		}},
		{"non UTC recorded", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			e[0].recorded = "2030-01-01T10:00:00+00:00"
			return d, e
		}},
		{"retroactive date", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			e[0].effective = "2029-12-31"
			return d, e
		}},
		{"overlap", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			return d, append(e[:1:1], e[2:]...)
		}},
		{"same date", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			e[2].effective = e[1].effective
			return d, e
		}},
		{"wrong retirement", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			e[1].definition = d[1].id
			return d, e
		}},
		{"recycled ID", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			e[2].definition = d[0].id
			return d, e
		}},
		{"acceptance order", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			e[1].recorded = "2030-01-01T00:00:00Z"
			return d, e
		}},
		{"after release", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			e[6].recorded = "2030-03-23T00:00:00Z"
			return d, e
		}},
		{"missing provenance", func(d []historyDefinition, e []historyEvent) ([]historyDefinition, []historyEvent) {
			e[0].evidence = ""
			return d, e
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, e := historyFixture()
			d, e = tc.mutate(d, e)
			h, err := newCurrencyHistory("fixture-release:2", "2030-03-22T00:00:00Z", d, e)
			if !errors.Is(err, errInvalidHistory) || h.release != "" || h.definitions != nil || h.events != nil {
				t.Fatalf("partial or accepted history: %+v %v", h, err)
			}
		})
	}
	d, e := historyFixture()
	for _, tc := range []struct {
		release, accepted string
		definitions       []historyDefinition
	}{{"", "2030-03-22T00:00:00Z", d}, {"fixture-release:2", "", d}, {"fixture-release:2", "2030-03-22T00:00:00Z", nil}} {
		if _, err := newCurrencyHistory(tc.release, tc.accepted, tc.definitions, e); !errors.Is(err, errInvalidHistory) {
			t.Fatal("invalid release accepted")
		}
	}
}

func TestHistoryAppendOnlyCompatibility(t *testing.T) {
	d, e := historyFixture()
	first := mustHistory(t, "fixture-release:1", "2030-01-10T00:00:00Z", d[:1], e[:1])
	next := mustHistory(t, "fixture-release:2", "2030-03-22T00:00:00Z", d, e)
	if err := requireHistoryExtension(first, next); err != nil {
		t.Fatal(err)
	}
	old, err := first.lookupDefinition(d[0].id)
	if err != nil {
		t.Fatal(err)
	}
	got, err := next.lookupDefinition(d[0].id)
	if err != nil || got != old {
		t.Fatal("definition changed across release")
	}
	unchanged := mustHistory(t, "fixture-release:3", "2030-03-23T00:00:00Z", d, e)
	if err := requireHistoryExtension(next, unchanged); err != nil {
		t.Fatal(err)
	}
	changed := slices.Clone(d)
	changed[0].exponent = 0
	changedEvidence := slices.Clone(e)
	changedEvidence[0].evidence = "fixture-evidence:99"
	backdated := append(slices.Clone(e[:1]), historyEvent{d[0].id, "2030-01-10", "2030-01-10T00:00:01Z", "fixture-evidence:99", false})
	for _, bad := range []currencyHistory{
		{}, first,
		mustHistory(t, "fixture-release:3", "2030-03-23T00:00:00Z", changed, e),
		mustHistory(t, "fixture-release:3", "2030-03-23T00:00:00Z", d, changedEvidence),
		mustHistory(t, "fixture-release:3", "2030-03-23T00:00:00Z", d[:1], nil),
		mustHistory(t, "fixture-release:3", "2030-03-23T00:00:00Z", d[:1], backdated),
	} {
		if err := requireHistoryExtension(first, bad); !errors.Is(err, errHistoryConflict) {
			t.Fatal("nonappend-only change accepted")
		}
	}
	// Even a still-unused definition is immutable once accepted.
	reserved := mustHistory(t, "fixture-release:2", "2030-01-11T00:00:00Z", d[:2], e[:1])
	removed := mustHistory(t, "fixture-release:3", "2030-01-12T00:00:00Z", d[:1], e[:1])
	if err := requireHistoryExtension(reserved, removed); !errors.Is(err, errHistoryConflict) {
		t.Fatal("unused definition removed")
	}
	reusedRelease := mustHistory(t, "fixture-release:1", "2030-03-23T00:00:00Z", d, e)
	if err := requireHistoryExtension(next, reusedRelease); !errors.Is(err, errHistoryConflict) {
		t.Fatal("release identity reused")
	}
	if err := requireHistoryExtension(next, first); !errors.Is(err, errHistoryConflict) {
		t.Fatal("rollback accepted")
	}
}

func TestHistoryImmutableConcurrentReads(t *testing.T) {
	d, e := historyFixture()
	want := d[0]
	h := mustHistory(t, "fixture-release:2", "2030-03-22T00:00:00Z", d, e)
	d[0].exponent = 3
	e[0].effective = "2099-01-01"
	returned, err := h.lookupDefinition(want.id)
	if err != nil {
		t.Fatal(err)
	}
	returned.unit = "changed"
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				got, err := h.selectDefinition("ZZZ", "2030-01-02")
				if err != nil || got != want || got == returned {
					t.Error("history mutated")
					return
				}
			}
		}()
	}
	wg.Wait()
	var zero currencyHistory
	if _, err := zero.lookupDefinition(want.id); !errors.Is(err, errUnknownDefinition) {
		t.Fatal("zero history accepted ID")
	}
	if _, err := zero.selectDefinition("ZZZ", "2030-01-02"); !errors.Is(err, errNoSelection) {
		t.Fatal("zero history selected definition")
	}
}
