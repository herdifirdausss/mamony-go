package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"
)

type fakeRegistrationProtocol struct {
	calls     int
	evidence  PasskeyRegistrationEvidence
	response  []byte
	candidate PasskeyRegistrationCandidate
	err       error
}

func (f *fakeRegistrationProtocol) VerifyRegistration(_ context.Context,
	evidence PasskeyRegistrationEvidence, raw []byte) (PasskeyRegistrationCandidate, error) {
	f.calls++
	f.evidence = evidence
	f.response = append([]byte(nil), raw...)
	return f.candidate, f.err
}

func passkeyProofFixture(t *testing.T) (*PasskeyRegistrationProofVerifier,
	*stubPasskeyEvidenceStore, *fakeRegistrationProtocol, string) {
	t.Helper()
	policy, err := NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := NewOpaqueSessionAuthority(&stubPasskeySessionStore{
		subject: "sub_AAAAAAAAAAAAAAAAAAAAAA",
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &stubPasskeyEvidenceStore{
		found: true,
		evidence: PasskeyRegistrationEvidence{
			Subject:          "sub_AAAAAAAAAAAAAAAAAAAAAA",
			Epoch:            3,
			Nonce:            [32]byte{0xaa},
			RPID:             policy.RPID(),
			Origin:           policy.Origin(),
			PolicyGeneration: 1,
		},
	}
	reader, err := NewPasskeyRegistrationEvidenceReader(sessions, store, policy)
	if err != nil {
		t.Fatal(err)
	}
	proto := &fakeRegistrationProtocol{candidate: PasskeyRegistrationCandidate{
		CredentialID: []byte{1, 2}, PublicKeyCOSE: []byte{3, 4}, AAGUID: make([]byte, 16),
		UserPresent: true, UserVerified: true,
	}}
	verify, err := NewPasskeyRegistrationProofVerifier(reader, proto)
	if err != nil {
		t.Fatal(err)
	}
	bearer := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x61}, 32))
	return verify, store, proto, bearer
}

func TestPasskeyProofVerifierPreservesReadBoundaryAndCopiesCandidate(t *testing.T) {
	t.Parallel()
	v, store, proto, bearer := passkeyProofFixture(t)
	raw := []byte(`{"type":"public-key","response":{}}`)
	got, err := v.VerifyOnly(context.Background(), bearer,
		"wrg_AAAAAAAAAAAAAAAAAAAAAA", raw)
	if err != nil || len(got.CredentialID) != 2 ||
		proto.calls != 1 || proto.evidence != store.evidence ||
		!bytes.Equal(proto.response, raw) {
		t.Fatalf("protocol verifier did not receive trusted DB evidence: %v", err)
	}
	got.CredentialID[0] = 99
	got.PublicKeyCOSE[0] = 99
	if proto.candidate.CredentialID[0] == 99 || proto.candidate.PublicKeyCOSE[0] == 99 {
		t.Fatal("caller can corrupt adapter-held credential candidate")
	}
	if _, err := NewPasskeyRegistrationProofVerifier(nil, proto); !errors.Is(err, ErrPasskeyProtocolUnavailable) {
		t.Fatal("missing DB evidence reader accepted")
	}
	if _, err := NewPasskeyRegistrationProofVerifier(v.expected, nil); !errors.Is(err, ErrPasskeyProtocolUnavailable) {
		t.Fatal("missing protocol verifier accepted")
	}
}

func TestPasskeyProofVerifierRejectsUntrustedOrIncompleteEvidence(t *testing.T) {
	t.Parallel()
	v, store, proto, bearer := passkeyProofFixture(t)
	id := "wrg_AAAAAAAAAAAAAAAAAAAAAA"
	for _, bad := range [][]byte{nil, {}, make([]byte, (128<<10)+1)} {
		if got, err := v.VerifyOnly(context.Background(), bearer, id, bad); !errors.Is(err, ErrPasskeyProtocolDenied) || len(got.CredentialID) != 0 {
			t.Fatalf("bad payload reached protocol verifier: %v", err)
		}
	}
	if proto.calls != 0 {
		t.Fatal("invalid request reached verifier")
	}
	raw := []byte(`{"type":"public-key"}`)
	if got, err := v.VerifyOnly(context.Background(), "invalid", id, raw); !errors.Is(err, ErrPasskeyProtocolDenied) || len(got.CredentialID) != 0 {
		t.Fatal("invalid session reached protocol")
	}
	if proto.calls != 0 {
		t.Fatal("untrusted subject reached protocol")
	}
	store.found = false
	if got, err := v.VerifyOnly(context.Background(), bearer, id, raw); !errors.Is(err, ErrPasskeyProtocolDenied) || len(got.CredentialID) != 0 {
		t.Fatal("nonexisting pending intent reached protocol")
	}
	store.found = true
	store.err = errors.New("lost evidence response")
	if got, err := v.VerifyOnly(context.Background(), bearer, id, raw); !errors.Is(err, ErrPasskeyProtocolUnavailable) || len(got.CredentialID) != 0 {
		t.Fatal("database fault leaked protocol result")
	}
	store.err = nil
	proto.err = ErrPasskeyProtocolDenied
	if got, err := v.VerifyOnly(context.Background(), bearer, id, raw); !errors.Is(err, ErrPasskeyProtocolDenied) || len(got.CredentialID) != 0 {
		t.Fatal("invalid WebAuthn proof became verified candidate")
	}
	proto.err = nil
	for _, incomplete := range []PasskeyRegistrationCandidate{
		{PublicKeyCOSE: []byte{1}, AAGUID: make([]byte, 16), UserPresent: true, UserVerified: true},
		{CredentialID: []byte{1}, AAGUID: make([]byte, 16), UserPresent: true, UserVerified: true},
		{CredentialID: []byte{1}, PublicKeyCOSE: []byte{2}, UserPresent: true, UserVerified: true},
		{CredentialID: []byte{1}, PublicKeyCOSE: []byte{2}, AAGUID: make([]byte, 16), UserPresent: false, UserVerified: true},
		{CredentialID: []byte{1}, PublicKeyCOSE: []byte{2}, AAGUID: make([]byte, 16), UserPresent: true, UserVerified: false},
	} {
		proto.candidate = incomplete
		if got, err := v.VerifyOnly(context.Background(), bearer, id, raw); !errors.Is(err, ErrPasskeyProtocolDenied) || len(got.CredentialID) != 0 {
			t.Fatalf("incomplete candidate accepted: %+v %v", incomplete, err)
		}
	}
}
