package postgres

import (
	"context"
	"crypto/sha256"
	"errors"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// RecoveryCodeSecurityNoticeStore requires ONLY the v26
// mamony_identity_recovery_code_notice_runtime EXECUTE-only credential.
// This worker can neither mint an offline code nor issue recovery approval.
type RecoveryCodeSecurityNoticeStore struct{ db Transactor }

func NewRecoveryCodeSecurityNoticeStore(db Transactor) (*RecoveryCodeSecurityNoticeStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &RecoveryCodeSecurityNoticeStore{db: db}, nil
}

func (s *RecoveryCodeSecurityNoticeStore) Claim(ctx context.Context, lease string) (usecase.RecoveryCodeSecurityNoticeJob, bool, error) {
	var job usecase.RecoveryCodeSecurityNoticeJob
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		!recoveryDeliveryLeaseID.MatchString(lease) {
		return job, false, ErrStorageUnavailable
	}
	var sendable bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT event_id,recipient,notice_kind,sendable FROM mamony_identity.claim_recovery_code_security_notice($1)",
			lease).Scan(&job.EventID, &job.Recipient, &job.Kind, &sendable)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.RecoveryCodeSecurityNoticeJob{}, false, nil
	}
	if err != nil {
		return usecase.RecoveryCodeSecurityNoticeJob{}, false, ErrStorageUnavailable
	}
	if !sendable {
		return usecase.RecoveryCodeSecurityNoticeJob{}, false, nil
	}
	job.LeaseID = lease
	return job, true, nil
}

func (s *RecoveryCodeSecurityNoticeStore) Finalize(ctx context.Context, event, lease string,
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
		normalized, ok := usecase.NormalizeMailgunMessageID(providerID)
		if !ok {
			return ErrStorageUnavailable
		}
		digest := sha256.Sum256([]byte(normalized))
		providerDigest = digest[:]
	} else if providerID != "" {
		return ErrStorageUnavailable
	}
	var committed bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.finalize_recovery_code_security_notice($1,$2,$3,$4)",
			event, lease, string(outcome), providerDigest).Scan(&committed)
	})
	if err != nil || !committed {
		return ErrStorageUnavailable
	}
	return nil
}

// RecoveryCodeSecurityNoticeHealth contains counts only, not user IDs,
// recipients, provider IDs or raw event identifiers.
type RecoveryCodeSecurityNoticeHealth struct {
	PendingDue           int64
	Dead                 int64
	SubmittedUnconfirmed int64
	Retrying             int64
	ExpiredPending       int64
}

type RecoveryCodeSecurityNoticeHealthStore struct{ db Transactor }

func NewRecoveryCodeSecurityNoticeHealthStore(db Transactor) (*RecoveryCodeSecurityNoticeHealthStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &RecoveryCodeSecurityNoticeHealthStore{db: db}, nil
}

func (s *RecoveryCodeSecurityNoticeHealthStore) Read(ctx context.Context) (RecoveryCodeSecurityNoticeHealth, error) {
	var result RecoveryCodeSecurityNoticeHealth
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil {
		return result, ErrStorageUnavailable
	}
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT * FROM mamony_identity.recovery_code_security_notice_health()").Scan(
			&result.PendingDue, &result.Dead, &result.SubmittedUnconfirmed,
			&result.Retrying, &result.ExpiredPending)
	})
	if err != nil {
		return RecoveryCodeSecurityNoticeHealth{}, ErrStorageUnavailable
	}
	return result, nil
}
