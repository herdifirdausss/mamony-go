package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"
)

type fakePasskeyActivationIssuerStore struct {
	allowed      bool
	err          error
	calls        int
	session      [32]byte
	credentialID []byte
	id           string
	nonce        [32]byte
	rpID         string
	origin       string
}

func (s *fakePasskeyActivationIssuerStore) Issue(_ context.Context,
	session [32]byte, credentialID []byte, id string, nonce [32]byte,
	rpID, origin string) (bool, error) {
	s.calls++
	s.session, s.credentialID, s.id, s.nonce, s.rpID, s.origin =
		session, append([]byte(nil), credentialID...), id, nonce, rpID, origin
	return s.allowed, s.err
}

type fakePasskeyActivationReaderStore struct {
	record  PasskeyActivationEvidence
	found   bool
	err     error
	calls   int
	session [32]byte
	id      string
}

func (s *fakePasskeyActivationReaderStore) Read(_ context.Context,
	session [32]byte, id string) (PasskeyActivationEvidence, bool, error) {
	s.calls++
	s.session, s.id = session, id
	return s.record, s.found, s.err
}

func activationFixture(t *testing.T) (
	*PasskeyActivationChallengeIssuer, *PasskeyActivationEvidenceReader,
	*fakePasskeyActivationIssuerStore, *fakePasskeyActivationReaderStore, string, []byte) {
	t.Helper()
	subject := "sub_AAAAAAAAAAAAAAAAAAAAAA"
	policy, err := NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := NewOpaqueSessionAuthority(&stubPasskeySessionStore{subject: subject})
	if err != nil {
		t.Fatal(err)
	}
	issueStore := &fakePasskeyActivationIssuerStore{allowed: true}
	issuer, err := NewPasskeyActivationChallengeIssuer(sessions, issueStore, policy)
	if err != nil {
		t.Fatal(err)
	}
	credID := bytes.Repeat([]byte{0x5b}, 32)
	readStore := &fakePasskeyActivationReaderStore{found: true,
		record: PasskeyActivationEvidence{
			Subject: subject, Epoch: 2, Nonce: [32]byte{1},
			CredentialID:  append([]byte(nil), credID...),
			PublicKeyCOSE: bytes.Repeat([]byte{0xa3}, 64),
			RPID:          policy.RPID(), Origin: policy.Origin(),
			PolicyGeneration: 1,
		},
	}
	reader, err := NewPasskeyActivationEvidenceReader(sessions, readStore, policy)
	if err != nil {
		t.Fatal(err)
	}
	bearer := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x66}, 32))
	return issuer, reader, issueStore, readStore, bearer, credID
}

func TestPasskeyActivationBeginOnlyIssuesSessionAndCredentialBoundNonce(t *testing.T) {
	t.Parallel()
	issuer, _, store, _, bearer, credID := activationFixture(t)
	issuer.random = bytes.NewReader(bytes.Repeat([]byte{0x72}, 64))
	out, err := issuer.Begin(context.Background(), bearer, credID)
	want, digestErr := sessionTokenDigest(bearer)
	if err != nil || digestErr != nil || store.calls != 1 || store.session != want ||
		store.id != out.ChallengeID() || store.nonce == ([32]byte{}) ||
		!bytes.Equal(store.nonce[:], out.Challenge()) ||
		!bytes.Equal(store.credentialID, credID) ||
		!bytes.Equal(out.CredentialID(), credID) ||
		store.rpID != out.RPID() || store.origin != out.Origin() {
		t.Fatalf("activation challenge detached from original session/credential: %v", err)
	}
	p := out.CredentialID()
	p[0] ^= 0xff
	if !bytes.Equal(out.CredentialID(), credID) {
		t.Fatal("credential selector returned aliased slice")
	}
	q := out.Challenge()
	q[0] ^= 0xff
	if bytes.Equal(q, out.Challenge()) {
		t.Fatal("challenge returned aliased slice")
	}
}

func TestPasskeyActivationIssuerFailsClosed(t *testing.T) {
	t.Parallel()
	issuer, _, store, _, bearer, credID := activationFixture(t)
	if _, err := NewPasskeyActivationChallengeIssuer(nil, store, issuer.policy); !errors.Is(err, ErrPasskeyActivationUnavailable) {
		t.Fatal("nil sessions accepted")
	}
	if _, err := NewPasskeyActivationChallengeIssuer(issuer.sessions, nil, issuer.policy); !errors.Is(err, ErrPasskeyActivationUnavailable) {
		t.Fatal("nil store accepted")
	}
	if _, err := NewPasskeyActivationChallengeIssuer(issuer.sessions, store, PasskeyRelyingParty{}); !errors.Is(err, ErrPasskeyActivationUnavailable) {
		t.Fatal("empty RP accepted")
	}
	for _, tc := range []struct {
		bearer   string
		selector []byte
	}{
		{"bad-session", credID},
		{bearer, []byte{1}},
		{bearer, bytes.Repeat([]byte{1}, 1024)},
	} {
		if got, err := issuer.Begin(context.Background(), tc.bearer, tc.selector); !errors.Is(err, ErrPasskeyActivationDenied) || got.ChallengeID() != "" {
			t.Fatalf("invalid begin issued nonce: %v", err)
		}
	}
	if store.calls != 0 {
		t.Fatal("bad session/selector reached database")
	}
	store.allowed = false
	if got, err := issuer.Begin(context.Background(), bearer, credID); !errors.Is(err, ErrPasskeyActivationDenied) || got.ChallengeID() != "" {
		t.Fatal("database denial disclosed activation nonce")
	}
	store.err = errors.New("lost COMMIT")
	if got, err := issuer.Begin(context.Background(), bearer, credID); !errors.Is(err, ErrPasskeyActivationUnavailable) || got.ChallengeID() != "" {
		t.Fatal("unknown COMMIT returned activation nonce")
	}
	issuer.random = bytes.NewReader(nil)
	if got, err := issuer.Begin(context.Background(), bearer, credID); !errors.Is(err, ErrPasskeyActivationUnavailable) || got.ChallengeID() != "" {
		t.Fatal("entropy exhaustion returned challenge")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := issuer.Begin(ctx, bearer, credID); !errors.Is(err, ErrPasskeyActivationUnavailable) {
		t.Fatal("canceled context staged an activation challenge")
	}
}

func TestPasskeyActivationEvidenceRequiresOriginalSessionAndPolicy(t *testing.T) {
	t.Parallel()
	issuer, reader, _, store, bearer, _ := activationFixture(t)
	id := "pav_AAAAAAAAAAAAAAAAAAAAAA"
	evidence, err := reader.Read(context.Background(), bearer, id)
	want, digestErr := sessionTokenDigest(bearer)
	if err != nil || digestErr != nil || store.calls != 1 ||
		store.session != want || store.id != id || evidence.Subject != store.record.Subject ||
		evidence.Epoch != 2 || evidence.RPID != issuer.policy.RPID() {
		t.Fatalf("reader lost trusted nonce/RP/credential binding: %v", err)
	}
	evidence.CredentialID[0] ^= 0xff
	evidence.PublicKeyCOSE[0] ^= 0xff
	if !bytes.Equal(store.record.CredentialID, bytes.Repeat([]byte{0x5b}, 32)) ||
		!bytes.Equal(store.record.PublicKeyCOSE, bytes.Repeat([]byte{0xa3}, 64)) {
		t.Fatal("reader leaked mutable buffer aliases")
	}
	if _, err := NewPasskeyActivationEvidenceReader(nil, store, reader.policy); !errors.Is(err, ErrPasskeyActivationUnavailable) {
		t.Fatal("nil session authority accepted")
	}
	if _, err := NewPasskeyActivationEvidenceReader(reader.sessions, nil, reader.policy); !errors.Is(err, ErrPasskeyActivationUnavailable) {
		t.Fatal("nil reader store accepted")
	}
	if _, err := NewPasskeyActivationEvidenceReader(reader.sessions, store, PasskeyRelyingParty{}); !errors.Is(err, ErrPasskeyActivationUnavailable) {
		t.Fatal("no RP accepted")
	}
	for _, invalid := range []string{"pav_invalid", "wrg_AAAAAAAAAAAAAAAAAAAAAA", ""} {
		if got, err := reader.Read(context.Background(), bearer, invalid); !errors.Is(err, ErrPasskeyActivationDenied) || got.Subject != "" {
			t.Fatal("invalid activation ID released credential public key")
		}
	}
	if _, err := reader.Read(context.Background(), "bad-session", id); !errors.Is(err, ErrPasskeyActivationDenied) {
		t.Fatal("invalid session released key material")
	}
	valid := store.record
	for _, tc := range []struct {
		name   string
		mutate func(*PasskeyActivationEvidence)
	}{
		{"other subject", func(e *PasskeyActivationEvidence) { e.Subject = "sub_BBBBBBBBBBBBBBBBBBBBBB" }},
		{"wrong RP", func(e *PasskeyActivationEvidence) { e.RPID = "evil.example.test" }},
		{"wrong origin", func(e *PasskeyActivationEvidence) { e.Origin = "https://evil.example.test" }},
		{"stale epoch", func(e *PasskeyActivationEvidence) { e.Epoch = 0 }},
		{"zero nonce", func(e *PasskeyActivationEvidence) { e.Nonce = [32]byte{} }},
		{"missing key", func(e *PasskeyActivationEvidence) { e.PublicKeyCOSE = nil }},
		{"missing credential", func(e *PasskeyActivationEvidence) { e.CredentialID = nil }},
		{"wrong generation", func(e *PasskeyActivationEvidence) { e.PolicyGeneration = 2 }},
		{"invalid backup flags", func(e *PasskeyActivationEvidence) { e.BackupState = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store.record = valid
			tc.mutate(&store.record)
			if got, err := reader.Read(context.Background(), bearer, id); !errors.Is(err, ErrPasskeyActivationDenied) || got.Subject != "" {
				t.Fatalf("invalid expected assertion context escaped: %v", err)
			}
		})
	}
	store.record = valid
	store.found = false
	if got, err := reader.Read(context.Background(), bearer, id); !errors.Is(err, ErrPasskeyActivationDenied) || got.Subject != "" {
		t.Fatal("missing challenge released key")
	}
	store.found = true
	store.err = errors.New("SQL unavailable")
	if _, err := reader.Read(context.Background(), bearer, id); !errors.Is(err, ErrPasskeyActivationUnavailable) {
		t.Fatal("SQL error disclosed evidence")
	}
}
