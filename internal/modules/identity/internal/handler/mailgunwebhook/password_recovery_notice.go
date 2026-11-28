package mailgunwebhook

import (
	"net/http"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

// PasswordRecoveryNotice uses exactly the established bounded
// HMAC-SHA256 validation, timestamp/domain normalization, event classifier,
// key-rotation and anti-replay identity reduction of the existing verifier.
// It is a DIFFERENT unmounted HTTP handler whose recorder must use the v23
// recovery-specific SQL credential and recipient-free receipt table.
type PasswordRecoveryNotice struct {
	verified *SecurityNotice
}

func NewPasswordRecoveryNotice(keys [][]byte, domain string,
	recorder usecase.SecurityNoticeWebhookRecorder) (*PasswordRecoveryNotice, error) {
	v, err := NewSecurityNotice(keys, domain, recorder)
	if err != nil {
		return nil, usecase.ErrDeliveryUnavailable
	}
	return &PasswordRecoveryNotice{verified: v}, nil
}

func (h *PasswordRecoveryNotice) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.verified == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	h.verified.ServeHTTP(w, r)
}

var _ http.Handler = (*PasswordRecoveryNotice)(nil)
