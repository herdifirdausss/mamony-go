package webauthn

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

// Every positive test constructs a genuine ES256 signature over
// authenticatorData || SHA256(clientDataJSON), using a freshly generated
// P-256 TEST key and serializes that key as a COSE EC2 public key.
// This is never a copied fixture or a mock cryptographic verifier.
type assertionFixture struct {
	challenge [32]byte
	rpID string
	origin string
	ceremonyType string
	flags byte
	counter uint32
	id []byte
	userHandle []byte
	crossOrigin bool
	tamperSignature bool
}

func syntheticSignedAssertion(t *testing.T, key *ecdsa.PrivateKey,
	f assertionFixture) ([]byte, usecase.PasskeyActivationEvidence) {
	t.Helper()
	if f.challenge == ([32]byte{}) {
		f.challenge = [32]byte{1, 2, 3, 4}
	}
	if f.rpID == "" {
		f.rpID = "login.example.test"
	}
	if f.origin == "" {
		f.origin = "https://login.example.test"
	}
	if f.ceremonyType == "" {
		f.ceremonyType = "webauthn.get"
	}
	if f.flags == 0 {
		f.flags = 0x05 // UP + UV, unbacked
	}
	if f.id == nil {
		f.id = bytes.Repeat([]byte{0x61}, 32)
	}
	pub := elliptic.Marshal(elliptic.P256(), key.PublicKey.X, key.PublicKey.Y)
	cose, err := cbor.Marshal(map[int]any{
		1: 2, 3: -7, -1: 1, -2: pub[1:33], -3: pub[33:65],
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := json.Marshal(map[string]any{
		"type":        f.ceremonyType,
		"challenge":   base64.RawURLEncoding.EncodeToString(f.challenge[:]),
		"origin":      f.origin,
		"crossOrigin": f.crossOrigin,
	})
	if err != nil {
		t.Fatal(err)
	}
	rpHash := sha256.Sum256([]byte(f.rpID))
	authData := append([]byte(nil), rpHash[:]...)
	authData = append(authData, f.flags)
	var count [4]byte
	binary.BigEndian.PutUint32(count[:], f.counter)
	authData = append(authData, count[:]...)
	clientHash := sha256.Sum256(client)
	signedBytes := append(append([]byte(nil), authData...), clientHash[:]...)
	digest := sha256.Sum256(signedBytes)
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	if f.tamperSignature {
		sig[len(sig)-1] ^= 0x01
	}
	encodedID := base64.RawURLEncoding.EncodeToString(f.id)
	response, err := json.Marshal(map[string]any{
		"id": encodedID, "rawId": encodedID, "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    base64.RawURLEncoding.EncodeToString(client),
			"authenticatorData": base64.RawURLEncoding.EncodeToString(authData),
			"signature":         base64.RawURLEncoding.EncodeToString(sig),
			"userHandle":        base64.RawURLEncoding.EncodeToString(f.userHandle),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := usecase.PasskeyActivationEvidence{
		Subject:          "sub_AAAAAAAAAAAAAAAAAAAAAA",
		Epoch:            2,
		Nonce:            f.challenge,
		CredentialID:     append([]byte(nil), f.id...),
		PublicKeyCOSE:    cose,
		RPID:             "login.example.test",
		Origin:           "https://login.example.test",
		PolicyGeneration: 1,
	}
	if f.flags&0x08 != 0 {
		expected.BackupEligible = true
	}
	if f.flags&0x10 != 0 {
		expected.BackupState = true
	}
	return response, expected
}

func assertionVerifier(t *testing.T) *ActivationAssertionVerifier {
	t.Helper()
	policy, err := usecase.NewPasskeyRelyingParty(
		"login.example.test", "https://login.example.test")
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewActivationAssertionVerifier(policy)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func signedTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestPasskeyActivationAssertionVerifiesRealES256Signature(t *testing.T) {
	t.Parallel()
	key := signedTestKey(t)
	v := assertionVerifier(t)
	payload, expected := syntheticSignedAssertion(t, key, assertionFixture{
		flags: 0x05, counter: 1,
		userHandle: []byte("sub_AAAAAAAAAAAAAAAAAAAAAA"),
	})
	result, err := v.VerifyAssertion(context.Background(), expected, payload)
	if err != nil || !bytes.Equal(result.CredentialID, expected.CredentialID) ||
		!result.UserPresent || !result.UserVerified ||
		result.BackupEligible || result.BackupState || result.Counter != 1 {
		t.Fatalf("valid independently signed ES256 assertion rejected: %v %+v", err, result)
	}
	// Passkeys with naturally zero counters are permitted; only monotonically
	// detectable regressions/clone-warning are fatal.
	payloadZero, expectedZero := syntheticSignedAssertion(t, key, assertionFixture{
		flags: 0x05, counter: 0,
	})
	result, err = v.VerifyAssertion(context.Background(), expectedZero, payloadZero)
	if err != nil || result.Counter != 0 {
		t.Fatalf("zero-zero synced passkey counter improperly rejected: %v", err)
	}
	// A backup-eligible signed assertion is allowed when BE was registered.
	payloadBackup, expectedBackup := syntheticSignedAssertion(t, key, assertionFixture{
		flags: 0x1d, counter: 4, // UP+UV+BE+BS
	})
	expectedBackup.BackupState = false // BS may legitimately transition.
	result, err = v.VerifyAssertion(context.Background(), expectedBackup, payloadBackup)
	if err != nil || !result.BackupEligible || !result.BackupState || result.Counter != 4 {
		t.Fatalf("valid BE/BS transition blocked: %v %+v", err, result)
	}
	if _, err := NewActivationAssertionVerifier(usecase.PasskeyRelyingParty{}); !errors.Is(err, usecase.ErrPasskeyAssertionUnavailable) {
		t.Fatal("RP policy absent but verifier was constructed")
	}
}

func TestPasskeyActivationAssertionRejectsAlteredSignatureAndCeremony(t *testing.T) {
	t.Parallel()
	key := signedTestKey(t)
	v := assertionVerifier(t)
	for _, tc := range []struct {
		name string
		opts assertionFixture
		mutate func(*usecase.PasskeyActivationEvidence)
	}{
		{"tampered signature", assertionFixture{flags: 0x05, counter: 1, tamperSignature: true}, nil},
		{"wrong challenge", assertionFixture{flags: 0x05, counter: 1, challenge: [32]byte{9}}, func(x *usecase.PasskeyActivationEvidence) { x.Nonce = [32]byte{1} }},
		{"wrong origin", assertionFixture{flags: 0x05, counter: 1, origin: "https://evil.example.test"}, nil},
		{"wrong RP hash", assertionFixture{flags: 0x05, counter: 1, rpID: "evil.example.test"}, nil},
		{"not user present", assertionFixture{flags: 0x04, counter: 1}, nil},
		{"not user verified", assertionFixture{flags: 0x01, counter: 1}, nil},
		{"creation ceremony type", assertionFixture{flags: 0x05, counter: 1, ceremonyType: "webauthn.create"}, nil},
		{"cross-origin context", assertionFixture{flags: 0x05, counter: 1, crossOrigin: true}, nil},
		{"wrong user handle", assertionFixture{flags: 0x05, counter: 1, userHandle: []byte("sub_BBBBBBBBBBBBBBBBBBBBBB")}, nil},
		{"counter did not advance", assertionFixture{flags: 0x05, counter: 2}, func(x *usecase.PasskeyActivationEvidence) { x.SignCount = 2 }},
		{"counter regressed", assertionFixture{flags: 0x05, counter: 1}, func(x *usecase.PasskeyActivationEvidence) { x.SignCount = 3 }},
		{"BE mismatch", assertionFixture{flags: 0x0d, counter: 1}, func(x *usecase.PasskeyActivationEvidence) { x.BackupEligible = false; x.BackupState = false }},
		{"bad BS without BE", assertionFixture{flags: 0x15, counter: 1}, func(x *usecase.PasskeyActivationEvidence) { x.BackupEligible = false; x.BackupState = false }},
		{"wrong expected subject", assertionFixture{flags: 0x05, counter: 1}, func(x *usecase.PasskeyActivationEvidence) { x.Subject = "" }},
		{"wrong expected origin", assertionFixture{flags: 0x05, counter: 1}, func(x *usecase.PasskeyActivationEvidence) { x.Origin = "https://evil.example.test" }},
		{"wrong expected RP", assertionFixture{flags: 0x05, counter: 1}, func(x *usecase.PasskeyActivationEvidence) { x.RPID = "evil.example.test" }},
		{"stale policy", assertionFixture{flags: 0x05, counter: 1}, func(x *usecase.PasskeyActivationEvidence) { x.PolicyGeneration = 2 }},
		{"zero expected nonce", assertionFixture{flags: 0x05, counter: 1}, func(x *usecase.PasskeyActivationEvidence) { x.Nonce = [32]byte{} }},
		{"absent COSE key", assertionFixture{flags: 0x05, counter: 1}, func(x *usecase.PasskeyActivationEvidence) { x.PublicKeyCOSE = nil }},
		{"wrong expected key", assertionFixture{flags: 0x05, counter: 1}, func(x *usecase.PasskeyActivationEvidence) { x.PublicKeyCOSE = bytes.Repeat([]byte{0x41}, 64) }},
		{"wrong credential ID", assertionFixture{flags: 0x05, counter: 1}, func(x *usecase.PasskeyActivationEvidence) { x.CredentialID = bytes.Repeat([]byte{0x35}, 32) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, expected := syntheticSignedAssertion(t, key, tc.opts)
			if tc.mutate != nil {
				tc.mutate(&expected)
			}
			out, err := v.VerifyAssertion(context.Background(), expected, payload)
			if !errors.Is(err, usecase.ErrPasskeyAssertionDenied) || len(out.CredentialID) != 0 {
				t.Fatalf("untrusted activation assertion accepted: %+v %v", out, err)
			}
		})
	}
	// A valid signature generated by one private key must be rejected if
	// the server's credential belongs to a DIFFERENT private key.
	key2 := signedTestKey(t)
	payload, _ := syntheticSignedAssertion(t, key2, assertionFixture{flags: 0x05, counter: 1})
	_, correct := syntheticSignedAssertion(t, key, assertionFixture{flags: 0x05, counter: 1})
	if got, err := v.VerifyAssertion(context.Background(), correct, payload); !errors.Is(err, usecase.ErrPasskeyAssertionDenied) || len(got.CredentialID) != 0 {
		t.Fatalf("assertion signed with unrelated key verified: %v", err)
	}
	for _, raw := range [][]byte{
		nil,
		[]byte("not-json"),
		[]byte(`{"id":"not-base64","type":"public-key"}`),
		make([]byte, (128<<10)+1),
	} {
		valid, expected := syntheticSignedAssertion(t, key, assertionFixture{flags: 0x05, counter: 1})
		_ = valid
		if got, err := v.VerifyAssertion(context.Background(), expected, raw); !errors.Is(err, usecase.ErrPasskeyAssertionDenied) || len(got.CredentialID) != 0 {
			t.Fatalf("malformed assertion accepted: %v", err)
		}
	}
}
