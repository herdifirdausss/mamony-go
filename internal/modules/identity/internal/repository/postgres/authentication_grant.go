package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// AuthenticationGrantStore uses ONLY the narrowly provisioned grant-stager
// role; it cannot mint/resolve/revoke sessions or read credential verifiers.
// The trusted Identity Go login flow MUST call it only after real Argon2id.
type AuthenticationGrantStore struct {
	db Transactor
}

func NewAuthenticationGrantStore(db Transactor) (*AuthenticationGrantStore, error) {
	if db == nil {
		return nil, usecase.ErrSessionUnavailable
	}
	return &AuthenticationGrantStore{db: db}, nil
}

func (s *AuthenticationGrantStore) Stage(ctx context.Context, subject string, digest []byte) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		len(subject) != 26 || len(digest) != 32 {
		return false, usecase.ErrSessionUnavailable
	}
	var allowed bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT mamony_identity.stage_password_auth_grant($1,$2)",
			subject, digest).Scan(&allowed)
	})
	if err != nil {
		// Includes ambiguous commit; no caller can reuse a lost grant secret.
		return false, usecase.ErrSessionUnavailable
	}
	return allowed, nil
}
