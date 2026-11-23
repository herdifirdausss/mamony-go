package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

// MailgunSecurityNoticeWebhook is an UNMOUNTED handler candidate. Only a
// separately reviewed, TLS-terminated, rate-limited webhook ingress may mount
// it. Never share the Mailgun Sending API key as the webhook signing key.
type MailgunSecurityNoticeWebhook struct {
	keys     [][]byte
	domain   string
	recorder usecase.SecurityNoticeWebhookRecorder
	clock    func() time.Time
}

func NewMailgunSecurityNoticeWebhook(keys [][]byte, domain string, recorder usecase.SecurityNoticeWebhookRecorder) (*MailgunSecurityNoticeWebhook, error) {
	if recorder == nil || len(keys) == 0 || len(keys) > 2 ||
		domain == "" || domain != strings.ToLower(domain) ||
		len(domain) > 253 || strings.ContainsAny(domain, " /\\\r\n") {
		return nil, usecase.ErrDeliveryUnavailable
	}
	copied := make([][]byte, 0, len(keys))
	for _, key := range keys {
		if len(key) < 16 || len(key) > 256 {
			return nil, usecase.ErrDeliveryUnavailable
		}
		copied = append(copied, append([]byte(nil), key...))
	}
	return &MailgunSecurityNoticeWebhook{
		keys: copied, domain: domain, recorder: recorder, clock: time.Now,
	}, nil
}

type mailgunNoticeEnvelope struct {
	Signature struct {
		Timestamp string `json:"timestamp"`
		Token     string `json:"token"`
		Signature string `json:"signature"`
	} `json:"signature"`
	Event struct {
		Event     string      `json:"event"`
		ID        string      `json:"id"`
		Timestamp json.Number `json:"timestamp"`
		Severity  string      `json:"severity"`
		Domain    struct {
			Name string `json:"name"`
		} `json:"domain"`
		Message struct {
			Headers struct {
				MessageID string `json:"message-id"`
			} `json:"headers"`
		} `json:"message"`
	} `json:"event-data"`
}

func validWebhookHex(raw string, length int) bool {
	if len(raw) != length {
		return false
	}
	for _, c := range raw {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' ||
			c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func (h *MailgunSecurityNoticeWebhook) validSignature(sig mailgunNoticeEnvelope) bool {
	ts := sig.Signature.Timestamp
	token := sig.Signature.Token
	raw := sig.Signature.Signature
	if !validWebhookHex(token, 50) || !validWebhookHex(raw, 64) ||
		len(ts) < 10 || len(ts) > 12 || strings.Trim(ts, "0123456789") != "" {
		return false
	}
	seconds, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || h.clock == nil {
		return false
	}
	// Mailgun can retry for up to eight hours. Allow nine hours of past
	// clock skew, with durable token/event dedup to prevent signed replays.
	now := h.clock().UTC()
	stamp := time.Unix(seconds, 0)
	if stamp.Before(now.Add(-9*time.Hour)) || stamp.After(now.Add(15*time.Minute)) {
		return false
	}
	given, err := hex.DecodeString(raw)
	if err != nil {
		return false
	}
	match := false
	for _, key := range h.keys {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(ts + token))
		if hmac.Equal(given, mac.Sum(nil)) {
			match = true
		}
	}
	return match
}

func classifyMailgunNoticeEvent(event, severity string) (string, bool) {
	switch event {
	case "delivered":
		return "DELIVERED", true
	case "failed":
		switch severity {
		case "temporary":
			return "TEMPORARY_FAILURE", true
		case "permanent":
			return "PERMANENT_FAILURE", true
		}
	case "complained":
		return "COMPLAINED", true
	case "unsubscribed":
		return "UNSUBSCRIBED", true
	}
	return "", false
}

// ServeHTTP is never mounted by the Identity API router. Bad requests receive
// a generic 406 (Mailgun does not retry); only transient DB failures return
// 503 so Mailgun can retry. No provider PII/body is copied into logs or errors.
func (h *MailgunSecurityNoticeWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.recorder == nil || h.clock == nil || r == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16385))
	if err != nil || len(body) == 0 || len(body) > 16384 {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}
	var payload mailgunNoticeEnvelope
	if err := json.Unmarshal(body, &payload); err != nil || !h.validSignature(payload) {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}
	state, recognized := classifyMailgunNoticeEvent(payload.Event.Event, payload.Event.Severity)
	if !recognized {
		// No tracking/click data is needed; do not persist unrelated events.
		w.WriteHeader(http.StatusOK)
		return
	}
	if payload.Event.Domain.Name != h.domain ||
		len(payload.Event.ID) < 3 || len(payload.Event.ID) > 128 ||
		strings.ContainsAny(payload.Event.ID, "\r\n\t") {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}
	message, ok := usecase.NormalizeMailgunMessageID(payload.Event.Message.Headers.MessageID)
	if !ok {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}
	unixSeconds, err := strconv.ParseFloat(string(payload.Event.Timestamp), 64)
	if err != nil || math.IsNaN(unixSeconds) || math.IsInf(unixSeconds, 0) ||
		unixSeconds < 0 || unixSeconds > float64(h.clock().Unix()+15*60) {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}
	eventAt := time.Unix(int64(unixSeconds), 0).UTC()
	if eventAt.Before(h.clock().Add(-9 * time.Hour)) {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}
	tokenDigest := sha256.Sum256([]byte(payload.Signature.Token))
	eventDigest := sha256.Sum256([]byte(h.domain + "\x00" +
		eventAt.Format("2006-01-02") + "\x00" + payload.Event.ID))
	messageDigest := sha256.Sum256([]byte(message))
	if err := h.recorder.Record(r.Context(), usecase.VerifiedSecurityNoticeReceipt{
		TokenDigest: tokenDigest, EventDigest: eventDigest,
		MessageDigest: messageDigest, DeliveryState: state, EventAt: eventAt,
	}); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Implement the net/http handler contract without activating a route.
var _ http.Handler = (*MailgunSecurityNoticeWebhook)(nil)
