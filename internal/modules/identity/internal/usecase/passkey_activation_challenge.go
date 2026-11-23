package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
)

var (
	ErrPasskeyActivationDenied      = errors.New("passkey activation challenge denied")
	ErrPasskeyActivationUnavailable = errors.New("passkey activation challenge unavailable")
)

// PasskeyActivationChallenge is PUBLIC *request* material: only a nonce,
// credential selector and RP policy. It is NOT a verified assertion,
// login factor, credential activation or independent account authority.
type PasskeyActivationChallenge struct {
	id           string
	challenge    [32]byte
	credentialID []byte
	rp           PasskeyRelyingParty
}

func (c PasskeyActivationChallenge) ChallengeID() string { return c.id }
func (c PasskeyActivationChallenge) Challenge() []byte {
	return append([]byte(nil), c.challenge[:]...)
}
func (c PasskeyActivationChallenge) CredentialID() []byte {
	return append([]byte(nil), c.credentialID...)
}
func (c PasskeyActivationChallenge) RPID() string   { return c.rp.RPID() }
func (c PasskeyActivationChallenge) Origin() string { return c.rp.Origin() }

// PasskeyActivationChallengeStore is an EXECUTE-only v34 issuer. Database
// chooses the subject from the original live opaque session and checks its
// already-staged credential, original registration session and notice.
type PasskeyActivationChallengeStore interface {
	Issue(context.Context, [32]byte, []byte, string, [32]byte, string, string) (bool, error)
}

type PasskeyActivationChallengeIssuer struct {
	sessions *OpaqueSessionAuthority
	store    PasskeyActivationChallengeStore
	policy   PasskeyRelyingParty
	random   io.Reader
}

func NewPasskeyActivationChallengeIssuer(sessions *OpaqueSessionAuthority,
	store PasskeyActivationChallengeStore, policy PasskeyRelyingParty) (*PasskeyActivationChallengeIssuer, error) {
	if sessions == nil || store == nil || !policy.Configured() {
		return nil, ErrPasskeyActivationUnavailable
	}
	return &PasskeyActivationChallengeIssuer{
		sessions: sessions, store: store, policy: policy, random: rand.Reader,
	}, nil
}

// Begin issues a one-use future ASSERTION challenge for a selected
// PENDING_ASSERTION credential. It neither authenticates a WebAuthn
// assertion nor activates credentials or changes recovery assurance.
func (i *PasskeyActivationChallengeIssuer) Begin(ctx context.Context,
	bearer string, credentialID []byte) (PasskeyActivationChallenge, error) {
	var empty PasskeyActivationChallenge
	if i == nil || i.sessions == nil || i.store == nil || i.random == nil ||
		!i.policy.Configured() || ctx == nil || ctx.Err() != nil {
		return empty, ErrPasskeyActivationUnavailable
	}
	if len(credentialID) < 16 || len(credentialID) > 1023 {
		return empty, ErrPasskeyActivationDenied
	}
	subject, err := i.sessions.Resolve(ctx, bearer)
	if errors.Is(err, ErrSessionInvalid) {
		return empty, ErrPasskeyActivationDenied
	}
	if err != nil {
		return empty, ErrPasskeyActivationUnavailable
	}
	if !validSessionSubject.MatchString(subject) {
		return empty, ErrPasskeyActivationDenied
	}
	sessionDigest, err := sessionTokenDigest(bearer)
	if err != nil {
		return empty, ErrPasskeyActivationDenied
	}
	var challenge [32]byte
	if _, err := io.ReadFull(i.random, challenge[:]); err != nil {
		return empty, ErrPasskeyActivationUnavailable
	}
	id, err := newOpaqueID(i.random, "pav_")
	if err != nil || challenge == ([32]byte{}) {
		return empty, ErrPasskeyActivationUnavailable
	}
	// The input cannot mutate the issued credential selector on return.
	selector := append([]byte(nil), credentialID...)
	ok, err := i.store.Issue(ctx, sessionDigest, selector, id,
		challenge, i.policy.RPID(), i.policy.Origin())
	if err != nil || ctx.Err() != nil {
		// Unknown COMMIT outcome: never return the nonce or retry.
		return empty, ErrPasskeyActivationUnavailable
	}
	if !ok {
		return empty, ErrPasskeyActivationDenied
	}
	return PasskeyActivationChallenge{
		id: id, challenge: challenge, credentialID: selector, rp: i.policy,
	}, nil
}
