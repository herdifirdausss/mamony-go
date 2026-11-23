package usecase

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
)

type syntheticFactorVerifier struct {
	evidence        RecoveryFactorEvidence
	found           bool
	readErr         error
	failErr         error
	completeErr     error
	completeAllowed bool
	burns           int
	approves        int
}

func (f *syntheticFactorVerifier) Read(_ context.Context, _ string) (RecoveryFactorEvidence, bool, error) {
	return f.evidence, f.found, f.readErr
}
func (f *syntheticFactorVerifier) Fail(_ context.Context, _ string) error {
	f.burns++
	return f.failErr
}
func (f *syntheticFactorVerifier) Complete(_ context.Context, _ string,
	key, digest [32]byte, event string) (bool, error) {
	f.approves++
	if key != f.evidence.PublicKey || digest == ([32]byte{}) || !strings.HasPrefix(event, "evt_") {
		return false, errors.New("invalid approval parameters")
	}
	return f.completeAllowed, f.completeErr
}

func TestRecoveryFactorEd25519PurposeAndRedaction(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [32]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		t.Fatal(err)
	}
	subject := "sub_AAAAAAAAAAAAAAAAAAAAAA"
	challenge := "fac_BBBBBBBBBBBBBBBBBBBBBB"
	fake := &syntheticFactorVerifier{
		evidence: RecoveryFactorEvidence{Subject: subject, Epoch: 2, Nonce: nonce},
		found:    true, completeAllowed: true,
	}
	copy(fake.evidence.PublicKey[:], pub)
	verifier, err := NewRecoveryFactorApprovalVerifier(fake)
	if err != nil {
		t.Fatal(err)
	}
	challengeData := RecoveryFactorChallenge{id: challenge, subject: subject, epoch: 2, nonce: nonce}
	message := challengeData.ForTrustedAuthenticator()
	if len(message) < 100 || !bytes.HasPrefix(message, []byte(recoveryFactorChallengeDomain)) {
		t.Fatal("challenge signature missing explicit domain and full account context")
	}
	sig := ed25519.Sign(priv, message)
	if !ed25519.Verify(pub, message, sig) {
		t.Fatal("test signing fixture invalid")
	}
	tampered := RecoveryFactorChallenge{id: challenge, subject: subject, epoch: 3, nonce: nonce}
	if ed25519.Verify(pub, tampered.ForTrustedAuthenticator(), sig) {
		t.Fatal("signature accepted another session epoch")
	}
	tampered = RecoveryFactorChallenge{id: "fac_CCCCCCCCCCCCCCCCCCCCCC", subject: subject, epoch: 2, nonce: nonce}
	if ed25519.Verify(pub, tampered.ForTrustedAuthenticator(), sig) {
		t.Fatal("signature accepted another challenge ID")
	}
	if got := recoveryFactorSignedPayload(challenge, subject, 0, nonce); got != nil {
		t.Fatal("zero epoch allowed signing")
	}
	if got := recoveryFactorSignedPayload("bad", subject, 2, nonce); got != nil {
		t.Fatal("malformed challenge ID allowed signing")
	}
	wrongKey := make([]byte, len(sig))
	copy(wrongKey, sig)
	wrongKey[0] ^= 0xff
	if _, err := verifier.Verify(context.Background(), challenge, wrongKey); !errors.Is(err, ErrRecoveryFactorDenied) || fake.burns != 1 || fake.approves != 0 {
		t.Fatal("invalid signature minted approval or skipped attempt accounting")
	}
	proof, err := verifier.Verify(context.Background(), challenge, sig)
	if err != nil || len(proof.ForTrustedEnrollment()) != 43 || fake.approves != 1 {
		t.Fatal("genuine independent factor signature did not create one-use approval")
	}
	if fmt.Sprintf("%+v %#v %v", proof, proof, proof) == proof.ForTrustedEnrollment() ||
		strings.Contains(fmt.Sprintf("%+v %#v %v", proof, proof, proof), proof.ForTrustedEnrollment()) {
		t.Fatal("factor approval leaked in fmt formatting")
	}
	if proof.LogValue().Kind() != slog.KindString {
		t.Fatal("approval slog redaction missing")
	}
	if raw, err := json.Marshal(proof); err == nil || len(raw) != 0 {
		t.Fatal("independent approval serialized")
	}
	if _, err := proof.MarshalText(); err == nil {
		t.Fatal("independent approval converted to plaintext")
	}
	fake.completeErr = errors.New("sensitive postgres DSN")
	if _, err := verifier.Verify(context.Background(), challenge, sig); !errors.Is(err, ErrRecoveryFactorUnavailable) ||
		err.Error() != ErrRecoveryFactorUnavailable.Error() {
		t.Fatal("ambiguous SQL failure leaked an error or returned a proof")
	}
	fake.completeErr = nil
	fake.completeAllowed = false
	if _, err := verifier.Verify(context.Background(), challenge, sig); !errors.Is(err, ErrRecoveryFactorDenied) {
		t.Fatal("uncommitted SQL proof returned as approved")
	}
}

func TestRecoveryFactorDenialsAreFailClosed(t *testing.T) {
	if _, err := NewRecoveryFactorApprovalVerifier(nil); err == nil {
		t.Fatal("nil factor verifier allowed")
	}
	var nonce [32]byte
	nonce[0] = 1
	fake := &syntheticFactorVerifier{found: true, evidence: RecoveryFactorEvidence{
		Subject: "sub_AAAAAAAAAAAAAAAAAAAAAA", Epoch: 2, Nonce: nonce,
	}}
	v, err := NewRecoveryFactorApprovalVerifier(fake)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(context.Background(), "fac_AAAAAAAAAAAAAAAAAAAAAA", nil); !errors.Is(err, ErrRecoveryFactorDenied) || fake.burns != 1 {
		t.Fatal("empty signature did not charge an attempt")
	}
	fake.failErr = errors.New("failed attempt not durable")
	if _, err := v.Verify(context.Background(), "fac_AAAAAAAAAAAAAAAAAAAAAA", nil); !errors.Is(err, ErrRecoveryFactorUnavailable) {
		t.Fatal("failed attempt SQL error ignored")
	}
	fake.failErr = nil
	fake.found = false
	if _, err := v.Verify(context.Background(), "fac_AAAAAAAAAAAAAAAAAAAAAA", nil); !errors.Is(err, ErrRecoveryFactorDenied) {
		t.Fatal("missing factor challenge allowed")
	}
	fake.found = true
	v.random = io.LimitReader(bytes.NewReader(nil), 1)
	// Broken entropy cannot create an approval even with an otherwise
	// valid 64-byte signature; fake evidence does not authorize zero-key.
	if _, err := v.Verify(context.Background(), "", make([]byte, 64)); !errors.Is(err, ErrRecoveryFactorDenied) {
		t.Fatal("malformed challenge ID not denied")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := v.Verify(ctx, "fac_AAAAAAAAAAAAAAAAAAAAAA", nil); !errors.Is(err, ErrRecoveryFactorUnavailable) {
		t.Fatal("cancelled verifier did not fail closed")
	}
}
