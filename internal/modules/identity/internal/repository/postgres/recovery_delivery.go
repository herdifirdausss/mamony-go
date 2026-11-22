package postgres

import (
	"context"
	"errors"
	"regexp"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

var recoveryDeliveryLeaseID = regexp.MustCompile("^lse_[A-Za-z0-9_-]{22}$")

// RecoveryDeliveryStore depends exclusively on the dedicated v20
// mamony_identity_recovery_delivery_runtime SQL credential. The worker
// cannot SELECT account aliases, password verifiers or challenge digests.
type RecoveryDeliveryStore struct{ db Transactor }

func NewRecoveryDeliveryStore(db Transactor) (*RecoveryDeliveryStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &RecoveryDeliveryStore{db: db}, nil
}

// Claim atomically fences a worker's lease while checking that the current
// verified destination and session epoch still match the issued challenge.
// A stale/expired item is marked DEAD without revealing its recipient/blob.
func (s *RecoveryDeliveryStore) Claim(ctx context.Context, lease string) (usecase.RecoveryDeliveryJob, bool, error) {
	var job usecase.RecoveryDeliveryJob
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		!recoveryDeliveryLeaseID.MatchString(lease) {
		return job, false, ErrStorageUnavailable
	}
	var sendable bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT challenge_id,recipient,key_id,sealed_token,expires_at,sendable
			FROM mamony_identity.claim_password_recovery_delivery($1)`, lease).
			Scan(&job.ChallengeID, &job.Recipient, &job.KeyID, &job.SealedToken,
				&job.ExpiresAt, &sendable)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.RecoveryDeliveryJob{}, false, nil
	}
	if err != nil {
		return usecase.RecoveryDeliveryJob{}, false, ErrStorageUnavailable
	}
	if !sendable {
		// The SQL transaction committed an intentionally quarantined job.
		return usecase.RecoveryDeliveryJob{}, false, nil
	}
	job.LeaseID = lease
	return job, true, nil
}

func (s *RecoveryDeliveryStore) Finalize(ctx context.Context, challenge, lease string,
	outcome usecase.ProofDeliveryOutcome) error {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		!recoveryChallengeID.MatchString(challenge) ||
		!recoveryDeliveryLeaseID.MatchString(lease) ||
		(outcome != usecase.DeliveryAccepted && outcome != usecase.DeliveryTemporary &&
			outcome != usecase.DeliveryPermanent) {
		return ErrStorageUnavailable
	}
	var accepted bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT mamony_identity.finalize_password_recovery_delivery($1,$2,$3)`,
			challenge, lease, string(outcome)).Scan(&accepted)
	})
	if err != nil || !accepted {
		return ErrStorageUnavailable
	}
	return nil
}
