package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// PasskeyRegistrationIntentStore has exactly one capability:
// EXECUTE issue_passkey_registration_intent as the dedicated issuer runtime.
// No credential binder, ceremony verifier, passkey table or public route
// exists. Direct table privileges are intentionally absent.
type PasskeyRegistrationIntentStore struct{ db Transactor }

func NewPasskeyRegistrationIntentStore(db Transactor) (*PasskeyRegistrationIntentStore, error) {
	if db == nil {
		return nil, usecase.ErrPasskeyRegistrationUnavailable
	}
	return &PasskeyRegistrationIntentStore{db: db}, nil
}

func (s *PasskeyRegistrationIntentStore) Issue(ctx context.Context,
	sessionDigest [32]byte, intentID string, challenge [32]byte,
	rpID, origin string) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		sessionDigest == ([32]byte{}) || challenge == ([32]byte{}) ||
		len(intentID) != 26 || intentID[:4] != "wrg_" ||
		rpID == "" || origin != "https://"+rpID {
		return false, usecase.ErrPasskeyRegistrationUnavailable
	}
	var ok bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.issue_passkey_registration_intent($1,$2,$3,$4,$5)",
			sessionDigest[:], intentID, challenge[:], rpID, origin).Scan(&ok)
	})
	if err != nil {
		return false, usecase.ErrPasskeyRegistrationUnavailable
	}
	return ok, nil
}
