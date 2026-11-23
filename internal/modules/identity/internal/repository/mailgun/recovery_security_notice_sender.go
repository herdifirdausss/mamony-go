package mailgun

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

// RecoverySecurityNoticeSender is deliberately distinct from both the
// password-changed notice and recovery proof/link sender. Its message is
// always fixed, text-only, link-free and token-free.
type RecoverySecurityNoticeSender struct {
	transport *SecurityNoticeSender
}

func NewRecoverySecurityNoticeSender(cfg SecurityNoticeConfig, client *http.Client) (*RecoverySecurityNoticeSender, error) {
	transport, err := NewSecurityNoticeSender(cfg, client)
	if err != nil {
		return nil, ErrConfig
	}
	return &RecoverySecurityNoticeSender{transport: transport}, nil
}

func (s *RecoverySecurityNoticeSender) SendPasswordRecovered(ctx context.Context, eventID, recipient string) (string, error) {
	if s == nil || s.transport == nil || ctx == nil || ctx.Err() != nil ||
		!validSecurityEventID(eventID) || !validRecipient(recipient) {
		return "", usecase.ErrPermanentDelivery
	}
	const message = "The password for your Mamony account was reset through account recovery.\n\n" +
		"If you requested this reset, no action is needed.\n" +
		"If you did not request this reset, open Mamony using a trusted route " +
		"and contact support through an official channel. Never share your password or security codes.\n"
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	fields := [][2]string{
		{"from", s.transport.from},
		{"to", recipient},
		{"subject", "Mamony security alert: password recovered"},
		{"text", message},
		{"o:tracking", "no"},
		{"o:tracking-clicks", "no"},
		{"o:tracking-opens", "no"},
		{"o:require-tls", "yes"},
		{"o:deliver-within", "5m"},
	}
	for _, pair := range fields {
		if err := form.WriteField(pair[0], pair[1]); err != nil {
			return "", usecase.ErrDeliveryUnavailable
		}
	}
	if err := form.Close(); err != nil {
		return "", usecase.ErrDeliveryUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.transport.endpoint, &buf)
	if err != nil {
		return "", usecase.ErrDeliveryUnavailable
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth("api", s.transport.key)
	resp, err := s.transport.client.Do(req)
	if err != nil {
		// Provider errors may carry secrets, tokens, email or URLs.
		return "", usecase.ErrDeliveryUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 420 ||
			resp.StatusCode == http.StatusRequestTimeout ||
			resp.StatusCode >= http.StatusInternalServerError {
			return "", usecase.ErrDeliveryUnavailable
		}
		return "", usecase.ErrPermanentDelivery
	}
	var ack struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&ack); err != nil {
		return "", usecase.ErrDeliveryUnavailable
	}
	id, valid := usecase.NormalizeMailgunMessageID(ack.ID)
	if !valid {
		return "", usecase.ErrDeliveryUnavailable
	}
	return id, nil
}
