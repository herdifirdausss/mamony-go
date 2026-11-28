package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// A failure to start/commit a restricted PostgreSQL transaction must never
// turn a potentially valid bearer into a public invalid-proof diagnosis.
type failedConfirmationTx struct {
	err error
}

func (f failedConfirmationTx) WithTx(context.Context, func(pgx.Tx) error) error {
	return f.err
}

func TestIdentityProofConsumeClassifiesDatabaseFailuresWithoutLeakingDetails(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"unexpected database driver failure", errors.New("sensitive SQL connection string")},
		{"transaction returns unexpected no rows", pgx.ErrNoRows},
		{"database transaction canceled", context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewProofStore(failedConfirmationTx{err: tc.err})
			if err != nil {
				t.Fatal(err)
			}
			err = store.Consume(context.Background(), make([]byte, 32),
				"evt_AAAAAAAAAAAAAAAAAAAAAA")
			if !errors.Is(err, usecase.ErrEmailProofUnavailable) ||
				err.Error() != usecase.ErrEmailProofUnavailable.Error() {
				t.Fatal("raw transaction failure leaked or was considered an invalid proof")
			}
		})
	}
	if err := confirmationScanError(pgx.ErrNoRows); !errors.Is(err, usecase.ErrEmailProofInvalid) {
		t.Fatal("authoritative missing proof must be considered invalid")
	}
	if err := confirmationScanError(errors.New("sensitive SQL error")); !errors.Is(err, usecase.ErrEmailProofUnavailable) ||
		err.Error() != usecase.ErrEmailProofUnavailable.Error() {
		t.Fatal("unexpected SQL result leaked an internal error")
	}
	var absent *ProofStore
	err := absent.Consume(context.Background(), make([]byte, 32), "evt_AAAAAAAAAAAAAAAAAAAAAA")
	if !errors.Is(err, usecase.ErrEmailProofUnavailable) {
		t.Fatal("absent database authority must be unavailable")
	}
	err = absent.Consume(context.Background(), nil, "evt_AAAAAAAAAAAAAAAAAAAAAA")
	if !errors.Is(err, usecase.ErrEmailProofInvalid) {
		t.Fatal("malformed digest must be invalid before any store access")
	}
}
