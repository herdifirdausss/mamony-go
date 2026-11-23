package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
)

var (
	ErrPasskeyRegistrationDenied      = errors.New("passkey registration intent denied")
	ErrPasskeyRegistrationUnavailable = errors.New("passkey registration intent unavailable")
)

// PasskeyRegistrationIntent is PUBLIC ceremony material, not a verified
// authenticator, trusted binding grant, or proof of user verification.
// Its challenge is never itself an authorization to create credentials.
type PasskeyRegistrationIntent struct {
	id        string
	challenge [32]byte
	rp        PasskeyRelyingParty
}

func (i PasskeyRegistrationIntent) IntentID() string { return i.id }
func (i PasskeyRegistrationIntent) Challenge() []byte {
	out := make([]byte, len(i.challenge))
	copy(out, i.challenge[:])
	return out
}
func (i PasskeyRegistrationIntent) RPID() string   { return i.rp.RPID() }
func (i PasskeyRegistrationIntent) Origin() string { return i.rp.Origin() }

// PasskeyRegistrationIntentStore is implemented by a dedicated EXECUTE-only
// SQL role. SQL alone authenticates the session and selects its subject;
// neither this API nor its caller may select or rebind an account ID.
type PasskeyRegistrationIntentStore interface {
	Issue(context.Context, [32]byte, string, [32]byte, string, string) (bool, error)
}

type PasskeyRegistrationIntentIssuer struct {
	sessions *OpaqueSessionAuthority
	store    PasskeyRegistrationIntentStore
	policy   PasskeyRelyingParty
	random   io.Reader
}

func NewPasskeyRegistrationIntentIssuer(sessions *OpaqueSessionAuthority,
	store PasskeyRegistrationIntentStore, policy PasskeyRelyingParty) (*PasskeyRegistrationIntentIssuer, error) {
	if sessions == nil || store == nil || !policy.Configured() {
		return nil, ErrPasskeyRegistrationUnavailable
	}
	return &PasskeyRegistrationIntentIssuer{
		sessions: sessions, store: store, policy: policy, random: rand.Reader,
	}, nil
}

// Begin deliberately stages ONLY a challenge. It does NOT authenticate a
// WebAuthn response, enroll any passkey, attest presence/user-verification,
// assert independent MFA, or grant recovery-code bootstrap eligibility.
//
// The SQL issuer rechecks active contact/subject, epoch and session expiry
// under locks; this preliminary Resolve is fail-closed and denies bad tokens.
func (i *PasskeyRegistrationIntentIssuer) Begin(ctx context.Context, bearer string) (PasskeyRegistrationIntent, error) {
	if i == nil || i.sessions == nil || i.store == nil || i.random == nil ||
		!i.policy.Configured() || ctx == nil || ctx.Err() != nil {
		return PasskeyRegistrationIntent{}, ErrPasskeyRegistrationUnavailable
	}
	if _, err := i.sessions.Resolve(ctx, bearer); err != nil {
		if errors.Is(err, ErrSessionInvalid) {
			return PasskeyRegistrationIntent{}, ErrPasskeyRegistrationDenied
		}
		return PasskeyRegistrationIntent{}, ErrPasskeyRegistrationUnavailable
	}
	digest, err := sessionTokenDigest(bearer)
	if err != nil {
		return PasskeyRegistrationIntent{}, ErrPasskeyRegistrationDenied
	}
	var challenge [32]byte
	if _, err := io.ReadFull(i.random, challenge[:]); err != nil {
		return PasskeyRegistrationIntent{}, ErrPasskeyRegistrationUnavailable
	}
	id, err := newOpaqueID(i.random, "wrg_")
	if err != nil {
		return PasskeyRegistrationIntent{}, ErrPasskeyRegistrationUnavailable
	}
	if challenge == ([32]byte{}) {
		return PasskeyRegistrationIntent{}, ErrPasskeyRegistrationUnavailable
	}
	ok, err := i.store.Issue(ctx, digest, id, challenge, i.policy.RPID(), i.policy.Origin())
	if err != nil || ctx.Err() != nil {
		// Even a commit/lost-response ambiguity never returns a challenge.
		// Replays and competing issuances are refused by the DB.
		return PasskeyRegistrationIntent{}, ErrPasskeyRegistrationUnavailable
	}
	if !ok {
		return PasskeyRegistrationIntent{}, ErrPasskeyRegistrationDenied
	}
	return PasskeyRegistrationIntent{id: id, challenge: challenge, rp: i.policy}, nil
}
