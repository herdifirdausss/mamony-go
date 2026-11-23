package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// PasskeyRegistrationStageStore MUST use only the separate v33 staging role.
// Never reuse the v30 issuer, v31 evidence reader, v32 admission, SQL
// migrator or public HTTP role. That role cannot directly INSERT credential
// rows, issue login grants or enroll recovery factors.
type PasskeyRegistrationStageStore struct{ db Transactor }

func NewPasskeyRegistrationStageStore(db Transactor) (*PasskeyRegistrationStageStore, error) {
	if db == nil {
		return nil, usecase.ErrPasskeyStageUnavailable
	}
	return &PasskeyRegistrationStageStore{db: db}, nil
}

func (s *PasskeyRegistrationStageStore) Stage(ctx context.Context,
	sessionDigest [32]byte, intentID string,
	evidence usecase.PasskeyRegistrationEvidence,
	candidate usecase.PasskeyRegistrationCandidate, event string) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		sessionDigest == ([32]byte{}) || evidence.Nonce == ([32]byte{}) ||
		evidence.Epoch <= 0 || evidence.PolicyGeneration != 1 ||
		len(intentID) != 26 || intentID[:4] != "wrg_" ||
		evidence.RPID == "" || evidence.Origin != "https://"+evidence.RPID ||
		len(candidate.CredentialID) < 16 || len(candidate.CredentialID) > 1023 ||
		len(candidate.PublicKeyCOSE) < 16 || len(candidate.PublicKeyCOSE) > 2048 ||
		len(candidate.AAGUID) != 16 || !candidate.UserPresent ||
		!candidate.UserVerified || candidate.BackupState && !candidate.BackupEligible ||
		len(candidate.AttestationFormat) == 0 || len(candidate.AttestationFormat) > 64 ||
		len(candidate.AttestationType) == 0 || len(candidate.AttestationType) > 64 ||
		len(event) != 26 || event[:4] != "evt_" {
		return false, usecase.ErrPasskeyStageUnavailable
	}
	var allowed bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.stage_verified_passkey_credential("+
				"$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)",
			sessionDigest[:], intentID, evidence.Nonce[:], evidence.Epoch,
			evidence.RPID, evidence.Origin, candidate.CredentialID,
			candidate.PublicKeyCOSE, candidate.AAGUID,
			candidate.AttestationFormat, candidate.AttestationType,
			candidate.UserPresent, candidate.UserVerified,
			candidate.BackupEligible, candidate.BackupState,
			int64(candidate.AuthenticatorCount), event).Scan(&allowed)
	})
	if err != nil || ctx.Err() != nil {
		// Includes unknown COMMIT outcome, late event/credential conflict
		// or a failed notice insertion. Never retry this transaction.
		return false, usecase.ErrPasskeyStageUnavailable
	}
	return allowed, nil
}
