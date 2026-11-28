package usecase

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

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
