package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"
)

type stubPasskeySessionStore struct{ subject string }

func (*stubPasskeySessionStore) Issue(context.Context, []byte, []byte, string) (bool, error) {
	return false, ErrSessionUnavailable
}
func (s *stubPasskeySessionStore) Resolve(context.Context, []byte) (string, error) {
	return s.subject, nil
}
func (*stubPasskeySessionStore) Revoke(context.Context, []byte, string) error {
	return ErrSessionUnavailable
}

type stubPasskeyIntentStore struct {
	allowed bool
	err     error
	calls   int
	digest  [32]byte
	id      string
	nonce   [32]byte
	rpID    string
	origin  string
}

func (s *stubPasskeyIntentStore) Issue(_ context.Context,
	digest [32]byte, id string, nonce [32]byte, rpID, origin string) (bool, error) {
	s.calls++
	s.digest, s.id, s.nonce, s.rpID, s.origin = digest, id, nonce, rpID, origin
	return s.allowed, s.err
}

func TestPasskeyRegistrationBeginOnlyStagesSessionBoundIntent(t *testing.T) {
	t.Parallel()
	p, err := NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	if err != nil {
		t.Fatal(err)
	}
	sessionStore := &stubPasskeySessionStore{subject: "sub_AAAAAAAAAAAAAAAAAAAAAA"}
	sessions, err := NewOpaqueSessionAuthority(sessionStore)
	if err != nil {
		t.Fatal(err)
	}
	store := &stubPasskeyIntentStore{allowed: true}
	issuer, err := NewPasskeyRegistrationIntentIssuer(sessions, store, p)
	if err != nil {
		t.Fatal(err)
	}
	// Deterministic entropy ONLY for the unit test. Production constructor
	// always uses crypto/rand.Reader.
	issuer.random = bytes.NewReader(bytes.Repeat([]byte{0x7c}, 64))
	bearer := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x2a}, 32))
	intent, err := issuer.Begin(context.Background(), bearer)
	if err != nil || intent.IntentID() == "" || len(intent.Challenge()) != 32 {
		t.Fatalf("legitimate pending challenge denied: %v", err)
	}
	if intent.RPID() != p.RPID() || intent.Origin() != p.Origin() ||
		store.rpID != p.RPID() || store.origin != p.Origin() || store.calls != 1 {
		t.Fatal("RP configuration mismatch, duplicate issue or caller-controlled origin")
	}
	expectedDigest, err := sessionTokenDigest(bearer)
	if err != nil || store.digest != expectedDigest || store.id != intent.IntentID() ||
		!bytes.Equal(store.nonce[:], intent.Challenge()) {
		t.Fatal("stored intent not bound to caller session digest, ID and nonce")
	}
	// Returning mutable challenge slices cannot corrupt the issued immutable value.
	previous := intent.Challenge()
	previous[0] = 0
	if intent.Challenge()[0] == 0 {
		t.Fatal("caller could mutate durable challenge through aliasing")
	}
}

func TestPasskeyRegistrationIssuerFailsClosed(t *testing.T) {
	t.Parallel()
	p, _ := NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	sessionStore := &stubPasskeySessionStore{subject: "sub_AAAAAAAAAAAAAAAAAAAAAA"}
	sessions, _ := NewOpaqueSessionAuthority(sessionStore)
	store := &stubPasskeyIntentStore{allowed: true}
	issuer, _ := NewPasskeyRegistrationIntentIssuer(sessions, store, p)
	bearer := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	if _, err := NewPasskeyRegistrationIntentIssuer(sessions, store, PasskeyRelyingParty{}); !errors.Is(err, ErrPasskeyRegistrationUnavailable) {
		t.Fatal("empty RP configuration permitted registration issuance")
	}
	if _, err := NewPasskeyRegistrationIntentIssuer(nil, store, p); !errors.Is(err, ErrPasskeyRegistrationUnavailable) {
		t.Fatal("missing session authority permitted registration issuance")
	}
	if _, err := NewPasskeyRegistrationIntentIssuer(sessions, nil, p); !errors.Is(err, ErrPasskeyRegistrationUnavailable) {
		t.Fatal("missing issuer store permitted registration issuance")
	}
	if got, err := issuer.Begin(context.Background(), "not-a-session"); !errors.Is(err, ErrPasskeyRegistrationDenied) || got.IntentID() != "" {
		t.Fatal("invalid session minted a challenge")
	}
	if store.calls != 0 {
		t.Fatal("invalid bearer reached SQL issuer")
	}
	sessionStore.subject = ""
	if got, err := issuer.Begin(context.Background(), bearer); !errors.Is(err, ErrPasskeyRegistrationDenied) || got.IntentID() != "" {
		t.Fatal("unresolved live subject minted a challenge")
	}
	sessionStore.subject = "sub_AAAAAAAAAAAAAAAAAAAAAA"
	store.allowed = false
	if got, err := issuer.Begin(context.Background(), bearer); !errors.Is(err, ErrPasskeyRegistrationDenied) || got.IntentID() != "" {
		t.Fatal("SQL denial leaked an issued challenge")
	}
	store.err = errors.New("lost database response")
	if got, err := issuer.Begin(context.Background(), bearer); !errors.Is(err, ErrPasskeyRegistrationUnavailable) || got.IntentID() != "" {
		t.Fatal("ambiguous COMMIT returned a live challenge")
	}
	issuer.random = bytes.NewReader(nil)
	if got, err := issuer.Begin(context.Background(), bearer); !errors.Is(err, ErrPasskeyRegistrationUnavailable) || got.IntentID() != "" {
		t.Fatal("entropy exhaustion minted a challenge")
	}
}
