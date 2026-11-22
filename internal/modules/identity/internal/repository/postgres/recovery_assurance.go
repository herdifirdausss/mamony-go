package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// RecoveryAssuranceStore uses ONLY mamony_identity_recovery_assurance_runtime.
// That role has one EXECUTE and no read or write access to any Identity table,
// including the enrolled code digest, challenge, grant or audit event.
type RecoveryAssuranceStore struct{ db Transactor }

func NewRecoveryAssuranceStore(db Transactor) (*RecoveryAssuranceStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &RecoveryAssuranceStore{db: db}, nil
}

func (s *RecoveryAssuranceStore) Approve(ctx context.Context,
	recoveryDigest, offlineCodeDigest [32]byte, eventID string) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		recoveryDigest == ([32]byte{}) || offlineCodeDigest == ([32]byte{}) ||
		len(eventID) != 26 || eventID[:4] != "evt_" {
		return false, usecase.ErrRecoveryAssuranceUnavailable
	}
	var approved bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.approve_password_recovery_offline_code($1,$2,$3)",
			recoveryDigest[:], offlineCodeDigest[:], eventID).Scan(&approved)
	})
	if err != nil {
		return false, usecase.ErrRecoveryAssuranceUnavailable
	}
	return approved, nil
}
