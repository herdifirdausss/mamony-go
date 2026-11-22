package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

type fakeNoticeWebhookRecorder struct {
	calls int
	seen  usecase.VerifiedSecurityNoticeReceipt
	err   error
}

func (f *fakeNoticeWebhookRecorder) Record(_ context.Context, item usecase.VerifiedSecurityNoticeReceipt) error {
	f.calls++
	f.seen = item
	return f.err
}

func syntheticMailgunSignedPayload(t *testing.T, key []byte, now time.Time,
	eventKind, severity, domain, messageID, eventID string) []byte {
	t.Helper()
	ts := strconv.FormatInt(now.Unix(), 10)
	token := strings.Repeat("a", 50)
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(ts + token))
	raw, err := json.Marshal(map[string]any{
		"signature": map[string]any{
			"timestamp": ts, "token": token,
			"signature": hex.EncodeToString(h.Sum(nil)),
		},
		"event-data": map[string]any{
			"event": eventKind, "severity": severity,
			"id": eventID, "timestamp": float64(now.Unix()),
			"domain": map[string]any{"name": domain},
			"message": map[string]any{
				"headers": map[string]any{"message-id": messageID},
			},
			"recipient": "sensitive-recipient@example.test",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func runSyntheticNoticeWebhook(t *testing.T, h http.Handler, body []byte) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "https://example.test/private-mailgun", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	return resp.Code
}

func TestMailgunNoticeWebhookAuthenticatesAndRedacts(t *testing.T) {
	key := []byte("synthetic-webhook-signing-key-not-mailgun-sending-key")
	recorder := &fakeNoticeWebhookRecorder{}
	handler, err := NewMailgunSecurityNoticeWebhook([][]byte{key}, "mg.example.test", recorder)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	p := syntheticMailgunSignedPayload(t, key, now, "failed", "permanent",
		"mg.example.test", "2026.synthetic@mailgun.test", "event-one")
	if got := runSyntheticNoticeWebhook(t, handler, p); got != http.StatusOK {
		t.Fatalf("valid signed Mailgun webhook denied: %d", got)
	}
	wantMessage := sha256.Sum256([]byte("2026.synthetic@mailgun.test"))
	wantToken := sha256.Sum256([]byte(strings.Repeat("a", 50)))
	if recorder.calls != 1 || recorder.seen.DeliveryState != "PERMANENT_FAILURE" ||
		recorder.seen.MessageDigest != wantMessage || recorder.seen.TokenDigest != wantToken {
		t.Fatal("verified Mailgun event was not reduced to PII-free identifiers")
	}
	if bytes.Contains(recorder.seen.MessageDigest[:], []byte("sensitive-recipient")) {
		t.Fatal("PII unexpectedly present in durable event digest")
	}
}

func TestMailgunNoticeWebhookDeniesForgedOrMalformedRequestsBeforeStorage(t *testing.T) {
	key := []byte("synthetic-webhook-signing-key-not-mailgun-sending-key")
	otherKey := []byte("synthetic-other-signing-key-not-mailgun-sending-key")
	now := time.Now().UTC()
	for _, tc := range []struct {
		name    string
		key     []byte
		kind    string
		severity string
		domain  string
		message string
		eventID string
		shift   time.Duration
	}{
		{name: "wrong signature", key: otherKey, kind: "delivered", domain: "mg.example.test",
			message: "id@mailgun.test", eventID: "evt-1"},
		{name: "future signature timestamp", key: key, kind: "delivered", domain: "mg.example.test",
			message: "id@mailgun.test", eventID: "evt-1", shift: time.Hour},
		{name: "stale signature timestamp", key: key, kind: "delivered", domain: "mg.example.test",
			message: "id@mailgun.test", eventID: "evt-1", shift: -10*time.Hour},
		{name: "wrong sending domain", key: key, kind: "delivered", domain: "evil.example.test",
			message: "id@mailgun.test", eventID: "evt-1"},
		{name: "malformed provider id", key: key, kind: "delivered", domain: "mg.example.test",
			message: "id\r\n@mailgun.test", eventID: "evt-1"},
		{name: "malformed event identifier", key: key, kind: "delivered", domain: "mg.example.test",
			message: "id@mailgun.test", eventID: "bad\r\n"},
		{name: "unknown fail severity", key: key, kind: "failed", severity: "guess",
			domain: "mg.example.test", message: "id@mailgun.test", eventID: "evt-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &fakeNoticeWebhookRecorder{}
			h, err := NewMailgunSecurityNoticeWebhook([][]byte{key}, "mg.example.test", rec)
			if err != nil {
				t.Fatal(err)
			}
			raw := syntheticMailgunSignedPayload(t, tc.key, now.Add(tc.shift),
				tc.kind, tc.severity, tc.domain, tc.message, tc.eventID)
			code := runSyntheticNoticeWebhook(t, h, raw)
			if tc.severity == "guess" {
				if code != http.StatusOK || rec.calls != 0 {
					t.Fatal("unsupported signed event must not change status")
				}
			} else if code != http.StatusNotAcceptable || rec.calls != 0 {
				t.Fatalf("forged or malformed webhook was persisted: %d %d", code, rec.calls)
			}
		})
	}
}

func TestMailgunNoticeWebhookStrictEnvelopeAndStorageFailures(t *testing.T) {
	key := []byte("synthetic-webhook-signing-key-not-mailgun-sending-key")
	h, err := NewMailgunSecurityNoticeWebhook([][]byte{key}, "mg.example.test",
		&fakeNoticeWebhookRecorder{err: errors.New("secret in SQL DSN")})
	if err != nil {
		t.Fatal(err)
	}
	body := syntheticMailgunSignedPayload(t, key, time.Now().UTC(),
		"complained", "", "mg.example.test", "id@mailgun.test", "evt-one")
	if got := runSyntheticNoticeWebhook(t, h, body); got != http.StatusServiceUnavailable {
		t.Fatal("transient SQL failure must request provider retry")
	}
	for _, tc := range []struct {
		name   string
		body   []byte
		method string
		typeHeader string
	}{
		{"oversized", bytes.Repeat([]byte("x"), 16385), http.MethodPost, "application/json"},
		{"missing JSON", []byte("{}"), http.MethodPost, "application/json"},
		{"wrong method", body, http.MethodGet, "application/json"},
		{"wrong content type", body, http.MethodPost, "text/plain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "https://example.test/private", bytes.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.typeHeader)
			out := httptest.NewRecorder()
			h.ServeHTTP(out, req)
			if out.Code != http.StatusNotAcceptable {
				t.Fatalf("malformed event was not denied: %d", out.Code)
			}
		})
	}
}

func TestMailgunNoticeWebhookSupportsExplicitSigningKeyRotation(t *testing.T) {
	primary := []byte("primary-webhook-signing-key-long-enough")
	previous := []byte("previous-webhook-signing-key-long-enough")
	rec := &fakeNoticeWebhookRecorder{}
	h, err := NewMailgunSecurityNoticeWebhook([][]byte{primary, previous},
		"mg.example.test", rec)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, key := range [][]byte{primary, previous} {
		raw := syntheticMailgunSignedPayload(t, key, now, "delivered", "",
			"mg.example.test", "<message@mailgun.test>", "event-123")
		if code := runSyntheticNoticeWebhook(t, h, raw); code != http.StatusOK {
			t.Fatalf("configured webhook signing key rejected: %d", code)
		}
	}
	if rec.calls != 2 {
		t.Fatal("signing-key rotation validation lost verified events")
	}
	if _, err := NewMailgunSecurityNoticeWebhook([][]byte{[]byte("short")},
		"mg.example.test", rec); err == nil {
		t.Fatal("insufficient signing key accepted")
	}
}
