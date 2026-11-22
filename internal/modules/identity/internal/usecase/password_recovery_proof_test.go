package usecase

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestPasswordRecoveryProofRandomAndDomainSeparated(t *testing.T) {
	issued, err := NewPasswordRecoveryProof()
	if err != nil {
		t.Fatal(err)
	}
	raw := issued.ForTrustedDelivery()
	if len(raw) != passwordRecoveryProofLength {
		t.Fatal("password recovery bearer has the wrong encoding length")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) != passwordRecoveryProofBytes ||
		base64.RawURLEncoding.EncodeToString(decoded) != raw {
		t.Fatal("password recovery bearer is not canonical 256-bit base64url")
	}
	digest, err := PasswordRecoveryProofDigest(raw)
	if err != nil || digest != issued.Digest() {
		t.Fatal("issued proof cannot be deterministically digested for later verification")
	}
	if digest == sha256.Sum256(decoded) {
		t.Fatal("recovery digest must be domain separated from a raw SHA-256")
	}
	if bytes.Equal(digest[:], tokenDigest(raw)) {
		t.Fatal("recovery token must never verify as a registration/email proof")
	}
	sample, err := NewPasswordRecoveryProof()
	if err != nil || sample.Digest() == digest || sample.ForTrustedDelivery() == raw {
		t.Fatal("two freshly issued recovery proofs collided")
	}
}

func TestPasswordRecoveryProofDigestStrictInput(t *testing.T) {
	proof, err := newPasswordRecoveryProof(bytes.NewReader(bytes.Repeat([]byte{0xb4}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	good := proof.ForTrustedDelivery()
	for _, tc := range []string{
		"", "a", good[:42], good + "a", good + "=", " " + good,
		strings.Replace(good, "t", "+", 1), strings.Repeat("?", 43),
		strings.Repeat("a", 43), strings.Repeat("-", 43),
	} {
		if tc == good {
			continue
		}
		got, err := PasswordRecoveryProofDigest(tc)
		if !errors.Is(err, ErrPasswordRecoveryProofInvalid) || got != ([32]byte{}) {
			t.Fatal("noncanonical or malformed proof was accepted")
		}
	}
	matched, err := PasswordRecoveryProofDigest(good)
	if err != nil || matched != proof.Digest() {
		t.Fatal("canonical deterministic proof was rejected")
	}
	if _, err := PasswordRecoveryProofDigest(base64.RawURLEncoding.EncodeToString(make([]byte, 31))); err == nil {
		t.Fatal("a truncated bearer was accepted")
	}
}

func TestPasswordRecoveryProofFailsClosedOnEntropyError(t *testing.T) {
	for _, source := range []*bytes.Reader{nil, bytes.NewReader(nil), bytes.NewReader(bytes.Repeat([]byte{0x44}, 31))} {
		proof, err := newPasswordRecoveryProof(source)
		if !errors.Is(err, ErrPasswordRecoveryProofUnavailable) ||
			proof.ForTrustedDelivery() != "" || proof.Digest() != ([32]byte{}) {
			t.Fatal("incomplete entropy returned a deliverable recovery bearer")
		}
	}
}

func TestPasswordRecoveryProofRedactsLoggingAndSerialization(t *testing.T) {
	p, err := NewPasswordRecoveryProof()
	if err != nil {
		t.Fatal(err)
	}
	raw := p.ForTrustedDelivery()
	for _, value := range []string{
		fmt.Sprintf("%v", p),
		fmt.Sprintf("%+v", p),
		fmt.Sprintf("%#v", p),
		fmt.Sprintf("%s", p),
		fmt.Sprintf("%q", p),
		fmt.Sprintf("%x", p),
		p.String(),
		p.GoString(),
		p.LogValue().String(),
	} {
		if !strings.Contains(value, redactedPasswordRecoveryProof) ||
			strings.Contains(value, raw) {
			t.Fatal("password recovery proof leaked through ordinary formatting")
		}
	}
	if marshaled, err := json.Marshal(p); err == nil || bytes.Contains(marshaled, []byte(raw)) {
		t.Fatal("password recovery bearer was JSON-serializable")
	}
	if marshaled, err := p.MarshalText(); err == nil || bytes.Contains(marshaled, []byte(raw)) {
		t.Fatal("password recovery bearer was text-serializable")
	}
	var record bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&record, nil))
	logger.Info("password recovery", "proof", p)
	if strings.Contains(record.String(), raw) ||
		!strings.Contains(record.String(), redactedPasswordRecoveryProof) {
		t.Fatal("structured slog logging leaked the recovery bearer")
	}
}
