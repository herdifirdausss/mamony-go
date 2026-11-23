package handler

import (
	"net/http"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

// MailgunRecoveryNoticeWebhook uses exactly the established bounded
// HMAC-SHA256 validation, timestamp/domain normalization, event classifier,
// key-rotation and anti-replay identity reduction of the existing verifier.
// It is a DIFFERENT unmounted HTTP handler whose recorder must use the v23
// recovery-specific SQL credential and recipient-free receipt table.
type MailgunRecoveryNoticeWebhook struct {
	verified *MailgunSecurityNoticeWebhook
}

func NewMailgunRecoveryNoticeWebhook(keys [][]byte, domain string,
	recorder usecase.SecurityNoticeWebhookRecorder) (*MailgunRecoveryNoticeWebhook, error) {
	v, err := NewMailgunSecurityNoticeWebhook(keys, domain, recorder)
	if err != nil {
		return nil, usecase.ErrDeliveryUnavailable
	}
	return &MailgunRecoveryNoticeWebhook{verified: v}, nil
}

func (h *MailgunRecoveryNoticeWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.verified == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	h.verified.ServeHTTP(w, r)
}

var _ http.Handler = (*MailgunRecoveryNoticeWebhook)(nil)
