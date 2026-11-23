package postgres

import (
	"context"
	"errors"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// PasskeyRegistrationEvidenceStore uses ONLY the independent EXECUTE-only
// registration evidence reader role. It never grants table SELECT, credential
// binding, challenge consumption, session issuance or account mutation.
type PasskeyRegistrationEvidenceStore struct{ db Transactor }

func NewPasskeyRegistrationEvidenceStore(db Transactor) (*PasskeyRegistrationEvidenceStore, error) {
	if db == nil {
		return nil, usecase.ErrPasskeyRegistrationUnavailable
	}
	return &PasskeyRegistrationEvidenceStore{db: db}, nil
}

func (s *PasskeyRegistrationEvidenceStore) Read(ctx context.Context,
	digest [32]byte, id string) (usecase.PasskeyRegistrationEvidence, bool, error) {
	var evidence usecase.PasskeyRegistrationEvidence
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		digest == ([32]byte{}) || len(id) != 26 || id[:4] != "wrg_" {
		return evidence, false, usecase.ErrPasskeyRegistrationUnavailable
	}
	var nonce []byte
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT subject_id,subject_epoch,challenge,rp_id,origin,policy_generation
			FROM mamony_identity.read_passkey_registration_intent($1,$2)`,
			digest[:], id).Scan(&evidence.Subject, &evidence.Epoch, &nonce,
			&evidence.RPID, &evidence.Origin, &evidence.PolicyGeneration)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.PasskeyRegistrationEvidence{}, false, nil
	}
	if err != nil || len(nonce) != 32 {
		return usecase.PasskeyRegistrationEvidence{}, false, usecase.ErrPasskeyRegistrationUnavailable
	}
	copy(evidence.Nonce[:], nonce)
	return evidence, true, nil
}
