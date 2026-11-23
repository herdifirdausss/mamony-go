package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// RecoveryCodeManagerStore is bound only to
// mamony_identity_recovery_code_manager_runtime: one EXECUTE-only
// owner-controlled transaction; no raw bearer, email or account identity
// is passed as a selector.
type RecoveryCodeManagerStore struct{ db Transactor }

func NewRecoveryCodeManagerStore(db Transactor) (*RecoveryCodeManagerStore, error) {
	if db == nil {
		return nil, usecase.ErrRecoveryCodeManagerUnavailable
	}
	return &RecoveryCodeManagerStore{db: db}, nil
}

func (s *RecoveryCodeManagerStore) Replace(ctx context.Context, sessionDigest, grantDigest,
	proofDigest [32]byte, next [5][32]byte, eventID string) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		sessionDigest == ([32]byte{}) || grantDigest == ([32]byte{}) ||
		proofDigest == ([32]byte{}) || len(eventID) != 26 || eventID[:4] != "evt_" {
		return false, usecase.ErrRecoveryCodeManagerUnavailable
	}
	digests := make([][]byte, len(next))
	for i := range next {
		digests[i] = make([]byte, 32)
		copy(digests[i], next[i][:])
	}
	var allowed bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.replace_password_recovery_offline_codes($1,$2,$3,$4,$5)",
			sessionDigest[:], grantDigest[:], proofDigest[:], digests, eventID).Scan(&allowed)
	})
	if err != nil {
		return false, usecase.ErrRecoveryCodeManagerUnavailable
	}
	return allowed, nil
}
