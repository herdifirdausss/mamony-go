package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// RecoveryCodeNoticeWebhookStore is exclusively backed by the
// mamony_identity_recovery_code_notice_webhook_runtime credential. It has no
// SELECT/UPDATE privileges on either recovery-code or other password-change notices.
// Only signed, normalized digests enter the owner-controlled v27 function.
type RecoveryCodeNoticeWebhookStore struct{ db Transactor }

func NewRecoveryCodeNoticeWebhookStore(db Transactor) (*RecoveryCodeNoticeWebhookStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &RecoveryCodeNoticeWebhookStore{db: db}, nil
}

func (s *RecoveryCodeNoticeWebhookStore) Record(ctx context.Context, receipt usecase.VerifiedSecurityNoticeReceipt) error {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil {
		return ErrStorageUnavailable
	}
	var committed bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.record_mailgun_recovery_code_notice_receipt($1,$2,$3,$4,$5)",
			receipt.TokenDigest[:], receipt.EventDigest[:], receipt.MessageDigest[:],
			receipt.DeliveryState, receipt.EventAt).Scan(&committed)
	})
	if err != nil || !committed {
		return ErrStorageUnavailable
	}
	return nil
}
