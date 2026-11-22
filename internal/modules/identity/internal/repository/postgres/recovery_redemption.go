package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// RecoveryRedemptionStore is ONLY for
// mamony_identity_recovery_redeemer_runtime. It cannot select account,
// challenge, notifier, grant or verifier tables; only execute the owner
// controlled purpose-specific v21 recovery transaction.
type RecoveryRedemptionStore struct{ db Transactor }

func NewRecoveryRedemptionStore(db Transactor) (*RecoveryRedemptionStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &RecoveryRedemptionStore{db: db}, nil
}

func (s *RecoveryRedemptionStore) Redeem(ctx context.Context, tokenDigest [32]byte,
	email string, next domain.PasswordVerifier, event string) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		tokenDigest == ([32]byte{}) || !isCanonicalEmail(email) ||
		len(event) != 26 || event[:4] != "evt_" || !validVerifier(next) {
		return false, usecase.ErrPasswordRecoveryUnavailable
	}
	var ok bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.redeem_password_recovery($1,$2,$3,$4,$5)",
			tokenDigest[:], email, next.Salt, next.Digest, event).Scan(&ok)
	})
	if err != nil {
		// This includes a committed but unacknowledged write. Do not retry.
		return false, usecase.ErrPasswordRecoveryUnavailable
	}
	return ok, nil
}
