package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// SecurityNoticeWebhookStore must be constructed with the dedicated
// mamony_identity_notice_webhook_runtime pool. The SQL role has EXECUTE
// on one narrowly scoped receipt-recording function and no table access.
type SecurityNoticeWebhookStore struct {
	db Transactor
}

func NewSecurityNoticeWebhookStore(db Transactor) (*SecurityNoticeWebhookStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &SecurityNoticeWebhookStore{db: db}, nil
}

func (s *SecurityNoticeWebhookStore) Record(ctx context.Context, item usecase.VerifiedSecurityNoticeReceipt) error {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil {
		return ErrStorageUnavailable
	}
	var recorded bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT mamony_identity.record_mailgun_security_notice_receipt($1,$2,$3,$4,$5)`,
			item.TokenDigest[:], item.EventDigest[:], item.MessageDigest[:],
			item.DeliveryState, item.EventAt).Scan(&recorded)
	})
	if err != nil || !recorded {
		return ErrStorageUnavailable
	}
	return nil
}
