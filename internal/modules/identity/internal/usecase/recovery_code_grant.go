package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
)

// RecoveryCodeGrantStore is intentionally narrower than an ordinary login
// grant. The existing password-authentication principal can stage this
// purpose only after the trusted Go caller verifies the current password.
type RecoveryCodeGrantStore interface {
	StageRecoveryCodes(context.Context, string, int64, []byte) (bool, error)
}

type RecoveryCodeGrantAuthority struct {
	store  RecoveryCodeGrantStore
	random io.Reader
}

func NewRecoveryCodeGrantAuthority(store RecoveryCodeGrantStore) (*RecoveryCodeGrantAuthority, error) {
	if store == nil {
		return nil, ErrSessionUnavailable
	}
	return &RecoveryCodeGrantAuthority{store: store, random: rand.Reader}, nil
}

// Issue produces a purpose-bound 30-second assertion; neither SQL role nor
// public code can use it to create a session or change a password.
func (a *RecoveryCodeGrantAuthority) Issue(ctx context.Context, subject string, epoch int64) (AuthenticationGrant, error) {
	if a == nil || a.store == nil || a.random == nil || ctx == nil || ctx.Err() != nil ||
		!validSessionSubject.MatchString(subject) || epoch <= 0 {
		return AuthenticationGrant{}, ErrSessionUnavailable
	}
	var secret [32]byte
	if _, err := io.ReadFull(a.random, secret[:]); err != nil {
		return AuthenticationGrant{}, ErrSessionUnavailable
	}
	encoded := base64.RawURLEncoding.EncodeToString(secret[:])
	clear(secret[:])
	digest, err := authenticationGrantDigest(encoded)
	if err != nil {
		return AuthenticationGrant{}, ErrSessionUnavailable
	}
	ok, err := a.store.StageRecoveryCodes(ctx, subject, epoch, digest[:])
	if err != nil {
		// A lost COMMIT acknowledgement is ambiguous. Do not retry the
		// same assertion or make it accessible to the caller.
		return AuthenticationGrant{}, ErrSessionUnavailable
	}
	if !ok {
		return AuthenticationGrant{}, ErrSessionDenied
	}
	return AuthenticationGrant{secret: encoded}, nil
}
