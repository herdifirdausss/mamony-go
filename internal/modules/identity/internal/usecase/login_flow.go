package usecase

import (
	"context"
	"errors"
	"net/netip"
)

// ErrPasswordLoginThrottled is an INTERNAL budget result, not a public
// account-specific HTTP error. No login HTTP route is mounted.
var ErrPasswordLoginThrottled = errors.New("password login admission exhausted")

// ErrPasswordLoginUnavailable signals sanitized admission/authentication failure.
var ErrPasswordLoginUnavailable = errors.New("password login unavailable")

type PasswordLoginAdmission interface {
	Admit(context.Context, string, netip.Addr) (bool, error)
}

type PasswordLoginAuthenticator interface {
	Authenticate(context.Context, string, string) (string, error)
}

// PasswordLoginFlow is the only composed login candidate. Admission MUST
// commit before any password/verifier DB read and Argon2id operation.
// This is not a session issuer and does not grant resource permissions.
type PasswordLoginFlow struct {
	admission PasswordLoginAdmission
	password PasswordLoginAuthenticator
}

func NewPasswordLoginFlow(admission PasswordLoginAdmission, password PasswordLoginAuthenticator) (*PasswordLoginFlow, error) {
	if admission == nil || password == nil {
		return nil, ErrPasswordLoginUnavailable
	}
	return &PasswordLoginFlow{admission: admission, password: password}, nil
}

// Authenticate admits even syntactically valid unknown aliases and spends
// one shared budget before invoking the password KDF path. The client address
// MUST come from the reviewed ClientIPResolver, never a user-supplied header.
// A 503/commit ambiguity from the admission DB fails closed; it must not be
// retried in this call, because a budget may already have been charged.
func (f *PasswordLoginFlow) Authenticate(ctx context.Context, email, password string, peer netip.Addr) (string, error) {
	if f == nil || f.admission == nil || f.password == nil ||
		ctx == nil || ctx.Err() != nil {
		return "", ErrPasswordLoginUnavailable
	}
	ok, err := f.admission.Admit(ctx, email, peer)
	if errors.Is(err, ErrLoginAdmissionInvalid) {
		return "", ErrPasswordAuthenticationDenied
	}
	if err != nil {
		return "", ErrPasswordLoginUnavailable
	}
	if !ok {
		return "", ErrPasswordLoginThrottled
	}
	subject, err := f.password.Authenticate(ctx, email, password)
	if errors.Is(err, ErrPasswordAuthenticationDenied) {
		return "", ErrPasswordAuthenticationDenied
	}
	if err != nil || subject == "" {
		return "", ErrPasswordLoginUnavailable
	}
	// This candidate can become stale when a subject is frozen; any future
	// session creation MUST recheck status/verified contact atomically.
	return subject, nil
}
