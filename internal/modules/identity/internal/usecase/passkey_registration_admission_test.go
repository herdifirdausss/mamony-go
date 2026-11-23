package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/netip"
	"testing"
)

type fakePasskeyReauthStager struct {
	calls   int
	subject string
	epoch   int64
	digest  []byte
	allowed bool
	err     error
}

func (f *fakePasskeyReauthStager) StagePasskey(_ context.Context, subject string,
	epoch int64, digest []byte) (bool, error) {
	f.calls++
	f.subject, f.epoch = subject, epoch
	f.digest = append([]byte(nil), digest...)
	return f.allowed, f.err
}

type fakePasskeyAdmissionStore struct {
	calls   int
	session [32]byte
	intent  string
	grant   [32]byte
	allowed bool
	err     error
}

func (f *fakePasskeyAdmissionStore) Admit(_ context.Context,
	session [32]byte, intent string, grant [32]byte) (bool, error) {
	f.calls++
	f.session, f.intent, f.grant = session, intent, grant
	return f.allowed, f.err
}

func passkeyAdmissionFixture(t *testing.T) (*AuthenticatedPasskeyRegistrationAdmission,
	*loginGateFake, *loginPasswordFake, *fakePasskeyReauthStager,
	*fakePasskeyAdmissionStore, string) {
	t.Helper()
	subject := "sub_AAAAAAAAAAAAAAAAAAAAAA"
	sessionStore := &stubPasskeySessionStore{subject: subject}
	sessions, err := NewOpaqueSessionAuthority(sessionStore)
	if err != nil {
		t.Fatal(err)
	}
	gate := &loginGateFake{allowed: true}
	authenticator := &loginPasswordFake{subject: subject}
	flow, err := NewPasswordLoginFlow(gate, authenticator)
	if err != nil {
		t.Fatal(err)
	}
	stage := &fakePasskeyReauthStager{allowed: true}
	grants, err := NewPasskeyRegistrationReauthGrantAuthority(stage)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakePasskeyAdmissionStore{allowed: true}
	flowAdmission, err := NewAuthenticatedPasskeyRegistrationAdmission(
		sessions, flow, grants, store)
	if err != nil {
		t.Fatal(err)
	}
	bearer := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x3a}, 32))
	return flowAdmission, gate, authenticator, stage, store, bearer
}

func TestPasskeyRegistrationAdmissionStagesOnlyAfterRealReauthFlow(t *testing.T) {
	t.Parallel()
	a, gate, auth, stage, store, bearer := passkeyAdmissionFixture(t)
	id := "wrg_AAAAAAAAAAAAAAAAAAAAAA"
	peer := netip.MustParseAddr("192.0.2.65")
	err := a.Admit(context.Background(), bearer, id, "alice@example.test",
		"a correct passphrase", peer)
	sessionDigest, digestErr := sessionTokenDigest(bearer)
	if err != nil || digestErr != nil || gate.calls != 1 || auth.calls != 1 ||
		stage.calls != 1 || stage.epoch != 1 || stage.subject != auth.subject ||
		store.calls != 1 || store.session != sessionDigest || store.intent != id ||
		!bytes.Equal(stage.digest, store.grant[:]) || gate.peer != peer {
		t.Fatalf("reauthenticated passkey admission omitted required proof boundary: %v", err)
	}
}

func TestPasskeyRegistrationAdmissionDeniesWrongPasswordAndSubject(t *testing.T) {
	t.Parallel()
	peer := netip.MustParseAddr("192.0.2.66")
	for _, scenario := range []string{
		"invalid session", "unverified alias", "wrong password", "budget exhausted",
		"different subject", "staging unavailable", "stage denied",
		"admission unavailable", "admission denied",
	} {
		t.Run(scenario, func(t *testing.T) {
			a, gate, auth, stage, store, bearer := passkeyAdmissionFixture(t)
			email := "alice@example.test"
			switch scenario {
			case "invalid session":
				bearer = "not-a-token"
			case "unverified alias":
				email = "ALICE@EXAMPLE.TEST"
			case "wrong password":
				auth.err = ErrPasswordAuthenticationDenied
			case "budget exhausted":
				gate.allowed = false
			case "different subject":
				auth.subject = "sub_BBBBBBBBBBBBBBBBBBBBBB"
			case "staging unavailable":
				stage.err = errors.New("lost stage COMMIT")
			case "stage denied":
				stage.allowed = false
			case "admission unavailable":
				store.err = errors.New("lost admission COMMIT")
			case "admission denied":
				store.allowed = false
			}
			err := a.Admit(context.Background(), bearer,
				"wrg_AAAAAAAAAAAAAAAAAAAAAA", email, "password", peer)
			if !errors.Is(err, ErrPasskeyAdmissionDenied) &&
				!errors.Is(err, ErrPasskeyAdmissionUnavailable) {
				t.Fatalf("untrusted enrollment admission was accepted: %v", err)
			}
			if scenario == "wrong password" || scenario == "budget exhausted" ||
				scenario == "different subject" || scenario == "invalid session" ||
				scenario == "unverified alias" {
				if stage.calls != 0 || store.calls != 0 {
					t.Fatal("unverified account/session staged or consumed a proof")
				}
			}
		})
	}
	a, gate, auth, stage, store, bearer := passkeyAdmissionFixture(t)
	if err := a.Admit(context.Background(), bearer, "other_AAAAAAAAAAAAAAAAAAAAAA",
		"alice@example.test", "password", peer); !errors.Is(err, ErrPasskeyAdmissionDenied) ||
		gate.calls != 0 || auth.calls != 0 || stage.calls != 0 || store.calls != 0 {
		t.Fatal("invalid passkey purpose bypassed validation")
	}
}

func TestPasskeyRegistrationAdmissionMissingAuthorityAndCanceledContext(t *testing.T) {
	t.Parallel()
	a, _, _, stage, store, bearer := passkeyAdmissionFixture(t)
	if _, err := NewPasskeyRegistrationReauthGrantAuthority(nil); !errors.Is(err, ErrPasskeyAdmissionUnavailable) {
		t.Fatal("nil grant stager accepted")
	}
	if _, err := NewAuthenticatedPasskeyRegistrationAdmission(nil,
		a.current, a.grants, store); !errors.Is(err, ErrPasskeyAdmissionUnavailable) {
		t.Fatal("nil session authority accepted")
	}
	if _, err := NewAuthenticatedPasskeyRegistrationAdmission(a.sessions,
		nil, a.grants, store); !errors.Is(err, ErrPasskeyAdmissionUnavailable) {
		t.Fatal("nil password flow accepted")
	}
	if _, err := NewAuthenticatedPasskeyRegistrationAdmission(a.sessions,
		a.current, nil, store); !errors.Is(err, ErrPasskeyAdmissionUnavailable) {
		t.Fatal("nil grant authority accepted")
	}
	if _, err := NewAuthenticatedPasskeyRegistrationAdmission(a.sessions,
		a.current, a.grants, nil); !errors.Is(err, ErrPasskeyAdmissionUnavailable) {
		t.Fatal("nil SQL admission consumer accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Admit(ctx, bearer, "wrg_AAAAAAAAAAAAAAAAAAAAAA",
		"alice@example.test", "password", netip.MustParseAddr("192.0.2.67")); !errors.Is(err, ErrPasskeyAdmissionUnavailable) ||
		stage.calls != 0 || store.calls != 0 {
		t.Fatal("canceled operation staged trusted reauth")
	}
}
