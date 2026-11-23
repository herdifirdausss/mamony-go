package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// PasskeyActivationChallengeStore is an EXECUTE-only, distinct issuer
// principal for v34. A pending credential is NOT a trusted login factor.
type PasskeyActivationChallengeStore struct{ db Transactor }

func NewPasskeyActivationChallengeStore(db Transactor) (*PasskeyActivationChallengeStore, error) {
	if db == nil {
		return nil, usecase.ErrPasskeyActivationUnavailable
	}
	return &PasskeyActivationChallengeStore{db: db}, nil
}

func (s *PasskeyActivationChallengeStore) Issue(ctx context.Context,
	sessionDigest [32]byte, credentialID []byte, challengeID string,
	nonce [32]byte, rpID, origin string) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		sessionDigest == ([32]byte{}) || nonce == ([32]byte{}) ||
		len(credentialID) < 16 || len(credentialID) > 1023 ||
		len(challengeID) != 26 || challengeID[:4] != "pav_" ||
		rpID == "" || origin != "https://"+rpID {
		return false, usecase.ErrPasskeyActivationUnavailable
	}
	var allowed bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.issue_passkey_activation_challenge($1,$2,$3,$4,$5,$6)",
			sessionDigest[:], credentialID, challengeID, nonce[:], rpID, origin).Scan(&allowed)
	})
	if err != nil || ctx.Err() != nil {
		return false, usecase.ErrPasskeyActivationUnavailable
	}
	return allowed, nil
}
