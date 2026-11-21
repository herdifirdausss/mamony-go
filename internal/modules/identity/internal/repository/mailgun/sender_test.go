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

const sampleSecret = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const sampleChallenge = "vch_AAAAAAAAAAAAAAAAAAAAAA"

type redirectToMock struct {
	target *url.URL
	base   http.RoundTripper
}

func (r redirectToMock) RoundTrip(in *http.Request) (*http.Response, error) {
	c := in.Clone(in.Context())
	u := *in.URL
	if u.Scheme != "https" ||
		(u.Host != "api.mailgun.net" && u.Host != "api.eu.mailgun.net" && u.Host != "api.au.mailgun.net") {
		return nil, errors.New("not a supported Mailgun URL")
	}
	u.Scheme = r.target.Scheme
	u.Host = r.target.Host
	c.URL = &u
	c.Host = r.target.Host
	return r.base.RoundTrip(c)
}

func testSender(t *testing.T, host string, handler http.HandlerFunc) *Sender {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Transport: redirectToMock{target: u, base: http.DefaultTransport},
		Timeout:   600 * time.Millisecond,
	}
	sender, err := NewSender(Config{
		Region:          RegionUS,
		Domain:          "mg.example.test",
		From:            "Mamony Security <security@mg.example.test>",
		APIKey:          "synthetic-mailgun-secret",
		VerificationURL: "https://" + host + "/verify-email",
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	return sender
}

func TestMailgunSendsSingleTextOnlyVerificationWithMockHTTP(t *testing.T) {
	calls := 0
	s := testSender(t, "app.example.test", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v3/mg.example.test/messages" {
			t.Errorf("wrong Mailgun send endpoint: %s %s", r.Method, r.URL.Path)
		}
		username, key, ok := r.BasicAuth()
		if !ok || username != "api" || key != "synthetic-mailgun-secret" {
			t.Error("Basic Auth must use domain-sending key")
		}
		if r.Header.Get("Accept") != "application/json" || !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
			t.Error("unexpected request content type")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		reader, err := r.MultipartReader()
		if err != nil {
			t.Errorf("open multipart reader: %v", err)
			return
		}
		form := make(map[string][]string)
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Errorf("read multipart part: %v", err)
				return
			}
			value, readErr := io.ReadAll(io.LimitReader(part, 4097))
			_ = part.Close()
			if readErr != nil || len(value) > 4096 {
				t.Error("oversized or invalid multipart field")
				return
			}
			form[part.FormName()] = append(form[part.FormName()], string(value))
		}
		want := map[string]string{
			"from":              "Mamony Security <security@mg.example.test>",
			"to":                "alice@example.test",
			"subject":           "Verify your Mamony email",
			"o:tracking":        "no",
			"o:tracking-clicks": "no",
			"o:tracking-opens":  "no",
			"o:require-tls":     "yes",
			"o:deliver-within":  "5m",
		}
		for k, expected := range want {
			if len(form[k]) != 1 || form[k][0] != expected {
				t.Errorf("form field %q differs", k)
			}
		}
		if len(form["text"]) != 1 || !strings.Contains(form["text"][0], "https://app.example.test/verify-email#token="+sampleSecret) {
			t.Error("verification URL not in text body")
		}
		if strings.Contains(form["text"][0], "?token=") || strings.Contains(form["text"][0], "http://") {
			t.Error("proof must remain in HTTPS URL fragment")
		}
		for _, k := range []string{"html", "bcc", "cc", "v:my-var", "o:tracking-pixel-location-top", "o:inbox-placement-test-key"} {
			if _, ok := form[k]; ok {
				t.Errorf("unexpected metadata or tracking field %s", k)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"<fake@mailgun.test>","message":"Queued. Thank you."}`)
	})
	if err := s.SendVerification(context.Background(), sampleChallenge, "alice@example.test", sampleSecret); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one HTTP request: %d", calls)
	}
}

func TestMailgunRegionAndConfigAreStrict(t *testing.T) {
	for region, expected := range map[Region]string{
		RegionUS: "https://api.mailgun.net/v3/mg.example.test/messages",
		RegionEU: "https://api.eu.mailgun.net/v3/mg.example.test/messages",
		RegionAU: "https://api.au.mailgun.net/v3/mg.example.test/messages",
	} {
		s, err := NewSender(Config{
			Region:          region,
			Domain:          "mg.example.test",
			From:            "security@mg.example.test",
			APIKey:          "synthetic-key",
			VerificationURL: "https://app.example.test/verify-email",
		}, nil)
		if err != nil || s.endpoint != expected {
			t.Fatalf("region %q: endpoint=%q err=%v", region, s.endpoint, err)
		}
	}
	for _, tc := range []struct {
		domain, from, key, verify string
		region                    Region
	}{
		{"mg.example.test", "security@mg.example.test", "", "https://app.example.test/verify-email", RegionUS},
		{"mg.example.test", "security@mg.example.test", "fake", "http://app.example.test/verify-email", RegionUS},
		{"mg.example.test", "security@mg.example.test", "fake", "https://app.example.test/verify-email?token=x", RegionUS},
		{"mg.example.test", "security@mg.example.test", "fake", "https://attacker@app.example.test/verify-email", RegionUS},
		{"mg.example.test", "security@mg.example.test", "fake", "https://localhost/verify-email", RegionUS},
		{"mg.example.test", "security@mg.example.test", "fake", "https://app.example.test:8443/verify-email", RegionUS},
		{"mg.example.test", "security@evil.example.test", "fake", "https://app.example.test/verify-email", RegionUS},
		{"mg.example.test", "security@mg.example.test", "fake", "https://app.example.test/verify-email", Region("invalid")},
		{"../../bad", "security@mg.example.test", "fake", "https://app.example.test/verify-email", RegionUS},
		{"mg.example.test", "security@mg.example.test", "fake\nheader:value", "https://app.example.test/verify-email", RegionUS},
	} {
		_, err := NewSender(Config{Region: tc.region, Domain: tc.domain, From: tc.from, APIKey: tc.key, VerificationURL: tc.verify}, nil)
		if !errors.Is(err, ErrConfig) {
			t.Fatalf("untrusted config accepted: region=%q domain=%q", tc.region, tc.domain)
		}
	}
}

func TestMailgunMockProviderStatusClassificationAndSecretsRedaction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"accepted", 200, `{"id":"<fake@mailgun.test>"}`, nil},
		{"bad accepted body", 200, `{"unexpected":"secret"}`, usecase.ErrDeliveryUnavailable},
		{"validation", 400, `{"message":"` + sampleSecret + `"}`, usecase.ErrPermanentDelivery},
		{"unauthorized", 401, "synthetic-mailgun-secret", usecase.ErrPermanentDelivery},
		{"forbidden", 403, "synthetic-mailgun-secret", usecase.ErrPermanentDelivery},
		{"redirect", 302, "bad redirect", usecase.ErrPermanentDelivery},
		{"recipient rate", 420, "throttled", usecase.ErrDeliveryUnavailable},
		{"timeout", 408, "timeout", usecase.ErrDeliveryUnavailable},
		{"rate", 429, "rate-limited", usecase.ErrDeliveryUnavailable},
		{"server", 503, "backend error", usecase.ErrDeliveryUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testSender(t, "app.example.test", func(w http.ResponseWriter, r *http.Request) {
				if tc.status == 302 {
					w.Header().Set("Location", "https://attacker.example.test/collect")
				}
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})
			err := s.SendVerification(context.Background(), sampleChallenge, "alice@example.test", sampleSecret)
			if tc.want == nil && err != nil {
				t.Fatal(err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("status %d: got %v want %v", tc.status, err, tc.want)
			}
			if err != nil && (strings.Contains(err.Error(), sampleSecret) || strings.Contains(err.Error(), "synthetic-mailgun-secret")) {
				t.Fatal("provider secret leaked in error")
			}
		})
	}
}

func TestMailgunRejectsInvalidRecipientTokenAndExpiredContext(t *testing.T) {
	calls := 0
	s := testSender(t, "app.example.test", func(w http.ResponseWriter, _ *http.Request) { calls++ })
	for _, tc := range []struct{ id, to, token string }{
		{"bad", "alice@example.test", sampleSecret},
		{sampleChallenge, "Alice <alice@example.test>", sampleSecret},
		{sampleChallenge, "a@example.test,b@example.test", sampleSecret},
		{sampleChallenge, "alice@example.test", "invalid"},
	} {
		if err := s.SendVerification(context.Background(), tc.id, tc.to, tc.token); !errors.Is(err, usecase.ErrPermanentDelivery) {
			t.Fatalf("invalid payload was sent: %v", err)
		}
	}
	if calls != 0 {
		t.Fatal("malformed payload reached Mailgun")
	}
}

func TestMailgunMockHTTPTimeoutIsRetryable(t *testing.T) {
	s := testSender(t, "app.example.test", func(w http.ResponseWriter, _ *http.Request) {
		// Bounded delay prevents a mock server handler from hanging forever
		// if the test transport does not propagate client cancellation.
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.SendVerification(ctx, sampleChallenge, "alice@example.test", sampleSecret); !errors.Is(err, usecase.ErrDeliveryUnavailable) {
		t.Fatalf("timeout should be retryable: %v", err)
	}
}
