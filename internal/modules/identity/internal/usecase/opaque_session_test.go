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
	"net/netip"
	"strings"
	"testing"
)

type fakeSessionStore struct {
	allow      bool
	subject    string
	issued     int
	resolved   int
	revoked    int
	issueErr   error
	resolveErr error
	revokeErr  error
	digest      []byte
	grantDigest []byte
	event      string
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

type fakeAuthenticationGrantStore struct {
	allow   bool
	err     error
	stages  int
	subject string
	digest  []byte
}

func (f *fakeAuthenticationGrantStore) Stage(_ context.Context, subject string, digest []byte) (bool, error) {
	f.stages++
	f.subject = subject
	f.digest = append([]byte(nil), digest...)
	return f.allow, f.err
}

func testAuthenticationGrantAuthority(t *testing.T) (*AuthenticationGrantAuthority, *fakeAuthenticationGrantStore) {
	t.Helper()
	store := &fakeAuthenticationGrantStore{allow: true}
	a, err := NewAuthenticationGrantAuthority(store)
	if err != nil {
		t.Fatal(err)
	}
	return a, store
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

type sessionIssuerFake struct {
	token OpaqueSession
	err   error
	calls int
}

func (f *sessionIssuerFake) Issue(context.Context, AuthenticationGrant) (OpaqueSession, error) {
	f.calls++
	return f.token, f.err
}

// SessionLogin accepts ONLY the concrete PasswordLoginFlow. Its own tests
// therefore exercise admission -> password -> epoch-rechecking issuer, not
// a forged structural interface returning arbitrary subject candidates.
func TestSessionLoginNeverIssuesWithoutAuthenticatedCandidateOrRecheck(t *testing.T) {
	grant, _ := testAuthenticationGrantAuthority(t)
	if _, err := NewSessionLogin(nil, grant, &sessionIssuerFake{}); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatal("nil pre-KDF authenticator accepted")
	}
	goodFlow, err := NewPasswordLoginFlow(&loginGateFake{allowed: true}, &loginPasswordFake{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSessionLogin(goodFlow, grant, nil); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatal("nil atomic session issuer accepted")
	}
	if _, err := NewSessionLogin(goodFlow, nil, &sessionIssuerFake{}); !errors.Is(err, ErrSessionUnavailable) {
		t.Fatal("nil authenticated grant authority accepted")
	}
	for _, tc := range []struct {
		name          string
		admit         bool
		admitErr      error
		passwordErr   error
		subject       string
		issueErr      error
		want          error
		passwordCalls int
		issueCount    int
	}{
		{"wrong password", true, nil, ErrPasswordAuthenticationDenied, "", nil, ErrPasswordAuthenticationDenied, 1, 0},
		{"rate limited", false, nil, nil, "", nil, ErrPasswordLoginThrottled, 0, 0},
		{"admission down", false, ErrLoginAdmissionUnavailable, nil, "", nil, ErrSessionUnavailable, 0, 0},
		{"KDF unavailable", true, nil, ErrPasswordAuthenticationUnavailable, "", nil, ErrSessionUnavailable, 1, 0},
		{"invalid subject", true, nil, nil, "suspicious", nil, ErrSessionUnavailable, 1, 0},
		{"frozen after KDF", true, nil, nil, "sub_AAAAAAAAAAAAAAAAAAAAAA", ErrSessionDenied, ErrPasswordAuthenticationDenied, 1, 1},
		{"ambiguous session commit", true, nil, nil, "sub_AAAAAAAAAAAAAAAAAAAAAA", ErrSessionUnavailable, ErrSessionUnavailable, 1, 1},
		{"minted", true, nil, nil, "sub_AAAAAAAAAAAAAAAAAAAAAA", nil, nil, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gate := &loginGateFake{allowed: tc.admit, err: tc.admitErr}
			password := &loginPasswordFake{subject: tc.subject, err: tc.passwordErr}
			candidate, err := NewPasswordLoginFlow(gate, password)
			if err != nil {
				t.Fatal(err)
			}
			issuer := &sessionIssuerFake{token: OpaqueSession{secret: "secret"}, err: tc.issueErr}
			grant, grantStore := testAuthenticationGrantAuthority(t)
			login, err := NewSessionLogin(candidate, grant, issuer)
			if err != nil {
				t.Fatal(err)
			}
			session, gotErr := login.Authenticate(context.Background(), "alice@example.test",
				"password", netip.MustParseAddr("192.0.2.1"))
			if gate.calls != 1 || password.calls != tc.passwordCalls ||
				issuer.calls != tc.issueCount || grantStore.stages != tc.issueCount {
				t.Fatal("session authority bypassed admission or password decision")
			}
			if tc.want == nil {
				if gotErr != nil || session.ForTrustedTransport() == "" {
					t.Fatal("successful internal session login lost minted bearer")
				}
			} else if !errors.Is(gotErr, tc.want) || session.ForTrustedTransport() != "" ||
				gotErr.Error() != tc.want.Error() {
				t.Fatal("login failure leaked bearer or private error")
			}
		})
	}
}
