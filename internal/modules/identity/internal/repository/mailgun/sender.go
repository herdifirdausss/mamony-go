// Package mailgun provides a narrowly configured Mailgun Messages API adapter.
// No worker, HTTP handler or production credentials are wired by this package.
package mailgun

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

var ErrConfig = errors.New("mailgun sender configuration unavailable")

type Region string

const (
	RegionUS Region = "US"
	RegionEU Region = "EU"
	RegionAU Region = "AU"
)

type Config struct {
	Region          Region
	Domain          string
	From            string
	APIKey          string
	VerificationURL string
}

type Sender struct {
	endpoint  string
	from      string
	key       string
	verifyURL url.URL
	client    http.Client
}

// NewSender validates static deployment settings. The network target is
// derived only from a supported Mailgun region, never an input URL. Tests may
// inject a transport that sends requests to an in-process mock HTTP server.
func NewSender(cfg Config, client *http.Client) (*Sender, error) {
	host := ""
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
		strings.ContainsAny(cfg.APIKey, "\r\n") {
		return nil, ErrConfig
	}
	from, err := mail.ParseAddress(cfg.From)
	if err != nil || from.Address == "" ||
		!strings.EqualFold(strings.Split(from.Address, "@")[len(strings.Split(from.Address, "@"))-1], cfg.Domain) ||
		strings.ContainsAny(cfg.From, "\r\n") {
		return nil, ErrConfig
	}
	parsed, err := url.Parse(cfg.VerificationURL)
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.User != nil ||
		parsed.Hostname() == "" || parsed.Host != parsed.Hostname() ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" ||
		!validDomain(parsed.Hostname()) || len(cfg.VerificationURL) > 512 {
		return nil, ErrConfig
	}
	c := http.Client{Timeout: 9 * time.Second}
	if client != nil {
		c = *client
		if c.Timeout == 0 || c.Timeout > 9*time.Second {
			c.Timeout = 9 * time.Second
		}
	}
	// Reject redirects, including same-host redirects. They may disclose the
	// API key or the challenge URL, and are never expected for a send call.
	c.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Sender{
		endpoint:  "https://" + host + "/v3/" + cfg.Domain + "/messages",
		from:      cfg.From,
		key:       cfg.APIKey,
		verifyURL: *parsed,
		client:    c,
	}, nil
}

func validDomain(domain string) bool {
	if len(domain) > 253 || len(domain) < 3 || strings.ToLower(domain) != domain ||
		strings.HasSuffix(domain, ".") || !strings.Contains(domain, ".") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

func validID(id string) bool {
	if len(id) != 26 || !strings.HasPrefix(id, "vch_") {
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

func validRecipient(raw string) bool {
	if len(raw) > 254 || raw == "" || strings.ToLower(raw) != raw {
		return false
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil || addr.Address != raw || strings.ContainsAny(raw, "\r\n,; ") {
		return false
	}
	parts := strings.Split(raw, "@")
	return len(parts) == 2 && validDomain(parts[1])
}

func validSecret(secret string) bool {
	if len(secret) != 43 {
		return false
	}
	b, err := base64.RawURLEncoding.DecodeString(secret)
	return err == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == secret
}

// SendVerification implements usecase.EmailSender. The fragment is delivered
// to the browser but not sent in a normal HTTP request to the verification
// page. A future frontend must explicitly POST it to a secure verification
// API. No email tracking, template variables, HTML or per-recipient metadata.
func (s *Sender) SendVerification(ctx context.Context, challengeID, recipient, secret string) error {
	if s == nil || !validID(challengeID) || !validRecipient(recipient) || !validSecret(secret) {
		return usecase.ErrPermanentDelivery
	}
	verify := s.verifyURL
	verify.Fragment = "token=" + secret
	text := "Verify your Mamony email address by opening this link:\n\n" +
		verify.String() + "\n\nThis link expires 20 minutes after issuance. " +
		"If you did not request it, ignore this email.\n"
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	values := [][2]string{
		{"from", s.from},
		{"to", recipient},
		{"subject", "Verify your Mamony email"},
		{"text", text},
		{"o:tracking", "no"},
		{"o:tracking-clicks", "no"},
		{"o:tracking-opens", "no"},
		{"o:require-tls", "yes"},
		{"o:deliver-within", "5m"},
	}
	for _, pair := range values {
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
		return usecase.ErrDeliveryUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	// Never return provider error bodies or HTTP URL errors: they can contain
	// submitted verification links or a provider's sensitive diagnostics.
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == 420 ||
			resp.StatusCode == http.StatusRequestTimeout ||
			resp.StatusCode >= http.StatusInternalServerError {
			return usecase.ErrDeliveryUnavailable
		}
		return usecase.ErrPermanentDelivery
	}
	var accepted struct {
		ID string `json:"id"`
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 4096))
	if err := dec.Decode(&accepted); err != nil || len(accepted.ID) < 3 || len(accepted.ID) > 512 {
		return usecase.ErrDeliveryUnavailable
	}
	return nil
}
