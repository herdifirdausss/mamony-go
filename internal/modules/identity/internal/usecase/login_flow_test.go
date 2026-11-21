package usecase

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

type loginGateFake struct {
	allowed bool
	err     error
	calls   int
	email   string
	peer    netip.Addr
}

func (f *loginGateFake) Admit(_ context.Context, email string, peer netip.Addr) (bool, error) {
	f.calls++
	f.email, f.peer = email, peer
	return f.allowed, f.err
}

type loginPasswordFake struct {
	calls    int
	subject  string
	err      error
	email    string
	password string
}

func (f *loginPasswordFake) Authenticate(_ context.Context, email, password string) (string, error) {
	f.calls++
	f.email, f.password = email, password
	return f.subject, f.err
}

func TestPasswordLoginFlowMustAdmitBeforeAnyPasswordWork(t *testing.T) {
	ip := netip.MustParseAddr("203.0.113.2")
	cases := []struct {
		name      string
		gateOK    bool
		gateErr   error
		authErr   error
		subject   string
		wantErr   error
		wantCalls int
	}{
		{"allowed and matched", true, nil, nil, "sub_0123456789012345678901", nil, 1},
		{"unknown email or wrong password", true, nil, ErrPasswordAuthenticationDenied, "", ErrPasswordAuthenticationDenied, 1},
		{"limit spent", false, nil, nil, "", ErrPasswordLoginThrottled, 0},
		{"invalid alias/peer", false, ErrLoginAdmissionInvalid, nil, "", ErrPasswordAuthenticationDenied, 0},
		{"DB admission unavailable", false, ErrLoginAdmissionUnavailable, nil, "", ErrPasswordLoginUnavailable, 0},
		{"unexpected admission storage failure", false, errors.New("private SQL DSN"), nil, "", ErrPasswordLoginUnavailable, 0},
		{"KDF unavailable", true, nil, ErrPasswordAuthenticationUnavailable, "", ErrPasswordLoginUnavailable, 1},
		{"unexpected auth error", true, nil, errors.New("private KDF salt"), "", ErrPasswordLoginUnavailable, 1},
		{"empty positive subject", true, nil, nil, "", ErrPasswordLoginUnavailable, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &loginGateFake{allowed: tc.gateOK, err: tc.gateErr}
			a := &loginPasswordFake{subject: tc.subject, err: tc.authErr}
			flow, err := NewPasswordLoginFlow(g, a)
			if err != nil {
				t.Fatal(err)
			}
			subject, gotErr := flow.Authenticate(context.Background(), "unknown@example.test", "synthetic passphrase", ip)
			if g.calls != 1 || g.email != "unknown@example.test" || g.peer != ip {
				t.Fatal("login gate must be the first operation for every request")
			}
			if a.calls != tc.wantCalls {
				t.Fatalf("password verifier was invoked %d times, expected %d", a.calls, tc.wantCalls)
			}
			if tc.wantCalls == 1 && (a.email != g.email || a.password != "synthetic passphrase") {
				t.Fatal("login flow changed credential inputs after admission")
			}
			if tc.wantErr == nil {
				if gotErr != nil || subject != tc.subject {
					t.Fatalf("allowed credential subject not returned: %v", gotErr)
				}
			} else if subject != "" || !errors.Is(gotErr, tc.wantErr) ||
				gotErr.Error() != tc.wantErr.Error() {
				t.Fatalf("login boundary leaked subject/error: %v", gotErr)
			}
		})
	}
}

func TestPasswordLoginFlowMissingAuthorityAndCancellation(t *testing.T) {
	g := &loginGateFake{allowed: true}
	p := &loginPasswordFake{subject: "sub_0123456789012345678901"}
	if _, err := NewPasswordLoginFlow(nil, p); !errors.Is(err, ErrPasswordLoginUnavailable) {
		t.Fatal("missing admission accepted")
	}
	if _, err := NewPasswordLoginFlow(g, nil); !errors.Is(err, ErrPasswordLoginUnavailable) {
		t.Fatal("missing password verifier accepted")
	}
	flow, _ := NewPasswordLoginFlow(g, p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if subject, err := flow.Authenticate(ctx, "a@example.test", "password", netip.MustParseAddr("127.0.0.1")); subject != "" ||
		!errors.Is(err, ErrPasswordLoginUnavailable) || g.calls != 0 || p.calls != 0 {
		t.Fatal("canceled flow ran an authority operation")
	}
	var absent *PasswordLoginFlow
	if subject, err := absent.Authenticate(context.Background(), "a@example.test", "password", netip.MustParseAddr("127.0.0.1")); subject != "" ||
		!errors.Is(err, ErrPasswordLoginUnavailable) {
		t.Fatal("nil flow admitted password verification")
	}
}
