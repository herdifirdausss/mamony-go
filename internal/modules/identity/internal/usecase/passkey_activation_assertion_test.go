package usecase

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

type fakeActivationAssertionProtocol struct {
	calls    int
	expected PasskeyActivationEvidence
	payload  []byte
	result   PasskeyActivationAssertionResult
	err      error
}

func (f *fakeActivationAssertionProtocol) VerifyAssertion(_ context.Context,
	expected PasskeyActivationEvidence, raw []byte) (PasskeyActivationAssertionResult, error) {
	f.calls++
	f.expected = expected
	f.payload = append([]byte(nil), raw...)
	return f.result, f.err
}

func assertionProofFixture(t *testing.T) (*PasskeyActivationAssertionVerifier,
	*fakePasskeyActivationReaderStore, *fakeActivationAssertionProtocol, string) {
	t.Helper()
	_, reader, _, store, bearer, _ := activationFixture(t)
	protocol := &fakeActivationAssertionProtocol{
		result: PasskeyActivationAssertionResult{
			CredentialID: append([]byte(nil), store.record.CredentialID...),
			Counter:      1, UserPresent: true, UserVerified: true,
		},
	}
	proof, err := NewPasskeyActivationAssertionVerifier(reader, protocol)
	if err != nil {
		t.Fatal(err)
	}
	return proof, store, protocol, bearer
}

func TestPasskeyActivationAssertionProofUsesSameTrustedDBEvidence(t *testing.T) {
	t.Parallel()
	v, store, proto, bearer := assertionProofFixture(t)
	raw := []byte(`{"type":"public-key"}`)
	got, err := v.VerifyOnly(context.Background(), bearer,
		"pav_AAAAAAAAAAAAAAAAAAAAAA", raw)
	if err != nil || proto.calls != 1 ||
		got.Counter != 1 || !got.UserVerified ||
		!bytes.Equal(got.CredentialID, store.record.CredentialID) ||
		proto.expected.Subject != store.record.Subject ||
		!bytes.Equal(proto.payload, raw) {
		t.Fatalf("trusted expected assertion not bound to signature verifier: %v", err)
	}
	got.CredentialID[0] ^= 0xff
	if bytes.Equal(got.CredentialID, proto.result.CredentialID) == false {
		// The returned candidate changed, but the underlying protocol fake
		// must NOT be affected by aliasing.
		if proto.result.CredentialID[0] != store.record.CredentialID[0] {
			t.Fatal("untrusted caller changed adapter-supplied credential ID")
		}
	} else {
		t.Fatal("candidate returned aliased credential ID")
	}
	if _, err := NewPasskeyActivationAssertionVerifier(nil, proto); !errors.Is(err, ErrPasskeyAssertionUnavailable) {
		t.Fatal("nil evidence reader accepted")
	}
	if _, err := NewPasskeyActivationAssertionVerifier(v.expected, nil); !errors.Is(err, ErrPasskeyAssertionUnavailable) {
		t.Fatal("nil maintained assertion verifier accepted")
	}
}

func TestPasskeyActivationAssertionProofRejectsUntrustedClaims(t *testing.T) {
	t.Parallel()
	v, store, proto, bearer := assertionProofFixture(t)
	id := "pav_AAAAAAAAAAAAAAAAAAAAAA"
	for _, raw := range [][]byte{nil, {}, make([]byte, (128<<10)+1)} {
		if got, err := v.VerifyOnly(context.Background(), bearer, id, raw); !errors.Is(err, ErrPasskeyAssertionDenied) || len(got.CredentialID) != 0 {
			t.Fatalf("malformed assertion reached library: %v", err)
		}
	}
	if proto.calls != 0 {
		t.Fatal("unbounded/empty payload reached crypto verifier")
	}
	raw := []byte(`{"type":"public-key"}`)
	if got, err := v.VerifyOnly(context.Background(), "invalid", id, raw); !errors.Is(err, ErrPasskeyAssertionDenied) || len(got.CredentialID) != 0 {
		t.Fatal("invalid session reached crypto verifier")
	}
	store.found = false
	if got, err := v.VerifyOnly(context.Background(), bearer, id, raw); !errors.Is(err, ErrPasskeyAssertionDenied) || len(got.CredentialID) != 0 {
		t.Fatal("missing v34 challenge reached crypto verifier")
	}
	store.found = true
	store.err = errors.New("connection lost")
	if got, err := v.VerifyOnly(context.Background(), bearer, id, raw); !errors.Is(err, ErrPasskeyAssertionUnavailable) || len(got.CredentialID) != 0 {
		t.Fatal("DB reader error ignored")
	}
	store.err = nil
	proto.err = ErrPasskeyAssertionDenied
	if got, err := v.VerifyOnly(context.Background(), bearer, id, raw); !errors.Is(err, ErrPasskeyAssertionDenied) || len(got.CredentialID) != 0 {
		t.Fatal("signature failure accepted")
	}
	proto.err = nil

	base := proto.result
	for _, tc := range []struct {
		name   string
		modify func(*PasskeyActivationAssertionResult)
	}{
		{"wrong ID", func(x *PasskeyActivationAssertionResult) { x.CredentialID = bytes.Repeat([]byte{0x31}, 32) }},
		{"missing UP", func(x *PasskeyActivationAssertionResult) { x.UserPresent = false }},
		{"missing UV", func(x *PasskeyActivationAssertionResult) { x.UserVerified = false }},
		{"unexpected BE", func(x *PasskeyActivationAssertionResult) { x.BackupEligible = true }},
		{"BS without BE", func(x *PasskeyActivationAssertionResult) { x.BackupState = true }},
		{"nonprogressing counter", func(x *PasskeyActivationAssertionResult) { x.Counter = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proto.result = base
			tc.modify(&proto.result)
			if got, err := v.VerifyOnly(context.Background(), bearer, id, raw); !errors.Is(err, ErrPasskeyAssertionDenied) || len(got.CredentialID) != 0 {
				t.Fatalf("fake vendor proof bypassed usecase fence: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := v.VerifyOnly(ctx, bearer, id, raw); !errors.Is(err, ErrPasskeyAssertionUnavailable) || len(got.CredentialID) != 0 {
		t.Fatal("canceled operation called assertion verifier")
	}
}
