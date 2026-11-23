package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/netip"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

type changeHasherFake struct {
	calls int
	err   error
}

func (f *changeHasherFake) Hash(_ context.Context, _ string) (domain.PasswordVerifier, error) {
	f.calls++
	if f.err != nil {
		return domain.PasswordVerifier{}, f.err
	}
	v := dummyAuthenticationVerifier()
	v.Salt = bytes.Repeat([]byte{0x74}, 16)
	v.Digest = bytes.Repeat([]byte{0x84}, 32)
	return v, nil
}

type changeStoreFake struct {
	calls    int
	allowed  bool
	err      error
	session  []byte
	proof    []byte
	verifier domain.PasswordVerifier
	event    string
}

func (f *changeStoreFake) Rotate(_ context.Context, session, proof []byte, verifier domain.PasswordVerifier, event string) (bool, error) {
	f.calls++
	f.session = append([]byte(nil), session...)
	f.proof = append([]byte(nil), proof...)
	f.verifier = verifier
	f.event = event
	return f.allowed, f.err
}

func TestAuthenticatedPasswordChangeRequiresSessionAndRealOldPasswordPath(t *testing.T) {
	sessionSecret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x36}, 32))
	newFixture := func() (*AuthenticatedPasswordChange, *loginGateFake, *loginPasswordFake, *fakeSessionStore, *fakeAuthenticationGrantStore, *changeHasherFake, *changeStoreFake) {
		t.Helper()
		sessionStore := &fakeSessionStore{subject: "sub_AAAAAAAAAAAAAAAAAAAAAA"}
		session, err := NewOpaqueSessionAuthority(sessionStore)
		if err != nil {
			t.Fatal(err)
		}
		gate := &loginGateFake{allowed: true}
		password := &loginPasswordFake{subject: sessionStore.subject}
		login, err := NewPasswordLoginFlow(gate, password)
		if err != nil {
			t.Fatal(err)
		}
		grant, grantStore := testAuthenticationGrantAuthority(t)
		hasher := &changeHasherFake{}
		changeStore := &changeStoreFake{allowed: true}
		change, err := NewAuthenticatedPasswordChange(session, login, grant, hasher, changeStore,
			domain.PasswordPolicy{IsBlocked: func(value, _ string) bool {
				return value == "blocked password new long enough"
			}})
		if err != nil {
			t.Fatal(err)
		}
		return change, gate, password, sessionStore, grantStore, hasher, changeStore
	}
	net := netip.MustParseAddr("192.0.2.63")
	const currentCandidate = "an old synthetic very strong password"
	const replacementCandidate = "a fresh synthetic very strong password"
	t.Run("good session and correct password stage exactly once", func(t *testing.T) {
		change, gate, password, session, grant, hasher, store := newFixture()
		err := change.Change(context.Background(), sessionSecret,
			"test@example.com", currentCandidate, replacementCandidate, net)
		if err != nil || gate.calls != 1 || password.calls != 1 ||
			session.resolved != 1 || grant.stages != 1 || hasher.calls != 1 || store.calls != 1 {
			t.Fatalf("missing mandatory authorization path: err=%v gate=%d KDF=%d session=%d grant=%d hash=%d SQL=%d",
				err, gate.calls, password.calls, session.resolved, grant.stages, hasher.calls, store.calls)
		}
		if grant.epoch != 1 || store.event == "" || len(store.session) != 32 ||
			len(store.proof) != 32 || len(store.verifier.Salt) != 16 {
			t.Fatal("password change lost trusted epoch, digest or event")
		}
	})
	t.Run("no valid session cannot invoke password comparison", func(t *testing.T) {
		change, gate, password, session, grant, hasher, store := newFixture()
		session.subject = ""
		err := change.Change(context.Background(), sessionSecret,
			"test@example.com", currentCandidate, replacementCandidate, net)
		if !errors.Is(err, ErrPasswordChangeDenied) || gate.calls != 0 ||
			password.calls != 0 || grant.stages != 0 || hasher.calls != 0 || store.calls != 0 {
			t.Fatal("invalid bearer progressed into old-password work")
		}
	})
	t.Run("different account current password cannot change this bearer", func(t *testing.T) {
		change, _, password, _, grant, hasher, store := newFixture()
		password.subject = "sub_BBBBBBBBBBBBBBBBBBBBBB"
		err := change.Change(context.Background(), sessionSecret,
			"test@example.com", currentCandidate, replacementCandidate, net)
		if !errors.Is(err, ErrPasswordChangeDenied) || grant.stages != 0 ||
			hasher.calls != 0 || store.calls != 0 {
			t.Fatal("cross-account password assertion mutated another identity")
		}
	})
	t.Run("old-password denial cannot hash new password or stage change proof", func(t *testing.T) {
		change, _, password, _, grant, hasher, store := newFixture()
		password.err = ErrPasswordAuthenticationDenied
		err := change.Change(context.Background(), sessionSecret,
			"test@example.com", currentCandidate, replacementCandidate, net)
		if !errors.Is(err, ErrPasswordChangeDenied) || grant.stages != 0 ||
			hasher.calls != 0 || store.calls != 0 {
			t.Fatal("wrong password reached mutation")
		}
	})
	t.Run("throttled pre KDF admission cannot mutate", func(t *testing.T) {
		change, gate, password, _, grant, hasher, store := newFixture()
		gate.allowed = false
		err := change.Change(context.Background(), sessionSecret,
			"test@example.com", currentCandidate, replacementCandidate, net)
		if !errors.Is(err, ErrPasswordChangeDenied) || password.calls != 0 ||
			grant.stages != 0 || hasher.calls != 0 || store.calls != 0 {
			t.Fatal("throttled password comparison bypassed quota")
		}
	})
	t.Run("invalid or same new password is denied before new KDF", func(t *testing.T) {
		for _, next := range []string{currentCandidate, "short", "blocked password new long enough"} {
			change, _, _, _, grant, hasher, store := newFixture()
			err := change.Change(context.Background(), sessionSecret,
				"test@example.com", currentCandidate, next, net)
			if !errors.Is(err, ErrPasswordChangeDenied) || grant.stages != 0 ||
				hasher.calls != 0 || store.calls != 0 {
				t.Fatalf("invalid new password %q reached mutation: %v", next, err)
			}
		}
	})
	t.Run("commit ambiguous rotation never reported success", func(t *testing.T) {
		change, _, _, _, _, _, store := newFixture()
		store.err = errors.New("private connection reset after commit")
		err := change.Change(context.Background(), sessionSecret,
			"test@example.com", currentCandidate, replacementCandidate, net)
		if !errors.Is(err, ErrPasswordChangeUnavailable) || store.calls != 1 ||
			err.Error() != ErrPasswordChangeUnavailable.Error() {
			t.Fatal("database ambiguity leaked or became success")
		}
	})
	t.Run("invalid collaborator is rejected before any work", func(t *testing.T) {
		_, gate, _, _, _, hasher, store := newFixture()
		login, _ := NewPasswordLoginFlow(gate, &loginPasswordFake{subject: "sub_AAAAAAAAAAAAAAAAAAAAAA"})
		grant, _ := testAuthenticationGrantAuthority(t)
		if _, err := NewAuthenticatedPasswordChange(nil, login, grant, hasher, store,
			domain.PasswordPolicy{IsBlocked: func(string, string) bool { return false }}); !errors.Is(err, ErrPasswordChangeUnavailable) {
			t.Fatal("missing verified session authority accepted")
		}
	})
}
