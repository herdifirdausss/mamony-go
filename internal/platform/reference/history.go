package reference

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	errInvalidHistory    = errors.New("invalid synthetic currency history")
	errHistoryConflict   = errors.New("currency history is not append-only")
	errUnknownDefinition = errors.New("unknown historical definition")
	errNoSelection       = errors.New("no effective currency definition")
)

// These private values model synthetic history only; Load remains unchanged.
type historyDefinition struct {
	id, code, unit, evidence string
	exponent                 uint8
}

type historyEvent struct {
	definition, effective, recorded, evidence string
	activate                                  bool
}

type currencyHistory struct {
	release, accepted string
	definitions       []historyDefinition
	events            []historyEvent
}

func validFixtureID(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	revision := strings.TrimPrefix(value, prefix)
	n, err := strconv.ParseUint(revision, 10, 64)
	return err == nil && n != 0 && strconv.FormatUint(n, 10) == revision
}

func validHistoryDate(value string) bool {
	d, err := time.Parse(time.DateOnly, value)
	return err == nil && d.Year() > 0 && d.Format(time.DateOnly) == value
}

func validHistoryTime(value string) bool {
	t, err := time.Parse(time.RFC3339, value)
	return err == nil && t.Year() > 0 && t.UTC().Format(time.RFC3339) == value
}

// newCurrencyHistory validates a complete immutable synthetic snapshot. Each
// code has ordered activate/retire events; activation cannot replace an active
// selection. Same-day transitions and retroactive corrections are not modeled.
func newCurrencyHistory(release, accepted string, definitions []historyDefinition, events []historyEvent) (currencyHistory, error) {
	if !validFixtureID(release, "fixture-release:") || !validHistoryTime(accepted) || len(definitions) == 0 {
		return currencyHistory{}, errInvalidHistory
	}
	byID := make(map[string]historyDefinition, len(definitions))
	for _, d := range definitions {
		if !asciiCode(d.code, 3) || !validFixtureID(d.id, "mamony-currency-v2:"+d.code+":") ||
			!validFixtureID(d.unit, "fixture-unit:") || !validFixtureID(d.evidence, "fixture-evidence:") || d.exponent > 3 {
			return currencyHistory{}, errInvalidHistory
		}
		if _, exists := byID[d.id]; exists {
			return currencyHistory{}, errInvalidHistory
		}
		byID[d.id] = d
	}
	active := make(map[string]string)
	lastDate := make(map[string]string)
	used := make(map[string]bool)
	lastRecorded := ""
	for _, e := range events {
		d, ok := byID[e.definition]
		if !ok || !validHistoryDate(e.effective) || !validHistoryTime(e.recorded) ||
			!validFixtureID(e.evidence, "fixture-evidence:") || e.recorded < lastRecorded || e.recorded > accepted ||
			e.effective < e.recorded[:10] || e.effective <= lastDate[d.code] {
			return currencyHistory{}, errInvalidHistory
		}
		if e.activate {
			if active[d.code] != "" || used[d.id] {
				return currencyHistory{}, errInvalidHistory
			}
			active[d.code] = d.id
			used[d.id] = true
		} else {
			if active[d.code] != d.id {
				return currencyHistory{}, errInvalidHistory
			}
			delete(active, d.code)
		}
		lastDate[d.code] = e.effective
		lastRecorded = e.recorded
	}
	return currencyHistory{release: release, accepted: accepted, definitions: slices.Clone(definitions), events: slices.Clone(events)}, nil
}

func (h currencyHistory) lookupDefinition(id string) (historyDefinition, error) {
	for _, d := range h.definitions {
		if d.id == id {
			return d, nil
		}
	}
	return historyDefinition{}, errUnknownDefinition
}

// selectDefinition is effective-date selection within this accepted snapshot,
// not a knowledge-time query or a financial authorization decision.
func (h currencyHistory) selectDefinition(code, date string) (historyDefinition, error) {
	if !validHistoryDate(date) || !asciiCode(code, 3) {
		return historyDefinition{}, errNoSelection
	}
	var selected historyDefinition
	for _, e := range h.events {
		d, err := h.lookupDefinition(e.definition)
		if err != nil {
			return historyDefinition{}, errInvalidHistory
		}
		if d.code != code || e.effective > date {
			continue
		}
		if e.activate {
			selected = d
		} else {
			selected = historyDefinition{}
		}
	}
	if selected.id == "" {
		return historyDefinition{}, errNoSelection
	}
	return selected, nil
}

// requireHistoryExtension compares validated snapshots. Preserve the entire
// prior prefix, including provenance and retired definitions. New events must
// be accepted later and cannot change dates at or before the prior cutoff day.
func requireHistoryExtension(previous, next currencyHistory) error {
	if previous.release == "" || next.release == "" || next.accepted <= previous.accepted ||
		len(next.definitions) < len(previous.definitions) || len(next.events) < len(previous.events) ||
		!slices.Equal(previous.definitions, next.definitions[:len(previous.definitions)]) ||
		!slices.Equal(previous.events, next.events[:len(previous.events)]) {
		return errHistoryConflict
	}
	// Release revisions must also increase so an earlier release ID cannot recur.
	oldRevision, err := strconv.ParseUint(strings.TrimPrefix(previous.release, "fixture-release:"), 10, 64)
	if err != nil {
		return errHistoryConflict
	}
	newRevision, err := strconv.ParseUint(strings.TrimPrefix(next.release, "fixture-release:"), 10, 64)
	if err != nil || newRevision <= oldRevision {
		return errHistoryConflict
	}
	for _, e := range next.events[len(previous.events):] {
		if e.recorded <= previous.accepted || e.effective <= previous.accepted[:10] {
			return errHistoryConflict
		}
	}
	return nil
}
