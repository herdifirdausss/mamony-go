package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

type fakeSessionStore struct {
	allow       bool
	subject     string
	issued      int
	resolved    int
	revoked     int
	issueErr    error
	resolveErr  error
	revokeErr   error
	digest      []byte
	grantDigest []byte
	event       string
}

func (f *fakeSessionStore) Issue(_ context.Context, grantDigest, digest []byte, event string) (bool, error) {
	f.issued++
	f.grantDigest = append([]byte(nil), grantDigest...)
	f.digest = append([]byte(nil), digest...)
	f.event = event
	return f.allow, f.issueErr
}
func (f *fakeSessionStore) Resolve(_ context.Context, digest []byte) (string, error) {
	f.resolved++
	f.digest = append([]byte(nil), digest...)
	return f.subject, f.resolveErr
}
func (f *fakeSessionStore) Revoke(_ context.Context, digest []byte, event string) error {
	f.revoked++
	f.digest = append([]byte(nil), digest...)
	f.event = event
	return f.revokeErr
}

func fixedAuthenticationGrant() AuthenticationGrant {
	return AuthenticationGrant{secret: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x71}, 32))}
}

func TestOpaqueSessionSecretsRedactedAndHashedOnly(t *testing.T) {
	store := &fakeSessionStore{allow: true, subject: "sub_AAAAAAAAAAAAAAAAAAAAAA"}
	authority, err := NewOpaqueSessionAuthority(store)
	if err != nil {
		t.Fatal(err)
	}
	s, err := authority.Issue(context.Background(), fixedAuthenticationGrant())
	if err != nil {
		t.Fatal(err)
	}
	secret := s.ForTrustedTransport()
	if len(secret) != 43 || len(store.digest) != 32 || store.issued != 1 ||
		len(store.event) != 26 || !strings.HasPrefix(store.event, "evt_") {
		t.Fatal("issued session lacks 256-bit entropy or durable audit obligation")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil || len(decoded) != 32 ||
		bytes.Equal(store.digest, decoded) || strings.Contains(string(store.digest), secret) {
		t.Fatal("session token was persisted as plaintext or has wrong entropy")
	}
	material := append([]byte(sessionDigestDomain), decoded...)
	want := sha256.Sum256(material)
	if !bytes.Equal(store.digest, want[:]) {
		t.Fatal("session token must be stored as a domain-separated digest")
	}
	if fmt.Sprint(s) == secret || fmt.Sprintf("%#v", s) == secret ||
		strings.Contains(fmt.Sprint(s), secret) {
		t.Fatal("opaque session leaked through Go formatting")
	}
	if b, err := json.Marshal(s); err == nil || bytes.Contains(b, []byte(secret)) {
		t.Fatal("opaque session JSON serialization must refuse a bearer")
	}
	if subject, err := authority.Resolve(context.Background(), secret); err != nil ||
		subject != store.subject || store.resolved != 1 {
		t.Fatal("valid opaque session did not resolve only to subject")
	}
	if err := authority.Revoke(context.Background(), secret); err != nil || store.revoked != 1 {
		t.Fatal("opaque session was not revoked")
	}
	if store.event == "" || store.event[:4] != "evt_" {
		t.Fatal("revocation lacked secure event ID")
	}
}

func TestOpaqueSessionFailClosedAndNeverReturnCommitAmbiguousBearer(t *testing.T) {
	store := &fakeSessionStore{allow: true, subject: "sub_AAAAAAAAAAAAAAAAAAAAAA"}
	a, err := NewOpaqueSessionAuthority(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewOpaqueSessionAuthority(nil); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatal("nil session authority accepted")
	}
	_, err = a.Issue(context.Background(), AuthenticationGrant{secret: "invalid"})
	if !errors.Is(err, ErrSessionDenied) || store.issued != 0 {
		t.Fatal("invalid subject reached session minting authority")
	}
	store.allow = false
	s, err := a.Issue(context.Background(), fixedAuthenticationGrant())
	if !errors.Is(err, ErrSessionDenied) || s.ForTrustedTransport() != "" {
		t.Fatal("ineligible status emitted a bearer")
	}
	store.allow = true
	store.issueErr = errors.New("commit outcome unknown and private database DSN")
	s, err = a.Issue(context.Background(), fixedAuthenticationGrant())
	if !errors.Is(err, ErrSessionUnavailable) || s.ForTrustedTransport() != "" ||
		strings.Contains(err.Error(), "DSN") {
		t.Fatal("ambiguous SQL commit leaked token or private error")
	}
	store.issueErr = nil
	a.random = io.LimitReader(bytes.NewReader(nil), 0)
	s, err = a.Issue(context.Background(), fixedAuthenticationGrant())
	if !errors.Is(err, ErrSessionUnavailable) || s.ForTrustedTransport() != "" {
		t.Fatal("failed CSPRNG allowed an issued session")
	}
	a.random = bytes.NewReader(bytes.Repeat([]byte{0x3d}, 32))
	s, err = a.Issue(context.Background(), fixedAuthenticationGrant())
	if !errors.Is(err, ErrSessionUnavailable) || s.ForTrustedTransport() != "" {
		t.Fatal("partial entropy for event ID allowed an issued session")
	}
	var nilAuthority *OpaqueSessionAuthority
	if _, err := nilAuthority.Issue(context.Background(), fixedAuthenticationGrant()); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatal("nil issuer accepted")
	}
	for _, bad := range []string{"", "AAAA", strings.Repeat("A", 43) + "x", strings.Repeat("?", 43)} {
		before := store.resolved
		if subject, err := a.Resolve(context.Background(), bad); subject != "" ||
			!errors.Is(err, ErrSessionInvalid) || store.resolved != before {
			t.Fatal("malformed session looked up secret in database")
		}
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if _, err := a.Issue(ctx, fixedAuthenticationGrant()); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatal("canceled issuance accepted")
	}
	if err := a.Revoke(ctx, strings.Repeat("A", 43)); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatal("canceled revocation reached authority")
	}
}
