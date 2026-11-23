package handler

import (
	"net/http"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

// MailgunRecoveryCodeNoticeWebhook uses exactly the established bounded
// HMAC-SHA256 validation, timestamp/domain normalization, event classifier,
// key-rotation and anti-replay identity reduction of the existing verifier.
// It is a DIFFERENT unmounted HTTP handler whose recorder must use the v27
// recovery-code-specific SQL credential and recipient-free receipt table.
type MailgunRecoveryCodeNoticeWebhook struct {
	verified *MailgunSecurityNoticeWebhook
}

func NewMailgunRecoveryCodeNoticeWebhook(keys [][]byte, domain string,
	recorder usecase.SecurityNoticeWebhookRecorder) (*MailgunRecoveryCodeNoticeWebhook, error) {
	v, err := NewMailgunSecurityNoticeWebhook(keys, domain, recorder)
	if err != nil {
		return nil, usecase.ErrDeliveryUnavailable
	}
	return &MailgunRecoveryCodeNoticeWebhook{verified: v}, nil
}

func (h *MailgunRecoveryCodeNoticeWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.verified == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	h.verified.ServeHTTP(w, r)
}

var _ http.Handler = (*MailgunRecoveryCodeNoticeWebhook)(nil)
