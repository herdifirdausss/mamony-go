package webauthn

import (
	"context"
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

// Independent synthetic "none" attestation fixture, not a copied vendor test
// response. This verifies challenge/origin/RP hash and authenticator flag
// enforcement; "none" attestation by itself is NOT hardware provenance or an
// independent identity/credential-binding authorization.
type registrationFixture struct {
	challenge   [32]byte
	rpID        string
	origin      string
	flags       byte
	clientType  string
	crossOrigin bool
	rawID       []byte
	attestedID  []byte
}

func syntheticRegistration(t *testing.T, f registrationFixture) []byte {
	t.Helper()
	if f.rpID == "" {
		f.rpID = "login.example.test"
	}
	if f.origin == "" {
		f.origin = "https://login.example.test"
	}
	if f.clientType == "" {
		f.clientType = "webauthn.create"
	}
	if f.rawID == nil {
		f.rawID = []byte("replay-safe-synthetic-cred-id-1234")
	}
	if f.attestedID == nil {
		f.attestedID = f.rawID
	}
	// A valid ES256 COSE EC2 public key with a deterministic, TEST-ONLY
	// private scalar. No private key enters the production verifier.
	scalar := make([]byte, 32)
	scalar[len(scalar)-1] = 7
	privateKey, err := ecdh.P256().NewPrivateKey(scalar)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := privateKey.PublicKey().Bytes()
	xCoord := publicKey[1:33]
	yCoord := publicKey[33:65]
	cose, err := cbor.Marshal(map[int]any{
		1: 2, 3: -7, -1: 1, -2: xCoord, -3: yCoord,
	})
	if err != nil {
		t.Fatal(err)
	}
	rpHash := sha256.Sum256([]byte(f.rpID))
	authData := append([]byte(nil), rpHash[:]...)
	authData = append(authData, f.flags)
	// 4-byte counter, 16-byte AAGUID, 2-byte big endian credential ID length.
	authData = append(authData, make([]byte, 4+16)...)
	var idLength [2]byte
	if len(f.attestedID) > 65535 {
		t.Fatal("synthetic attested credential ID exceeds uint16 length")
	}
	binary.BigEndian.PutUint16(idLength[:], uint16(len(f.attestedID)))
	authData = append(authData, idLength[:]...)
	authData = append(authData, f.attestedID...)
	authData = append(authData, cose...)
	attestation, err := cbor.Marshal(map[string]any{
		"fmt": "none", "authData": authData, "attStmt": map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := json.Marshal(map[string]any{
		"type":        f.clientType,
		"challenge":   base64.RawURLEncoding.EncodeToString(f.challenge[:]),
		"origin":      f.origin,
		"crossOrigin": f.crossOrigin,
	})
	if err != nil {
		t.Fatal(err)
	}
	encodedID := base64.RawURLEncoding.EncodeToString(f.rawID)
	result, err := json.Marshal(map[string]any{
		"id":    encodedID,
		"rawId": encodedID,
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    base64.RawURLEncoding.EncodeToString(client),
			"attestationObject": base64.RawURLEncoding.EncodeToString(attestation),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func testEvidence() usecase.PasskeyRegistrationEvidence {
	return usecase.PasskeyRegistrationEvidence{
		Subject: "sub_AAAAAAAAAAAAAAAAAAAAAA", Epoch: 2,
		Nonce: [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		RPID:  "login.example.test", Origin: "https://login.example.test",
		PolicyGeneration: 1,
	}
}

func TestWebAuthnRegistrationProtocolPositiveUntrustedCandidateOnly(t *testing.T) {
	t.Parallel()
	rp, err := usecase.NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewRegistrationVerifier(rp)
	if err != nil {
		t.Fatal(err)
	}
	evidence := testEvidence()
	// UP|UV|AT required for an attested credential; "none" attestation does
	// NOT establish device trust or independent recovery eligibility.
	payload := syntheticRegistration(t, registrationFixture{challenge: evidence.Nonce, flags: 0x45})
	got, err := v.VerifyRegistration(context.Background(), evidence, payload)
	if err != nil {
		t.Fatalf("valid synthetic WebAuthn create response rejected: %v", err)
	}
	if string(got.CredentialID) != "replay-safe-synthetic-cred-id-1234" ||
		len(got.PublicKeyCOSE) == 0 || len(got.AAGUID) != 16 ||
		!got.UserPresent || !got.UserVerified || got.AttestationFormat != "none" {
		t.Fatal("incomplete verified WebAuthn candidate")
	}
}

func TestWebAuthnRegistrationProtocolRejectsInvalidCeremonies(t *testing.T) {
	t.Parallel()
	rp, _ := usecase.NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	v, err := NewRegistrationVerifier(rp)
	if err != nil {
		t.Fatal(err)
	}
	evidence := testEvidence()
	base := registrationFixture{challenge: evidence.Nonce, flags: 0x45}
	badChallenge := evidence.Nonce
	badChallenge[0] ^= 0x80
	for _, tc := range []struct {
		name string
		f    registrationFixture
	}{
		{"mismatched challenge", registrationFixture{challenge: badChallenge, flags: 0x45}},
		{"wrong origin", registrationFixture{challenge: base.challenge, origin: "https://evil.example.test", flags: 0x45}},
		{"wrong RP hash", registrationFixture{challenge: base.challenge, rpID: "evil.example.test", flags: 0x45}},
		{"no user presence", registrationFixture{challenge: base.challenge, flags: 0x44}},
		{"no user verification", registrationFixture{challenge: base.challenge, flags: 0x41}},
		{"wrong ceremony type", registrationFixture{challenge: base.challenge, clientType: "webauthn.get", flags: 0x45}},
		{"cross-origin frame", registrationFixture{challenge: base.challenge, crossOrigin: true, flags: 0x45}},
		{"different attested credential ID", registrationFixture{challenge: base.challenge, flags: 0x45, attestedID: []byte("mismatched-attested-id")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := syntheticRegistration(t, tc.f)
			if got, err := v.VerifyRegistration(context.Background(), evidence, raw); !errors.Is(err, usecase.ErrPasskeyProtocolDenied) || len(got.CredentialID) != 0 {
				t.Fatalf("invalid WebAuthn registration accepted: %v", err)
			}
		})
	}
	for _, raw := range [][]byte{
		nil,
		[]byte("not-json"),
		[]byte(`{"id":"not-base64","type":"public-key"}`),
		make([]byte, 128<<10+1),
	} {
		if got, err := v.VerifyRegistration(context.Background(), evidence, raw); !errors.Is(err, usecase.ErrPasskeyProtocolDenied) || len(got.CredentialID) != 0 {
			t.Fatalf("invalid or oversized response accepted: %v", err)
		}
	}
	for _, mutate := range []func(*usecase.PasskeyRegistrationEvidence){
		func(e *usecase.PasskeyRegistrationEvidence) { e.Subject = "" },
		func(e *usecase.PasskeyRegistrationEvidence) { e.Epoch = 0 },
		func(e *usecase.PasskeyRegistrationEvidence) { e.Nonce = [32]byte{} },
		func(e *usecase.PasskeyRegistrationEvidence) { e.RPID = "evil.example.test" },
		func(e *usecase.PasskeyRegistrationEvidence) { e.Origin = "https://evil.example.test" },
		func(e *usecase.PasskeyRegistrationEvidence) { e.PolicyGeneration = 2 },
	} {
		bad := evidence
		mutate(&bad)
		if got, err := v.VerifyRegistration(context.Background(), bad, syntheticRegistration(t, base)); !errors.Is(err, usecase.ErrPasskeyProtocolDenied) || len(got.CredentialID) != 0 {
			t.Fatalf("untrusted expected ceremony context accepted: %v", err)
		}
	}
	if _, err := NewRegistrationVerifier(usecase.PasskeyRelyingParty{}); !errors.Is(err, usecase.ErrPasskeyProtocolUnavailable) {
		t.Fatal("missing trust-pinned RP accepted")
	}
}
