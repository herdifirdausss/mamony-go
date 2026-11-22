package mailgun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

const fixtureNoticeEvent = "evt_AAAAAAAAAAAAAAAAAAAAAA"

func newMockSecuritySender(t *testing.T, handler http.HandlerFunc) *SecurityNoticeSender {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	origin, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Timeout:   600 * time.Millisecond,
		Transport: redirectToMock{target: origin, base: http.DefaultTransport},
	}
	s, err := NewSecurityNoticeSender(SecurityNoticeConfig{
		Region: RegionEU,
		Domain: "mg.example.test",
		From:   "Mamony Security <security@mg.example.test>",
		APIKey: "synthetic-mailgun-sending-key",
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSecurityNoticeSenderSendsOnlyTokenFreeTextWithPinnedMailgunHost(t *testing.T) {
	requests := 0
	sender := newMockSecuritySender(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost ||
			r.URL.Path != "/v3/mg.example.test/messages" {
			t.Error("unexpected Mailgun request endpoint")
		}
		name, secret, ok := r.BasicAuth()
		if !ok || name != "api" || secret != "synthetic-mailgun-sending-key" {
			t.Error("missing domain sending key")
		}
		if r.Header.Get("Accept") != "application/json" ||
			!strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
			t.Error("request must be multipart with JSON response")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 12<<10)
		mr, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		fields := make(map[string][]string)
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Error("invalid multipart field")
				return
			}
			value, err := io.ReadAll(io.LimitReader(part, 4097))
			_ = part.Close()
			if err != nil || len(value) > 4096 {
				t.Error("message field too large")
				return
			}
			fields[part.FormName()] = append(fields[part.FormName()], string(value))
		}
		want := map[string]string{
			"from":              "Mamony Security <security@mg.example.test>",
			"to":                "alice@example.test",
			"subject":           "Mamony security alert: password changed",
			"o:tracking":        "no",
			"o:tracking-opens":  "no",
			"o:tracking-clicks": "no",
			"o:require-tls":     "yes",
			"o:deliver-within":  "5m",
		}
		for field, expected := range want {
			if len(fields[field]) != 1 || fields[field][0] != expected {
				t.Errorf("unexpected Mailgun field %q", field)
			}
		}
		if len(fields["text"]) != 1 ||
			!strings.Contains(fields["text"][0], "password") ||
			!strings.Contains(fields["text"][0], "contact support") ||
			strings.Contains(fields["text"][0], fixtureNoticeEvent) ||
			strings.Contains(fields["text"][0], "https://") ||
			strings.Contains(fields["text"][0], "http://") ||
			strings.Contains(fields["text"][0], "#token=") {
			t.Error("security alert must not contain a bearer, event ID or link")
		}
		if len(fields) != len(want)+1 {
			t.Error("unexpected dynamic Mailgun metadata was submitted")
		}
		for _, field := range []string{"html", "bcc", "cc", "v:tag", "template",
			"h:X-Idempotency-Key", "o:inbox-placement-test-key"} {
			if _, found := fields[field]; found {
				t.Errorf("forbidden Mailgun metadata %q", field)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"<synthetic-accepted@mailgun.test>","message":"Queued. Thank you."}`)
	})
	if sender.endpoint != "https://api.eu.mailgun.net/v3/mg.example.test/messages" {
		t.Fatal("sender used a caller-controlled or incorrect API endpoint")
	}
	if _, err := sender.SendPasswordChanged(context.Background(), fixtureNoticeEvent,
		"alice@example.test"); err != nil || requests != 1 {
		t.Fatalf("expected one mock provider acceptance: %v requests=%d", err, requests)
	}
}

func TestSecurityNoticeMailgunRegionsAndConfiguration(t *testing.T) {
	for region, host := range map[Region]string{
		RegionUS: "api.mailgun.net",
		RegionEU: "api.eu.mailgun.net",
		RegionAU: "api.au.mailgun.net",
	} {
		s, err := NewSecurityNoticeSender(SecurityNoticeConfig{
			Region: region, Domain: "mg.example.test",
			From: "security@mg.example.test", APIKey: "synthetic",
		}, nil)
		if err != nil || s.endpoint != "https://"+host+"/v3/mg.example.test/messages" {
			t.Fatalf("unexpected API host for %q: %v", region, err)
		}
	}
	tests := []SecurityNoticeConfig{
		{Region: "UNEXPECTED", Domain: "mg.example.test", From: "security@mg.example.test", APIKey: "synthetic"},
		{Region: RegionEU, Domain: "../../evil", From: "security@mg.example.test", APIKey: "synthetic"},
		{Region: RegionEU, Domain: "mg.example.test", From: "attacker@another.test", APIKey: "synthetic"},
		{Region: RegionEU, Domain: "mg.example.test", From: "security@mg.example.test", APIKey: ""},
		{Region: RegionEU, Domain: "mg.example.test", From: "security@mg.example.test", APIKey: "injected\r\n"},
		{Region: RegionEU, Domain: "mg.example.test", From: "security@mg.example.test\r\nBCC:bad", APIKey: "synthetic"},
	}
	for _, invalid := range tests {
		if _, err := NewSecurityNoticeSender(invalid, nil); !errors.Is(err, ErrConfig) {
			t.Fatal("unsafe Mailgun sender configuration accepted")
		}
	}
	if _, err := NewSecurityNoticeSender(SecurityNoticeConfig{}, nil); !errors.Is(err, ErrConfig) {
		t.Fatal("empty sender configuration accepted")
	}
}

func TestSecurityNoticeMailgunStatusClassificationAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"accepted", 200, `{"id":"<synthetic@mailgun.test>"}`, nil},
		{"malformed acceptance", 200, `{"unexpected":"private URL"}`, usecase.ErrDeliveryUnavailable},
		{"bad request", 400, `{"message":"synthetic-mailgun-sending-key"}`, usecase.ErrPermanentDelivery},
		{"unauthorized", 401, "synthetic-mailgun-sending-key", usecase.ErrPermanentDelivery},
		{"forbidden", 403, "alice@example.test", usecase.ErrPermanentDelivery},
		{"redirect", 302, "bad redirect", usecase.ErrPermanentDelivery},
		{"timeout", 408, "temporary", usecase.ErrDeliveryUnavailable},
		{"limit legacy", 420, "temporary", usecase.ErrDeliveryUnavailable},
		{"rate limit", 429, "temporary", usecase.ErrDeliveryUnavailable},
		{"provider error", 503, "temporary", usecase.ErrDeliveryUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newMockSecuritySender(t, func(w http.ResponseWriter, r *http.Request) {
				if tc.status == 302 {
					w.Header().Set("Location", "https://attacker.example.test/collect")
				}
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})
			_, err := s.SendPasswordChanged(context.Background(), fixtureNoticeEvent, "alice@example.test")
			if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("status %d: unexpected result: %v", tc.status, err)
			}
			if err != nil && (strings.Contains(err.Error(), "synthetic-mailgun-sending-key") ||
				strings.Contains(err.Error(), "alice@example.test") ||
				strings.Contains(err.Error(), "attacker.example.test")) {
				t.Fatal("provider or recipient details reached caller")
			}
		})
	}
}

func TestSecurityNoticeSenderRejectsInvalidEventRecipientAndCancel(t *testing.T) {
	calls := 0
	s := newMockSecuritySender(t, func(w http.ResponseWriter, _ *http.Request) { calls++ })
	for _, tc := range []struct{ event, recipient string }{
		{"bad", "alice@example.test"},
		{"vch_AAAAAAAAAAAAAAAAAAAAAA", "alice@example.test"},
		{fixtureNoticeEvent, "Alice <alice@example.test>"},
		{fixtureNoticeEvent, "alice@example.test,bob@example.test"},
		{fixtureNoticeEvent, "alice@example.test\nBCC:bad"},
	} {
		if _, err := s.SendPasswordChanged(context.Background(), tc.event, tc.recipient); !errors.Is(err, usecase.ErrPermanentDelivery) {
			t.Fatal("invalid security notice reached sender")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.SendPasswordChanged(ctx, fixtureNoticeEvent, "alice@example.test"); !errors.Is(err, usecase.ErrPermanentDelivery) {
		t.Fatal("canceled send attempted HTTP")
	}
	if calls != 0 {
		t.Fatal("malformed notice triggered a network call")
	}
}

func TestSecurityNoticeSenderMockHTTPTimeoutIsRedactedRetryable(t *testing.T) {
	s := newMockSecuritySender(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.SendPasswordChanged(ctx, fixtureNoticeEvent, "alice@example.test"); !errors.Is(err, usecase.ErrDeliveryUnavailable) {
		t.Fatalf("timeout should be a sanitized retryable result: %v", err)
	}
}
