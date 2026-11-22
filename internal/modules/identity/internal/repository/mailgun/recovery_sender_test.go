package mailgun

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

func testRecoverySender(t *testing.T, handler http.HandlerFunc) *RecoveryMailSender {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := NewRecoveryMailSender(RecoveryConfig{
		Region: RegionUS, Domain: "mg.example.test",
		From: "Mamony Security <security@mg.example.test>",
		APIKey: "synthetic-recovery-mailgun",
		RecoveryURL: "https://app.example.test/account/recover-password",
	}, &http.Client{
		Timeout:   1 * time.Second,
		Transport: redirectToMock{target: u, base: http.DefaultTransport},
	})
	if err != nil {
		t.Fatal(err)
	}
	return sender
}

func TestRecoveryMailSenderFragmentAndClassification(t *testing.T) {
	calls := 0
	status := 429
	s := testRecoverySender(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		who, key, ok := r.BasicAuth()
		if !ok || who != "api" || key != "synthetic-recovery-mailgun" ||
			r.Method != http.MethodPost || r.URL.Path != "/v3/mg.example.test/messages" {
			t.Error("recovery sender attempted an unapproved provider route")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 12<<10)
		if err := r.ParseMultipartForm(8192); err != nil {
			t.Error(err)
			return
		}
		if r.FormValue("to") != "owner@example.test" ||
			r.FormValue("subject") != "Mamony account password recovery" ||
			r.FormValue("o:tracking") != "no" ||
			r.FormValue("o:tracking-opens") != "no" ||
			r.FormValue("o:tracking-clicks") != "no" ||
			r.FormValue("o:require-tls") != "yes" ||
			r.FormValue("o:deliver-within") != "5m" ||
			!strings.Contains(r.FormValue("text"), "https://app.example.test/account/recover-password#token="+sampleSecret) ||
			strings.Contains(r.FormValue("text"), "?token=") {
			t.Error("recovery sender leaked proof in URL query or omitted controls")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == 200 {
			_, _ = fmt.Fprint(w, `{"id":"<synthetic@mailgun.test>"}`)
		} else {
			_, _ = fmt.Fprint(w, `{"message":"private-credential-leak-must-never-escape"}`)
		}
	})
	id := "rcv_AAAAAAAAAAAAAAAAAAAAAA"
	if err := s.SendRecovery(context.Background(), id, "owner@example.test", sampleSecret); !errors.Is(err, usecase.ErrDeliveryUnavailable) {
		t.Fatal("provider 429 must be a redacted retryable error")
	}
	status = 200
	if err := s.SendRecovery(context.Background(), id, "owner@example.test", sampleSecret); err != nil {
		t.Fatal(err)
	}
	status = 400
	if err := s.SendRecovery(context.Background(), id, "owner@example.test", sampleSecret); !errors.Is(err, usecase.ErrPermanentDelivery) {
		t.Fatal("provider 400 must not be automatically resent")
	}
	if calls != 3 {
		t.Fatal("recovery sender made unexpected HTTP requests")
	}
	if err := s.SendRecovery(context.Background(), "vch_AAAAAAAAAAAAAAAAAAAAAA", "owner@example.test", sampleSecret); !errors.Is(err, usecase.ErrPermanentDelivery) {
		t.Fatal("email-verification proof used as recovery challenge")
	}
	if err := s.SendRecovery(context.Background(), id, "evil\r\nBcc:attacker@example.test", sampleSecret); !errors.Is(err, usecase.ErrPermanentDelivery) {
		t.Fatal("invalid recipient accepted")
	}
	if calls != 3 {
		t.Fatal("malformed recipient/challenge reached provider")
	}
}

func TestRecoveryMailSenderRejectsInvalidConfig(t *testing.T) {
	base := RecoveryConfig{
		Region: RegionUS, Domain: "mg.example.test",
		From: "security@mg.example.test", APIKey: "synthetic-recovery-key",
		RecoveryURL: "https://app.example.test/account/recover-password",
	}
	bad := []string{
		"http://app.example.test/account/recover-password",
		"https://app.example.test/account/recover-password?token=fake",
		"https://app.example.test/account/recover-password#token=fake",
		"https://evil@app.example.test/account/recover-password",
		"https://app.example.test/verify-email",
		"https://app.example.test/account/recover-password/",
		"https://app.example.test:8443/account/recover-password",
		"https://localhost/account/recover-password",
	}
	for _, value := range bad {
		cfg := base
		cfg.RecoveryURL = value
		if _, err := NewRecoveryMailSender(cfg, nil); !errors.Is(err, ErrConfig) {
			t.Fatalf("unsafe recovery target accepted: %s", value)
		}
	}
	cfg := base
	cfg.APIKey = ""
	if _, err := NewRecoveryMailSender(cfg, nil); !errors.Is(err, ErrConfig) {
		t.Fatal("missing Mailgun sending key accepted")
	}
}
