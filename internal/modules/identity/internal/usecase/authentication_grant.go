package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
)

const passwordLoginGrantDomain = "mamony:identity:password-login-grant:v1\x00"

// AuthenticationGrant is an ephemeral, non-serializable proof reference.
// It is an ASSERTION by the trusted Go authenticator, NOT SQL proof of Argon2id.
// The raw 256-bit secret is never stored in PostgreSQL or returned by HTTP.
type AuthenticationGrant struct {
	secret string
}

func (AuthenticationGrant) String() string               { return "[authentication grant redacted]" }
func (AuthenticationGrant) GoString() string             { return "[authentication grant redacted]" }
func (AuthenticationGrant) MarshalJSON() ([]byte, error) { return nil, ErrSessionUnavailable }
func (AuthenticationGrant) MarshalText() ([]byte, error) { return nil, ErrSessionUnavailable }

// AuthenticationGrantStore uses a distinct DB role with STAGE only.
// A compromise of this role can assert arbitrary successful authentication;
// it must not be treated as a password verifier or independent security root.
type AuthenticationGrantStore interface {
	Stage(context.Context, string, []byte) (bool, error)
}

type AuthenticationGrantAuthority struct {
	store  AuthenticationGrantStore
	random io.Reader
}

func NewAuthenticationGrantAuthority(store AuthenticationGrantStore) (*AuthenticationGrantAuthority, error) {
	if store == nil {
		return nil, ErrSessionUnavailable
	}
	return &AuthenticationGrantAuthority{store: store, random: rand.Reader}, nil
}

func authenticationGrantDigest(secret string) ([32]byte, error) {
	var out [32]byte
	if len(secret) != 43 {
		return out, ErrSessionDenied
	}
	raw, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil || len(raw) != 32 || base64.RawURLEncoding.EncodeToString(raw) != secret {
		return out, ErrSessionDenied
	}
	material := make([]byte, 0, len(passwordLoginGrantDomain)+len(raw))
	material = append(material, passwordLoginGrantDomain...)
	material = append(material, raw...)
	return sha256.Sum256(material), nil
}

// Issue may be invoked ONLY after PasswordLoginFlow successfully consumes
// admission and verifies the password. Profile A relies on this trusted
// composition, NOT on a cryptographically independent SQL password check.
func (a *AuthenticationGrantAuthority) Issue(ctx context.Context, subject string) (AuthenticationGrant, error) {
	if a == nil || a.store == nil || a.random == nil || ctx == nil || ctx.Err() != nil {
		return AuthenticationGrant{}, ErrSessionUnavailable
	}
	if !validSessionSubject.MatchString(subject) {
		return AuthenticationGrant{}, ErrSessionDenied
	}
	var token [32]byte
	if _, err := io.ReadFull(a.random, token[:]); err != nil {
		return AuthenticationGrant{}, ErrSessionUnavailable
	}
	secret := base64.RawURLEncoding.EncodeToString(token[:])
	digest, err := authenticationGrantDigest(secret)
	if err != nil {
		return AuthenticationGrant{}, ErrSessionUnavailable
	}
	allowed, err := a.store.Stage(ctx, subject, digest[:])
	if err != nil {
		// Including an ambiguous commit: no raw grant may be returned or retried.
		return AuthenticationGrant{}, ErrSessionUnavailable
	}
	if !allowed {
		return AuthenticationGrant{}, ErrSessionDenied
	}
	return AuthenticationGrant{secret: secret}, nil
}
