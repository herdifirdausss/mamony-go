package usecase

import (
	"context"
	"errors"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

var ErrEnrollmentUnavailable = errors.New("identity enrollment unavailable")

// EnrollmentRegistrar returns a subject only when a new pending registration
// actually commits. The subject is trusted internal data, never an API field.
type EnrollmentRegistrar interface {
	RegisterPendingWithSubject(context.Context, string, string) (string, error)
}

type EnrollmentSubjectLookup interface {
	FindUnverifiedSubject(context.Context, string) (string, error)
}

type EnrollmentProofIssuer interface {
	Resend(context.Context, string) error
}

// EnrollmentFlow orchestrates internal writes AFTER an independent anonymous
// admission gate has authorized the request budget. It is not a public route,
// login flow, or proof of ownership of the submitted email address.
type EnrollmentFlow struct {
	registration EnrollmentRegistrar
	lookup EnrollmentSubjectLookup
	proof EnrollmentProofIssuer
}

func NewEnrollmentFlow(reg EnrollmentRegistrar, lookup EnrollmentSubjectLookup, proof EnrollmentProofIssuer) (*EnrollmentFlow, error) {
	if reg == nil || lookup == nil || proof == nil {
		return nil, ErrEnrollmentUnavailable
	}
	return &EnrollmentFlow{registration: reg, lookup: lookup, proof: proof}, nil
}

// Register stores a new pending subject with its enrollment event and then
// persists a separate encrypted verification proof. A crash between these
// transactions can leave an UNVERIFIED subject without a proof; a later
// admitted resend can recover it. The HTTP caller MUST NOT disclose results.
func (f *EnrollmentFlow) Register(ctx context.Context, email, password string) error {
	if f == nil || f.registration == nil || f.proof == nil {
		return ErrEnrollmentUnavailable
	}
	subject, err := f.registration.RegisterPendingWithSubject(ctx, email, password)
	if errors.Is(err, domain.ErrInvalidEmail) || errors.Is(err, domain.ErrInvalidPassword) {
		return err
	}
	if err != nil {
		return ErrEnrollmentUnavailable
	}
	if subject == "" {
		// Existing alias. Do NOT issue/rotate proof from registration.
		return nil
	}
	if err := f.proof.Resend(ctx, subject); err != nil {
		return ErrEnrollmentUnavailable
	}
	return nil
}

// Resend resolves the subject on the server, never from request JSON. Unknown,
// active or already verified accounts are deliberately no-ops. A proof may
// still be rejected by the separate subject-level cooldown or day quota.
func (f *EnrollmentFlow) Resend(ctx context.Context, email string) error {
	if f == nil || f.lookup == nil || f.proof == nil {
		return ErrEnrollmentUnavailable
	}
	canonical, err := domain.CanonicalEmail(email)
	if err != nil {
		return domain.ErrInvalidEmail
	}
	subject, err := f.lookup.FindUnverifiedSubject(ctx, canonical)
	if err != nil {
		return ErrEnrollmentUnavailable
	}
	if subject == "" {
		return nil
	}
	if err := f.proof.Resend(ctx, subject); err != nil {
		if errors.Is(err, ErrEmailProofThrottled) {
			return nil
		}
		return ErrEnrollmentUnavailable
	}
	return nil
}
