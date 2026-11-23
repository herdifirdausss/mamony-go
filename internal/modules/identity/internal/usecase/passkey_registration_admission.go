package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net/netip"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

var (
	ErrPasskeyAdmissionDenied      = errors.New("passkey registration reauthentication denied")
	ErrPasskeyAdmissionUnavailable = errors.New("passkey registration reauthentication unavailable")
)

// PasskeyRegistrationReauthGrantStore is used ONLY by the existing trusted
// password-auth-grant SQL principal. It does not enroll credentials.
type PasskeyRegistrationReauthGrantStore interface {
	StagePasskey(context.Context, string, int64, []byte) (bool, error)
}

type PasskeyRegistrationReauthGrantAuthority struct {
	store  PasskeyRegistrationReauthGrantStore
	random io.Reader
}

func NewPasskeyRegistrationReauthGrantAuthority(
	store PasskeyRegistrationReauthGrantStore) (*PasskeyRegistrationReauthGrantAuthority, error) {
	if store == nil {
		return nil, ErrPasskeyAdmissionUnavailable
	}
	return &PasskeyRegistrationReauthGrantAuthority{store: store, random: rand.Reader}, nil
}

// issue is deliberately unexported. Its only production caller is the
// session+current-password flow below. The secret is never an HTTP value.
func (a *PasskeyRegistrationReauthGrantAuthority) issue(
	ctx context.Context, subject string, epoch int64) (AuthenticationGrant, error) {
	if a == nil || a.store == nil || a.random == nil ||
		ctx == nil || ctx.Err() != nil || !validSessionSubject.MatchString(subject) ||
		epoch <= 0 {
		return AuthenticationGrant{}, ErrPasskeyAdmissionUnavailable
	}
	var secret [32]byte
	if _, err := io.ReadFull(a.random, secret[:]); err != nil {
		return AuthenticationGrant{}, ErrPasskeyAdmissionUnavailable
	}
	encoded := base64.RawURLEncoding.EncodeToString(secret[:])
	clear(secret[:])
	digest, err := authenticationGrantDigest(encoded)
	if err != nil {
		return AuthenticationGrant{}, ErrPasskeyAdmissionUnavailable
	}
	ok, err := a.store.StagePasskey(ctx, subject, epoch, digest[:])
	if err != nil || ctx.Err() != nil {
		// If COMMIT succeeded but the acknowledgment was lost, the grant
		// secret never leaves this method and cannot be automatically retried.
		return AuthenticationGrant{}, ErrPasskeyAdmissionUnavailable
	}
	if !ok {
		return AuthenticationGrant{}, ErrPasskeyAdmissionDenied
	}
	return AuthenticationGrant{secret: encoded}, nil
}

// PasskeyRegistrationAdmissionStore can execute only the v32 session+intent
// admission function, not stage reauth proof or read/write identity tables.
type PasskeyRegistrationAdmissionStore interface {
	Admit(context.Context, [32]byte, string, [32]byte) (bool, error)
}

// AuthenticatedPasskeyRegistrationAdmission marks a fresh v30 ceremony as
// PASSWORD_REAUTH_ONLY. A valid password+session does NOT establish independent
// MFA, passkey provenance, or recovery-code bootstrap eligibility.
type AuthenticatedPasskeyRegistrationAdmission struct {
	sessions *OpaqueSessionAuthority
	current  *PasswordLoginFlow
	grants   *PasskeyRegistrationReauthGrantAuthority
	store    PasskeyRegistrationAdmissionStore
}

func NewAuthenticatedPasskeyRegistrationAdmission(sessions *OpaqueSessionAuthority,
	current *PasswordLoginFlow, grants *PasskeyRegistrationReauthGrantAuthority,
	store PasskeyRegistrationAdmissionStore) (*AuthenticatedPasskeyRegistrationAdmission, error) {
	if sessions == nil || current == nil || grants == nil || store == nil {
		return nil, ErrPasskeyAdmissionUnavailable
	}
	return &AuthenticatedPasskeyRegistrationAdmission{
		sessions: sessions, current: current, grants: grants, store: store,
	}, nil
}

// Admit reauthenticates the password for the original session subject using
// the existing durable admission BEFORE Argon2id and stages a 30-second,
// purpose-separated SQL proof. The independent v32 role atomically consumes
// it against the SAME session, subject/epoch, current intent and DB clock.
//
// No public handler or finish/credential insertion is present. A later finish
// must separately check provenance and one-use nonce/admission atomically.
func (a *AuthenticatedPasskeyRegistrationAdmission) Admit(ctx context.Context,
	bearer, intentID, canonicalEmail, password string, peer netip.Addr) error {
	if a == nil || a.sessions == nil || a.current == nil || a.grants == nil ||
		a.store == nil || ctx == nil || ctx.Err() != nil {
		return ErrPasskeyAdmissionUnavailable
	}
	if len(intentID) != 26 || intentID[:4] != "wrg_" {
		return ErrPasskeyAdmissionDenied
	}
	email, err := domain.CanonicalEmail(canonicalEmail)
	if err != nil || email != canonicalEmail {
		return ErrPasskeyAdmissionDenied
	}
	subject, err := a.sessions.Resolve(ctx, bearer)
	if errors.Is(err, ErrSessionInvalid) {
		return ErrPasskeyAdmissionDenied
	}
	if err != nil {
		return ErrPasskeyAdmissionUnavailable
	}
	reauthed, err := a.current.AuthenticateWithEpoch(ctx, email, password, peer)
	if errors.Is(err, ErrPasswordAuthenticationDenied) ||
		errors.Is(err, ErrPasswordLoginThrottled) {
		return ErrPasskeyAdmissionDenied
	}
	if err != nil {
		return ErrPasskeyAdmissionUnavailable
	}
	if subject != reauthed.SubjectID || reauthed.SessionEpoch <= 0 {
		return ErrPasskeyAdmissionDenied
	}
	sessionDigest, err := sessionTokenDigest(bearer)
	if err != nil {
		return ErrPasskeyAdmissionDenied
	}
	grant, err := a.grants.issue(ctx, reauthed.SubjectID, reauthed.SessionEpoch)
	if errors.Is(err, ErrPasskeyAdmissionDenied) {
		return ErrPasskeyAdmissionDenied
	}
	if err != nil {
		return ErrPasskeyAdmissionUnavailable
	}
	proofDigest, err := authenticationGrantDigest(grant.secret)
	if err != nil {
		return ErrPasskeyAdmissionUnavailable
	}
	allowed, err := a.store.Admit(ctx, sessionDigest, intentID, proofDigest)
	if err != nil || ctx.Err() != nil {
		// Unknown COMMIT outcome MUST NOT claim successful admission or
		// retry the consumed / possibly staged assertion automatically.
		return ErrPasskeyAdmissionUnavailable
	}
	if !allowed {
		return ErrPasskeyAdmissionDenied
	}
	return nil
}
