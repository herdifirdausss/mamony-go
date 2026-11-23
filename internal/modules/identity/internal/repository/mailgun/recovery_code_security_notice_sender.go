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

// RecoveryCodeSecurityNoticeSender uses the existing allowlisted Mailgun
// transport, TLS-only, redirect-denying settings and sanitized error policy.
// It never receives or sends recovery codes, enrollment approval proofs,
// sessions, account IDs, backup-code counts or recovery links.
type RecoveryCodeSecurityNoticeSender struct {
	transport *SecurityNoticeSender
}

func NewRecoveryCodeSecurityNoticeSender(cfg SecurityNoticeConfig, client *http.Client) (*RecoveryCodeSecurityNoticeSender, error) {
	transport, err := NewSecurityNoticeSender(cfg, client)
	if err != nil {
		return nil, ErrConfig
	}
	return &RecoveryCodeSecurityNoticeSender{transport: transport}, nil
}

func (s *RecoveryCodeSecurityNoticeSender) SendRecoveryCodeSecurityNotice(
	ctx context.Context, eventID, recipient, kind string) (string, error) {
	if s == nil || s.transport == nil || ctx == nil || ctx.Err() != nil ||
		!validSecurityEventID(eventID) || !validRecipient(recipient) {
		return "", usecase.ErrPermanentDelivery
	}
	var subject, message string
	switch kind {
	case "RECOVERY_CODES_ENROLLED":
		subject = "Mamony security alert: recovery codes created"
		message = "Offline recovery codes were created for your Mamony account.\n\n" +
			"If you made this change, keep your saved codes offline and private.\n" +
			"If you did not, open Mamony using a trusted route and contact support. " +
			"Never share your password or security codes.\n"
	case "RECOVERY_CODES_ROTATED":
		subject = "Mamony security alert: recovery codes replaced"
		message = "Offline recovery codes for your Mamony account were replaced. " +
			"Previously saved recovery codes can no longer be used.\n\n" +
			"If you did not make this change, open Mamony using a trusted route " +
			"and contact support. Never share your password or security codes.\n"
	default:
		return "", usecase.ErrPermanentDelivery
	}
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	fields := [][2]string{
		{"from", s.transport.from},
		{"to", recipient},
		{"subject", subject},
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
	id, ok := usecase.NormalizeMailgunMessageID(ack.ID)
	if !ok {
		return "", usecase.ErrDeliveryUnavailable
	}
	return id, nil
}
