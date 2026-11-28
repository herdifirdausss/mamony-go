package mailgunwebhook

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// Recovery callback always uses the existing verified signature parser,
// but a different constructor, recorder and SQL credential. This regression
// intentionally asserts signed, bad-signer, bad-domain and key-rotation paths.
func TestMailgunRecoveryNoticeWebhookSignatureAndRotation(t *testing.T) {
	active := []byte("active-recovery-webhook-signing-fixture-key")
	retained := []byte("retained-recovery-webhook-signing-fixture-key")
	recorder := &fakeNoticeWebhookRecorder{}
	webhook, err := NewPasswordRecoveryNotice(
		[][]byte{active, retained}, "mg.example.test", recorder)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, key := range [][]byte{active, retained} {
		payload := syntheticMailgunSignedPayload(t, key, now, "delivered", "",
			"mg.example.test", "recovery-message@mailgun.test", "recovery-webhook")
		if got := runSyntheticNoticeWebhook(t, webhook, payload); got != http.StatusOK {
			t.Fatalf("configured recovery signing key was rejected: %d", got)
		}
	}
	if recorder.calls != 2 || recorder.seen.DeliveryState != "DELIVERED" {
		t.Fatal("recovery signer did not forward verified digest-only event")
	}
	forged := syntheticMailgunSignedPayload(t, []byte("invalid-recovery-signer-key-material"),
		now, "complained", "", "mg.example.test",
		"recovery-message@mailgun.test", "recovery-forged")
	if got := runSyntheticNoticeWebhook(t, webhook, forged); got != http.StatusNotAcceptable {
		t.Fatal("invalid recovery webhook signature accepted")
	}
	wrongDomain := syntheticMailgunSignedPayload(t, active, now, "failed", "permanent",
		"evil.example.test", "recovery-message@mailgun.test", "wrong-domain")
	if got := runSyntheticNoticeWebhook(t, webhook, wrongDomain); got != http.StatusNotAcceptable {
		t.Fatal("recovery webhook accepted a callback from wrong domain")
	}
	unknownEvent := syntheticMailgunSignedPayload(t, active, now, "clicked", "",
		"mg.example.test", "recovery-message@mailgun.test", "tracking-only")
	if got := runSyntheticNoticeWebhook(t, webhook, unknownEvent); got != http.StatusOK ||
		recorder.calls != 2 {
		t.Fatal("recovery webhook recorded an unrelated tracking event")
	}
	stale := syntheticMailgunSignedPayload(t, active, now.Add(-10*time.Hour),
		"delivered", "", "mg.example.test", "recovery-message@mailgun.test", "stale")
	if got := runSyntheticNoticeWebhook(t, webhook, stale); got != http.StatusNotAcceptable {
		t.Fatal("stale recovery callback timestamp was accepted")
	}
	if recorder.calls != 2 {
		t.Fatal("unverified callback reached recovery receipt recorder")
	}
	if _, err := NewPasswordRecoveryNotice(nil, "mg.example.test", recorder); err == nil {
		t.Fatal("recovery handler accepted missing signature verification keys")
	}
	if _, err := NewPasswordRecoveryNotice([][]byte{active}, "mg.example.test", nil); err == nil {
		t.Fatal("recovery handler accepted missing restricted recorder")
	}
	if strings.Contains(recorder.seen.DeliveryState, "sensitive") {
		t.Fatal("recipient-derived state was persisted")
	}
}
