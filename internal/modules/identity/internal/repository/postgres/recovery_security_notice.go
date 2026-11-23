package postgres

import (
	"context"
	"crypto/sha256"
	"errors"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

type RecoverySecurityNoticeStore struct{ db Transactor }

func NewRecoverySecurityNoticeStore(db Transactor) (*RecoverySecurityNoticeStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &RecoverySecurityNoticeStore{db: db}, nil
}

func (s *RecoverySecurityNoticeStore) Claim(ctx context.Context, lease string) (usecase.RecoverySecurityNoticeJob, bool, error) {
	var job usecase.RecoverySecurityNoticeJob
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		!recoveryDeliveryLeaseID.MatchString(lease) {
		return job, false, ErrStorageUnavailable
	}
	var sendable bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT event_id,recipient,sendable FROM mamony_identity.claim_password_recovery_notice($1)",
			lease).Scan(&job.EventID, &job.Recipient, &sendable)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.RecoverySecurityNoticeJob{}, false, nil
	}
	if err != nil {
		return usecase.RecoverySecurityNoticeJob{}, false, ErrStorageUnavailable
	}
	if !sendable {
		return usecase.RecoverySecurityNoticeJob{}, false, nil
	}
	job.LeaseID = lease
	return job, true, nil
}

func (s *RecoverySecurityNoticeStore) Finalize(ctx context.Context, event, lease string,
	outcome usecase.ProofDeliveryOutcome, providerID string) error {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		len(event) != 26 || event[:4] != "evt_" ||
		!recoveryDeliveryLeaseID.MatchString(lease) ||
		(outcome != usecase.DeliveryAccepted && outcome != usecase.DeliveryTemporary &&
			outcome != usecase.DeliveryPermanent) {
		return ErrStorageUnavailable
	}
	var providerDigest any
	if outcome == usecase.DeliveryAccepted {
		normalized, valid := usecase.NormalizeMailgunMessageID(providerID)
		if !valid {
			return ErrStorageUnavailable
		}
		digest := sha256.Sum256([]byte(normalized))
		providerDigest = digest[:]
	} else if providerID != "" {
		return ErrStorageUnavailable
	}
	var finalized bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.finalize_password_recovery_notice($1,$2,$3,$4)",
			event, lease, string(outcome), providerDigest).Scan(&finalized)
	})
	if err != nil || !finalized {
		return ErrStorageUnavailable
	}
	return nil
}

type RecoverySecurityNoticeHealth struct {
	PendingDue           int64
	Dead                 int64
	SubmittedUnconfirmed int64
	Retrying             int64
	ExpiredPending       int64
}

type RecoverySecurityNoticeHealthStore struct{ db Transactor }

func NewRecoverySecurityNoticeHealthStore(db Transactor) (*RecoverySecurityNoticeHealthStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &RecoverySecurityNoticeHealthStore{db: db}, nil
}

func (s *RecoverySecurityNoticeHealthStore) Read(ctx context.Context) (RecoverySecurityNoticeHealth, error) {
	var result RecoverySecurityNoticeHealth
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil {
		return result, ErrStorageUnavailable
	}
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT * FROM mamony_identity.password_recovery_notice_health()").Scan(
			&result.PendingDue, &result.Dead, &result.SubmittedUnconfirmed,
			&result.Retrying, &result.ExpiredPending)
	})
	if err != nil {
		return RecoverySecurityNoticeHealth{}, ErrStorageUnavailable
	}
	return result, nil
}
