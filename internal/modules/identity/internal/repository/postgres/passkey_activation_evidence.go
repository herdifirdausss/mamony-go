package postgres

import (
	"context"
	"errors"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// PasskeyActivationEvidenceStore has only EXECUTE on the separately
// gated v34 pending-credential reader, not direct SELECT or UPDATE.
type PasskeyActivationEvidenceStore struct{ db Transactor }

func NewPasskeyActivationEvidenceStore(db Transactor) (*PasskeyActivationEvidenceStore, error) {
	if db == nil {
		return nil, usecase.ErrPasskeyActivationUnavailable
	}
	return &PasskeyActivationEvidenceStore{db: db}, nil
}

func (s *PasskeyActivationEvidenceStore) Read(ctx context.Context,
	sessionDigest [32]byte, challengeID string) (usecase.PasskeyActivationEvidence, bool, error) {
	var empty usecase.PasskeyActivationEvidence
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		sessionDigest == ([32]byte{}) ||
		len(challengeID) != 26 || challengeID[:4] != "pav_" {
		return empty, false, usecase.ErrPasskeyActivationUnavailable
	}
	var result usecase.PasskeyActivationEvidence
	var nonce, aaguid []byte
	var signCount int64
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT subject_id,subject_epoch,challenge,credential_id,public_key_cose,"+
				"aaguid,sign_count,backup_eligible,backup_state,rp_id,origin,policy_generation "+
				"FROM mamony_identity.read_passkey_activation_challenge($1,$2)",
			sessionDigest[:], challengeID).Scan(
			&result.Subject, &result.Epoch, &nonce,
			&result.CredentialID, &result.PublicKeyCOSE, &aaguid,
			&signCount, &result.BackupEligible, &result.BackupState,
			&result.RPID, &result.Origin, &result.PolicyGeneration)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, false, nil
	}
	if err != nil || ctx.Err() != nil ||
		len(nonce) != 32 || len(aaguid) != 16 ||
		signCount < 0 || signCount > 4294967295 {
		return empty, false, usecase.ErrPasskeyActivationUnavailable
	}
	copy(result.Nonce[:], nonce)
	copy(result.AAGUID[:], aaguid)
	result.SignCount = uint32(signCount)
	return result, true, nil
}
