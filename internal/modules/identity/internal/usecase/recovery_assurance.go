package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
)

// ErrRecoveryAssuranceDenied reveals neither an account nor token/code
// eligibility. The caller must keep one generic user-facing response.
var (
	ErrRecoveryAssuranceDenied = errors.New("recovery assurance denied")
	ErrRecoveryAssuranceUnavailable = errors.New("recovery assurance unavailable")
)

// RecoveryAssuranceStore may only execute one owner-controlled SQL function.
// The store receives two purpose-separated digests and an opaque audit ID,
// never a subject ID, session, password verifier or raw bearer.
type RecoveryAssuranceStore interface {
	Approve(context.Context, [32]byte, [32]byte, string) (bool, error)
}

// RecoveryOfflineCodeAssurance is a trusted-only, unmounted candidate.
// The caller must present BOTH an issued 15-minute email recovery bearer
// AND a saved 256-bit offline code enrolled before that email challenge.
type RecoveryOfflineCodeAssurance struct {
	store RecoveryAssuranceStore
	random io.Reader
}

func NewRecoveryOfflineCodeAssurance(store RecoveryAssuranceStore) (*RecoveryOfflineCodeAssurance, error) {
	if store == nil {
		return nil, ErrRecoveryAssuranceUnavailable
	}
	return &RecoveryOfflineCodeAssurance{store: store, random: rand.Reader}, nil
}

func (a *RecoveryOfflineCodeAssurance) Approve(ctx context.Context,
	emailRecoveryBearer, savedOfflineCode string) error {
	if a == nil || a.store == nil || a.random == nil ||
		ctx == nil || ctx.Err() != nil {
		return ErrRecoveryAssuranceUnavailable
	}
	recoveryDigest, err := PasswordRecoveryProofDigest(emailRecoveryBearer)
	if err != nil {
		return ErrRecoveryAssuranceDenied
	}
	codeDigest, err := OfflineRecoveryCodeDigest(savedOfflineCode)
	if err != nil {
		return ErrRecoveryAssuranceDenied
	}
	event, err := newOpaqueID(a.random, "evt_")
	if err != nil {
		return ErrRecoveryAssuranceUnavailable
	}
	ok, err := a.store.Approve(ctx, recoveryDigest, codeDigest, event)
	if err != nil {
		// Includes possible committed-but-not-acknowledged outcomes. Do
		// NOT automatically retry or claim that the code remains unused.
		return ErrRecoveryAssuranceUnavailable
	}
	if !ok {
		return ErrRecoveryAssuranceDenied
	}
	return nil
}
