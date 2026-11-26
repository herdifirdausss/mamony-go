package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// PasskeyActivationFinishStore is an EXECUTE-only v36 SQL authority. The
// PostgreSQL connection MUST use the distinct finisher runtime principal,
// never the HTTP, issuer, reader, staging, session or migrator credentials.
// SQL rechecks the exact original signed evidence under fresh row locks;
// it does not itself verify WebAuthn signatures.
// The adapter validates its persistence input contract independently of the
// usecase. The database owns the authoritative state transition. Keep those
// responsibilities separate when evolving validation.
type PasskeyActivationFinishStore struct {
	db Transactor
}

func NewPasskeyActivationFinishStore(db Transactor) (*PasskeyActivationFinishStore, error) {
	if db == nil {
		return nil, usecase.ErrPasskeyActivationCommitUnavailable
	}
	return &PasskeyActivationFinishStore{db: db}, nil
}

// Finish returns false with nil error only for a definitive database rejection.
// Invalid internal handoff, unavailable processing and uncertain transaction
// outcomes return a sanitized domain error, not a definitive rejection.
// A true result with nil error means committed possession only.
func (s *PasskeyActivationFinishStore) Finish(ctx context.Context,
	r usecase.PasskeyActivationFinishRequest) (bool, error) {
	e := r.Expected
	v := r.Verified
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		r.SessionDigest == ([32]byte{}) ||
		len(r.ChallengeID) != 26 || r.ChallengeID[:4] != "pav_" ||
		e.Epoch <= 0 || e.Nonce == ([32]byte{}) ||
		e.PolicyGeneration != 1 || e.RPID == "" ||
		e.Origin != "https://"+e.RPID ||
		len(e.CredentialID) < 16 || len(e.CredentialID) > 1023 ||
		len(e.PublicKeyCOSE) < 16 || len(e.PublicKeyCOSE) > 2048 ||
		!v.UserPresent || !v.UserVerified ||
		len(v.CredentialID) != len(e.CredentialID) ||
		!equalOpaqueCredentialID(v.CredentialID, e.CredentialID) ||
		v.BackupEligible != e.BackupEligible ||
		(e.BackupState && !e.BackupEligible) ||
		(v.BackupState && !v.BackupEligible) ||
		((e.SignCount != 0 || v.Counter != 0) && v.Counter <= e.SignCount) ||
		len(r.EventID) != 26 || r.EventID[:4] != "evt_" {
		return false, usecase.ErrPasskeyActivationCommitUnavailable
	}
	var allowed bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.finish_verified_passkey_possession("+
				"$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)",
			r.SessionDigest[:], r.ChallengeID, e.Nonce[:], e.Epoch,
			e.RPID, e.Origin, e.CredentialID, e.PublicKeyCOSE,
			e.AAGUID[:], int64(e.SignCount), e.BackupEligible,
			e.BackupState, int64(v.Counter), v.BackupEligible,
			v.BackupState, v.UserPresent, v.UserVerified, r.EventID,
		).Scan(&allowed)
	})
	if err != nil || ctx.Err() != nil {
		// An ambiguous PostgreSQL COMMIT is never reported as success.
		// Client must not auto-replay a consumed challenge.
		return false, usecase.ErrPasskeyActivationCommitUnavailable
	}
	return allowed, nil
}
