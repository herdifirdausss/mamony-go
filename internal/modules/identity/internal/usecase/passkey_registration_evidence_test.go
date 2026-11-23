package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"
)

type stubPasskeyEvidenceStore struct {
	evidence PasskeyRegistrationEvidence
	found    bool
	err      error
	calls    int
	digest   [32]byte
	intentID string
}

func (s *stubPasskeyEvidenceStore) Read(_ context.Context, digest [32]byte,
	intentID string) (PasskeyRegistrationEvidence, bool, error) {
	s.calls++
	s.digest, s.intentID = digest, intentID
	return s.evidence, s.found, s.err
}

func TestPasskeyRegistrationEvidenceReadMatchesSessionAndPolicy(t *testing.T) {
	t.Parallel()
	policy, err := NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	if err != nil {
		t.Fatal(err)
	}
	subject := "sub_AAAAAAAAAAAAAAAAAAAAAA"
	sessionStore := &stubPasskeySessionStore{subject: subject}
	sessions, err := NewOpaqueSessionAuthority(sessionStore)
	if err != nil {
		t.Fatal(err)
	}
	store := &stubPasskeyEvidenceStore{
		found: true,
		evidence: PasskeyRegistrationEvidence{
			Subject: subject, Epoch: 2, Nonce: [32]byte{0x99},
			RPID: policy.RPID(), Origin: policy.Origin(), PolicyGeneration: 1,
		},
	}
	reader, err := NewPasskeyRegistrationEvidenceReader(sessions, store, policy)
	if err != nil {
		t.Fatal(err)
	}
	bearer := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x33}, 32))
	id := "wrg_AAAAAAAAAAAAAAAAAAAAAA"
	evidence, err := reader.Read(context.Background(), bearer, id)
	expectedDigest, digestErr := sessionTokenDigest(bearer)
	if err != nil || digestErr != nil || evidence.Subject != subject ||
		evidence.Nonce != store.evidence.Nonce || evidence.PolicyGeneration != 1 ||
		store.calls != 1 || store.digest != expectedDigest || store.intentID != id {
		t.Fatalf("trusted expected challenge was not session/RP-bound: %v", err)
	}
}

func TestPasskeyRegistrationEvidenceFailsClosed(t *testing.T) {
	t.Parallel()
	policy, _ := NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	subject := "sub_AAAAAAAAAAAAAAAAAAAAAA"
	sessions, _ := NewOpaqueSessionAuthority(&stubPasskeySessionStore{subject: subject})
	evidence := PasskeyRegistrationEvidence{
		Subject: subject, Epoch: 2, Nonce: [32]byte{0x44},
		RPID: policy.RPID(), Origin: policy.Origin(), PolicyGeneration: 1,
	}
	store := &stubPasskeyEvidenceStore{found: true, evidence: evidence}
	reader, _ := NewPasskeyRegistrationEvidenceReader(sessions, store, policy)
	bearer := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x55}, 32))
	id := "wrg_AAAAAAAAAAAAAAAAAAAAAA"

	if _, err := NewPasskeyRegistrationEvidenceReader(nil, store, policy); !errors.Is(err, ErrPasskeyRegistrationUnavailable) {
		t.Fatal("missing session authority accepted")
	}
	if _, err := NewPasskeyRegistrationEvidenceReader(sessions, nil, policy); !errors.Is(err, ErrPasskeyRegistrationUnavailable) {
		t.Fatal("missing evidence store accepted")
	}
	if _, err := NewPasskeyRegistrationEvidenceReader(sessions, store, PasskeyRelyingParty{}); !errors.Is(err, ErrPasskeyRegistrationUnavailable) {
		t.Fatal("empty relying-party policy accepted")
	}
	for _, wrong := range []string{"", "fac_AAAAAAAAAAAAAAAAAAAAAA", "wrg_broken"} {
		if got, err := reader.Read(context.Background(), bearer, wrong); !errors.Is(err, ErrPasskeyRegistrationDenied) || got.Subject != "" {
			t.Fatal("unrecognized registration intent identifier released evidence")
		}
	}
	if store.calls != 0 {
		t.Fatal("unvalidated intent reached database")
	}
	if got, err := reader.Read(context.Background(), "not-a-session", id); !errors.Is(err, ErrPasskeyRegistrationDenied) || got.Subject != "" {
		t.Fatal("invalid session released registration evidence")
	}

	tests := []struct {
		name string
		mutate func(*PasskeyRegistrationEvidence)
	}{
		{"other subject", func(x *PasskeyRegistrationEvidence) { x.Subject = "sub_BBBBBBBBBBBBBBBBBBBBBB" }},
		{"wrong RP", func(x *PasskeyRegistrationEvidence) { x.RPID = "other.example.test" }},
		{"wrong origin", func(x *PasskeyRegistrationEvidence) { x.Origin = "https://other.example.test" }},
		{"stale policy", func(x *PasskeyRegistrationEvidence) { x.PolicyGeneration = 2 }},
		{"zero epoch", func(x *PasskeyRegistrationEvidence) { x.Epoch = 0 }},
		{"empty nonce", func(x *PasskeyRegistrationEvidence) { x.Nonce = [32]byte{} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store.evidence = evidence
			tc.mutate(&store.evidence)
			got, err := reader.Read(context.Background(), bearer, id)
			if !errors.Is(err, ErrPasskeyRegistrationDenied) || got.Subject != "" {
				t.Fatalf("incorrect trusted evidence accepted: %v", err)
			}
		})
	}
	store.evidence = evidence
	store.found = false
	if got, err := reader.Read(context.Background(), bearer, id); !errors.Is(err, ErrPasskeyRegistrationDenied) || got.Subject != "" {
		t.Fatal("unknown challenge released expected registration data")
	}
	store.found = true
	store.err = errors.New("database unavailable")
	if got, err := reader.Read(context.Background(), bearer, id); !errors.Is(err, ErrPasskeyRegistrationUnavailable) || got.Subject != "" {
		t.Fatal("read failure released expected challenge")
	}
}
