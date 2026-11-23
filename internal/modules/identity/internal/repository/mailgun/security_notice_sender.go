package mailgun

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

// SecurityNoticeConfig intentionally has no verification URL or signing key.
// Password-change mail contains no bearer, reset link, personalization,
// tracking pixel or user-supplied message text.
type SecurityNoticeConfig struct {
	Region Region
	Domain string
	From   string
	APIKey string
}

// SecurityNoticeSender is independent of the token-bearing verification
// sender. Both target an allowlisted Mailgun region and a configured domain.
type SecurityNoticeSender struct {
	endpoint string
	from     string
	key      string
	client   http.Client
}

func NewSecurityNoticeSender(cfg SecurityNoticeConfig, client *http.Client) (*SecurityNoticeSender, error) {
	var host string
	switch cfg.Region {
	case RegionUS:
		host = "api.mailgun.net"
	case RegionEU:
		host = "api.eu.mailgun.net"
	case RegionAU:
		host = "api.au.mailgun.net"
	default:
		return nil, ErrConfig
	}
	if !validDomain(cfg.Domain) || cfg.APIKey == "" || len(cfg.APIKey) > 512 ||
		strings.ContainsAny(cfg.APIKey, "\r\n") || strings.ContainsAny(cfg.From, "\r\n") {
		return nil, ErrConfig
	}
	from, err := mail.ParseAddress(cfg.From)
	if err != nil || from.Address == "" {
		return nil, ErrConfig
	}
	parts := strings.Split(from.Address, "@")
	if len(parts) != 2 || !strings.EqualFold(parts[1], cfg.Domain) {
		return nil, ErrConfig
	}
	c := http.Client{Timeout: 9 * time.Second}
	if client != nil {
		c = *client
		if c.Timeout == 0 || c.Timeout > 9*time.Second {
			c.Timeout = 9 * time.Second
		}
	}
	// Never follow a redirect with authorization headers or recipient details,
	// even if it points to the same host.
	c.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &SecurityNoticeSender{
		endpoint: "https://" + host + "/v3/" + cfg.Domain + "/messages",
		from:     cfg.From,
		key:      cfg.APIKey,
		client:   c,
	}, nil
}

func validSecurityEventID(id string) bool {
	if len(id) != 26 || !strings.HasPrefix(id, "evt_") {
		return false
	}
	for _, ch := range id[4:] {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' ||
			ch >= '0' && ch <= '9' || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

// SendPasswordChanged implements the internal token-free notice port.
// The event ID is a local correlation key, NOT a Mailgun exactly-once key.
// A provider may accept an ambiguous HTTP request more than once.
func (s *SecurityNoticeSender) SendPasswordChanged(ctx context.Context, eventID, recipient string) error {
	if s == nil || ctx == nil || ctx.Err() != nil ||
		!validSecurityEventID(eventID) || !validRecipient(recipient) {
		return usecase.ErrPermanentDelivery
	}
	const message = "The password for your Mamony account was changed.\n\n" +
		"If you made this change, no action is needed.\n" +
		"If you did not make this change, open Mamony directly using your usual trusted method " +
		"and contact support through an official channel. Never share your password or security codes.\n"
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	fields := [][2]string{
		{"from", s.from},
		{"to", recipient},
		{"subject", "Mamony security alert: password changed"},
		{"text", message},
		{"o:tracking", "no"},
		{"o:tracking-clicks", "no"},
		{"o:tracking-opens", "no"},
		{"o:require-tls", "yes"},
		{"o:deliver-within", "5m"},
	}
	for _, pair := range fields {
		if err := form.WriteField(pair[0], pair[1]); err != nil {
			return usecase.ErrDeliveryUnavailable
		}
	}
	if err := form.Close(); err != nil {
		return usecase.ErrDeliveryUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, &buf)
	if err != nil {
		return usecase.ErrDeliveryUnavailable
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth("api", s.key)
	resp, err := s.client.Do(req)
	if err != nil {
		// Never expose provider URLs, credentials, recipient addresses or
		// response bodies via returned errors, audits or process logs.
		return usecase.ErrDeliveryUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 420 ||
			resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode >= http.StatusInternalServerError {
			return usecase.ErrDeliveryUnavailable
		}
		return usecase.ErrPermanentDelivery
	}
	var accepted struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&accepted); err != nil ||
		len(accepted.ID) < 3 || len(accepted.ID) > 512 {
		return usecase.ErrDeliveryUnavailable
	}
	return nil
}
