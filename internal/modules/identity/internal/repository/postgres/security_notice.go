package postgres

import (
	"context"
	"errors"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// SecurityNoticeStore executes two narrowly scoped SQL functions using ONLY
// mamony_identity_security_notice_runtime, not a verifier/session credential.
type SecurityNoticeStore struct {
	db Transactor
}

func NewSecurityNoticeStore(db Transactor) (*SecurityNoticeStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &SecurityNoticeStore{db: db}, nil
}

func (s *SecurityNoticeStore) Claim(ctx context.Context, lease string) (usecase.SecurityNoticeJob, bool, error) {
	var job usecase.SecurityNoticeJob
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		len(lease) != 26 || lease[:4] != "lse_" {
		return job, false, ErrStorageUnavailable
	}
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT event_id,recipient FROM mamony_identity.claim_password_change_notice($1)",
			lease).Scan(&job.EventID, &job.Recipient)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.SecurityNoticeJob{}, false, nil
	}
	if err != nil {
		return usecase.SecurityNoticeJob{}, false, ErrStorageUnavailable
	}
	job.LeaseID = lease
	return job, true, nil
}

func (s *SecurityNoticeStore) Finalize(ctx context.Context, event, lease string, outcome usecase.ProofDeliveryOutcome) error {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		len(event) != 26 || len(lease) != 26 ||
		(outcome != usecase.DeliveryAccepted && outcome != usecase.DeliveryTemporary && outcome != usecase.DeliveryPermanent) {
		return ErrStorageUnavailable
	}
	var accepted bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.finalize_password_change_notice($1,$2,$3)",
			event, lease, string(outcome)).Scan(&accepted)
	})
	if err != nil || !accepted {
		return ErrStorageUnavailable
	}
	return nil
}
