package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// RecoveryCodeGrantStore uses the trusted, isolated authenticator-grant
// credential; it can stage only the dedicated RECOVERY_CODE_ROTATION purpose.
type RecoveryCodeGrantStore struct{ db Transactor }

func NewRecoveryCodeGrantStore(db Transactor) (*RecoveryCodeGrantStore, error) {
	if db == nil {
		return nil, usecase.ErrSessionUnavailable
	}
	return &RecoveryCodeGrantStore{db: db}, nil
}

func (s *RecoveryCodeGrantStore) StageRecoveryCodes(ctx context.Context, subject string, epoch int64, digest []byte) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		!usecase.ValidRecoveryCodeSubject(subject) || epoch <= 0 || len(digest) != 32 {
		return false, usecase.ErrSessionUnavailable
	}
	var allowed bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.stage_recovery_code_rotation_grant($1,$2,$3)",
			subject, epoch, digest).Scan(&allowed)
	})
	if err != nil {
		return false, usecase.ErrSessionUnavailable
	}
	return allowed, nil
}
