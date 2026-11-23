package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// SecurityNoticeHealth is count-only; IDs, recipients, raw provider messages,
// webhook bodies and tokens must never cross this operator monitoring boundary.
type SecurityNoticeHealth struct {
	PendingDue       int64
	Dead             int64
	SubmittedUnknown int64
	Delivered        int64
	ProviderFailures int64
	UnmatchedEvents  int64
}

// SecurityNoticeHealthStore must use mamony_identity_maintenance_runtime,
// never the send-worker or webhook-receiver credential.
type SecurityNoticeHealthStore struct {
	db Transactor
}

func NewSecurityNoticeHealthStore(db Transactor) (*SecurityNoticeHealthStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &SecurityNoticeHealthStore{db: db}, nil
}

func (s *SecurityNoticeHealthStore) Read(ctx context.Context) (SecurityNoticeHealth, error) {
	var result SecurityNoticeHealth
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil {
		return result, ErrStorageUnavailable
	}
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT * FROM mamony_identity.security_notice_health()").Scan(
			&result.PendingDue, &result.Dead, &result.SubmittedUnknown,
			&result.Delivered, &result.ProviderFailures, &result.UnmatchedEvents)
	})
	if err != nil {
		return SecurityNoticeHealth{}, ErrStorageUnavailable
	}
	return result, nil
}
