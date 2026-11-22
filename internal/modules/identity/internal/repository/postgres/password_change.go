package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// PasswordChangeStore is intentionally backed by only the newly provisioned
// EXECUTE-only password-change runtime, not registration or session roles.
type PasswordChangeStore struct{ db Transactor }

func NewPasswordChangeStore(db Transactor) (*PasswordChangeStore, error) {
	if db == nil { return nil, usecase.ErrPasswordChangeUnavailable }
	return &PasswordChangeStore{db:db}, nil
}

func (s *PasswordChangeStore) Rotate(ctx context.Context,
	sessionDigest, grantDigest []byte, newVerifier domain.PasswordVerifier, event string) (bool,error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		len(sessionDigest) != 32 || len(grantDigest) != 32 ||
		len(event) != 26 || !validVerifier(newVerifier) {
		return false,usecase.ErrPasswordChangeUnavailable
	}
	var allowed bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.rotate_password_verifier($1,$2,$3,$4,$5)",
			sessionDigest,grantDigest,newVerifier.Salt,newVerifier.Digest,event).Scan(&allowed)
	})
	if err != nil { return false,usecase.ErrPasswordChangeUnavailable }
	return allowed,nil
}
