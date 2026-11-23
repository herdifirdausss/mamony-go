// Package usecase orchestrates Identity operations through owner-defined ports.
package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

var (
	ErrAliasAlreadyReserved    = errors.New("identity alias already reserved")
	ErrRegistrationUnavailable = errors.New("registration unavailable")
)

// PendingRegistration must be persisted atomically by the repository together
// with a durable identifier-only audit/outbox obligation. No public route,
// identity activation, email proof or login exists in this slice.
type PendingRegistration struct {
	SubjectID        string
	CanonicalEmail   string
	Status           string
	PasswordVerifier domain.PasswordVerifier
	EventID          string
	CreatedAt        time.Time
}

type RegistrationStore interface {
	CreatePendingWithObligation(context.Context, PendingRegistration) error
}

// PasswordHasher is a usecase-owned port. Credential KDF code lives in the
// identity repository/credential adapter rather than in the pure domain.
type PasswordHasher interface {
	Hash(context.Context, string) (domain.PasswordVerifier, error)
}

type Registration struct {
	store  RegistrationStore
	hasher PasswordHasher
	policy domain.PasswordPolicy
	random io.Reader
	clock  func() time.Time
}

func NewRegistration(store RegistrationStore, hasher PasswordHasher, policy domain.PasswordPolicy) (*Registration, error) {
	if store == nil || hasher == nil || policy.IsBlocked == nil {
		return nil, ErrRegistrationUnavailable
	}
	return &Registration{store: store, hasher: hasher, policy: policy, random: rand.Reader, clock: time.Now}, nil
}

func newOpaqueID(r io.Reader, prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", ErrRegistrationUnavailable
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// RegisterPending is not an HTTP handler. It does not send email or expose
// the existence of an alias. Neither a session nor ACTIVE status is issued.
// RegisterPending retains the legacy no-subject internal contract.
func (s *Registration) RegisterPending(ctx context.Context, email, password string) error {
	_, err := s.RegisterPendingWithSubject(ctx, email, password)
	return err
}

// RegisterPendingWithSubject returns a subject ONLY for a freshly persisted
// account. Duplicate aliases return ("", nil); callers must never serialize
// the new subject or branch on duplicate status in an HTTP response.
// Registration and proof issuance are separate transactions: if proof issue
// fails, the pending subject remains recoverable through a later resend.
func (s *Registration) RegisterPendingWithSubject(ctx context.Context, email, password string) (string, error) {
	if s == nil {
		return "", ErrRegistrationUnavailable
	}
	alias, err := domain.CanonicalEmail(email)
	if err != nil {
		return "", err
	}
	normalized, err := s.policy.NormalizeAndValidate(password, alias)
	if err != nil {
		return "", err
	}
	verifier, err := s.hasher.Hash(ctx, normalized)
	if err != nil {
		return "", ErrRegistrationUnavailable
	}
	subject, err := newOpaqueID(s.random, "sub_")
	if err != nil {
		return "", ErrRegistrationUnavailable
	}
	event, err := newOpaqueID(s.random, "evt_")
	if err != nil {
		return "", ErrRegistrationUnavailable
	}
	err = s.store.CreatePendingWithObligation(ctx, PendingRegistration{
		SubjectID: subject, CanonicalEmail: alias, Status: "UNVERIFIED",
		PasswordVerifier: verifier, EventID: event, CreatedAt: s.clock().UTC(),
	})
	if errors.Is(err, ErrAliasAlreadyReserved) {
		return "", nil
	}
	if err != nil {
		return "", ErrRegistrationUnavailable
	}
	return subject, nil
}
