package mailgun

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

// RecoveryConfig is a separate Mailgun sender configuration: the reset link
// is pinned to the intended first-party HTTPS page, never request Host.
// This package does not start a process or send any email at construction.
type RecoveryConfig struct {
	Region      Region
	Domain      string
	From        string
	APIKey      string
	RecoveryURL string
}

type RecoveryMailSender struct{ transport *Sender }

func NewRecoveryMailSender(cfg RecoveryConfig, client *http.Client) (*RecoveryMailSender, error) {
	parsed, err := url.Parse(cfg.RecoveryURL)
	if err != nil || parsed == nil || parsed.Path != "/account/recover-password" ||
		parsed.RawPath != "" || parsed.ForceQuery {
		return nil, ErrConfig
	}
	base, err := NewSender(Config{
		Region: cfg.Region, Domain: cfg.Domain, From: cfg.From,
		APIKey: cfg.APIKey, VerificationURL: cfg.RecoveryURL,
	}, client)
	if err != nil {
		return nil, ErrConfig
	}
	return &RecoveryMailSender{transport: base}, nil
}

func validRecoveryID(id string) bool {
	if len(id) != 26 || !strings.HasPrefix(id, "rcv_") {
		return false
	}
	for _, c := range id[4:] {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
			c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// SendRecovery submits a token-bearing message ONLY to the current verified
// email selected by Identity SQL. It does not redeem proofs or authenticate.
// Mailgun acceptance is not inbox delivery; ambiguous HTTP outcomes may retry.
func (s *RecoveryMailSender) SendRecovery(ctx context.Context, challenge, recipient, bearer string) error {
	if s == nil || s.transport == nil || ctx == nil || ctx.Err() != nil ||
		!validRecoveryID(challenge) || !validRecipient(recipient) || !validSecret(bearer) {
		return usecase.ErrPermanentDelivery
	}
	landing := s.transport.verifyURL
	landing.Fragment = "token=" + bearer
	message := "A password recovery request was received for your Mamony account.\n\n" +
		"To continue, open this link:\n\n" + landing.String() + "\n\n" +
		"This link expires 15 minutes after it was issued and can only be used once.\n" +
		"If you did not request recovery, ignore this message.\n" +
		"Never share this link or any security code.\n"
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	fields := [][2]string{
		{"from", s.transport.from},
		{"to", recipient},
		{"subject", "Mamony account password recovery"},
		{"text", message},
		{"o:tracking", "no"},
		{"o:tracking-clicks", "no"},
		{"o:tracking-opens", "no"},
		{"o:require-tls", "yes"},
		{"o:deliver-within", "5m"},
	}
	for _, field := range fields {
		if err := form.WriteField(field[0], field[1]); err != nil {
			return usecase.ErrDeliveryUnavailable
		}
	}
	if err := form.Close(); err != nil {
		return usecase.ErrDeliveryUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.transport.endpoint, &buf)
	if err != nil {
		return usecase.ErrDeliveryUnavailable
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth("api", s.transport.key)
	resp, err := s.transport.client.Do(req)
	if err != nil {
		return usecase.ErrDeliveryUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 420 ||
			resp.StatusCode == http.StatusRequestTimeout ||
			resp.StatusCode >= http.StatusInternalServerError {
			return usecase.ErrDeliveryUnavailable
		}
		return usecase.ErrPermanentDelivery
	}
	var ack struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&ack); err != nil {
		return usecase.ErrDeliveryUnavailable
	}
	if _, ok := usecase.NormalizeMailgunMessageID(ack.ID); !ok {
		return usecase.ErrDeliveryUnavailable
	}
	return nil
}
