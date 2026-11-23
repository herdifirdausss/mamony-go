package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// PasskeyRegistrationReauthGrantStore stages only the PASSKEY_REGISTRATION_REAUTH
// purpose under the existing trusted password-grant runtime, AFTER real
// admission-limited Argon2id. It has no credential / session authority.
type PasskeyRegistrationReauthGrantStore struct{ db Transactor }

func NewPasskeyRegistrationReauthGrantStore(db Transactor) (*PasskeyRegistrationReauthGrantStore, error) {
	if db == nil {
		return nil, usecase.ErrPasskeyAdmissionUnavailable
	}
	return &PasskeyRegistrationReauthGrantStore{db: db}, nil
}

func (s *PasskeyRegistrationReauthGrantStore) StagePasskey(ctx context.Context,
	subject string, epoch int64, digest []byte) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		len(subject) != 26 || subject[:4] != "sub_" || epoch <= 0 ||
		len(digest) != 32 {
		return false, usecase.ErrPasskeyAdmissionUnavailable
	}
	var allowed bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.stage_passkey_registration_reauth_grant($1,$2,$3)",
			subject, epoch, digest).Scan(&allowed)
	})
	if err != nil || ctx.Err() != nil {
		return false, usecase.ErrPasskeyAdmissionUnavailable
	}
	return allowed, nil
}

// PasskeyRegistrationAdmissionStore runs ONLY as the dedicated v32
// EXECUTE-only admission principal, never the grant issuer or v31 reader.
type PasskeyRegistrationAdmissionStore struct{ db Transactor }

func NewPasskeyRegistrationAdmissionStore(db Transactor) (*PasskeyRegistrationAdmissionStore, error) {
	if db == nil {
		return nil, usecase.ErrPasskeyAdmissionUnavailable
	}
	return &PasskeyRegistrationAdmissionStore{db: db}, nil
}

func (s *PasskeyRegistrationAdmissionStore) Admit(ctx context.Context,
	sessionDigest [32]byte, intentID string, grantDigest [32]byte) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		sessionDigest == ([32]byte{}) || grantDigest == ([32]byte{}) ||
		len(intentID) != 26 || intentID[:4] != "wrg_" {
		return false, usecase.ErrPasskeyAdmissionUnavailable
	}
	var allowed bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.admit_passkey_registration_intent($1,$2,$3)",
			sessionDigest[:], intentID, grantDigest[:]).Scan(&allowed)
	})
	if err != nil || ctx.Err() != nil {
		return false, usecase.ErrPasskeyAdmissionUnavailable
	}
	return allowed, nil
}
