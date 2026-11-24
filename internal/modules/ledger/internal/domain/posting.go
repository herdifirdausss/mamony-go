package domain

import (
	"errors"
	"math"
	"math/big"
	"slices"
	"strings"
)

var (
	ErrSnapshot = errors.New("invalid ledger account snapshots")
	ErrState    = errors.New("account not active")
	ErrFloor    = errors.New("balance below floor")
	ErrRange    = errors.New("ledger value out of range")
)

type AccountStep struct {
	Account  AccountID
	Before   int64
	Delta    int64
	After    int64
	Sequence int64
}

// PostingPlan describes a calculation only. It cannot authorize or persist it.
type PostingPlan struct {
	entries []Entry
	steps   []AccountStep
}

func (p PostingPlan) Entries() []Entry {
	return slices.Clone(p.entries)
}

func (p PostingPlan) Steps() []AccountStep {
	return slices.Clone(p.steps)
}

// PlanPosting evaluates final per-account net effects against supplied snapshots.
// Snapshot acquisition, authority, locking and persistence are outside K1.
func PlanPosting(j JournalDraft, snapshots []AccountSnapshot) (PostingPlan, error) {
	if err := j.validate(); err != nil {
		return PostingPlan{}, err
	}
	if len(snapshots) == 0 || len(snapshots) > len(j.entries) {
		return PostingPlan{}, ErrSnapshot
	}
	accounts := slices.Clone(snapshots)
	slices.SortFunc(accounts, func(a, b AccountSnapshot) int {
		return strings.Compare(string(a.ID), string(b.ID))
	})
	byID := make(map[AccountID]AccountSnapshot, len(accounts))
	for _, a := range accounts {
		if !a.valid() || a.Book != j.book || a.Currency != j.currency {
			return PostingPlan{}, ErrSnapshot
		}
		if _, exists := byID[a.ID]; exists {
			return PostingPlan{}, ErrSnapshot
		}
		byID[a.ID] = a
	}

	nets := make(map[AccountID]*big.Int, len(accounts))
	for _, e := range j.entries {
		a, exists := byID[e.Account]
		if !exists {
			return PostingPlan{}, ErrSnapshot
		}
		net := nets[e.Account]
		if net == nil {
			net = new(big.Int)
			nets[e.Account] = net
		}
		amount := big.NewInt(e.Amount.minor)
		if e.Direction == a.Type.normalSide() {
			net.Add(net, amount)
		} else {
			net.Sub(net, amount)
		}
	}
	if len(nets) != len(accounts) {
		return PostingPlan{}, ErrSnapshot
	}

	steps := make([]AccountStep, 0, len(accounts))
	for _, a := range accounts {
		if a.Status != Active {
			return PostingPlan{}, ErrState
		}
		after := new(big.Int).Add(big.NewInt(a.Balance), nets[a.ID])
		if after.Sign() < 0 {
			return PostingPlan{}, ErrFloor
		}
		if !after.IsInt64() || a.Sequence == math.MaxInt64 {
			return PostingPlan{}, ErrRange
		}
		final := after.Int64()
		// Both balances are nonnegative int64, so their difference fits.
		steps = append(steps, AccountStep{
			Account:  a.ID,
			Before:   a.Balance,
			Delta:    final - a.Balance,
			After:    final,
			Sequence: a.Sequence + 1,
		})
	}
	return PostingPlan{entries: j.Entries(), steps: steps}, nil
}
