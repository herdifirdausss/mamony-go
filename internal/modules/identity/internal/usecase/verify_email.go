// Email verification is an internal restricted-authority flow.
// It is not registration, authentication, recovery or an HTTP handler.
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

var (
	ErrEmailProofUnavailable = errors.New("email verification unavailable")
	ErrEmailProofInvalid     = errors.New("email verification invalid")
	ErrEmailProofIneligible  = errors.New("email proof cannot be issued")
)

const proofPrefix = "mamony:email-proof:v1:"

// EmailProofRequest is a trusted, single-transaction repository command.
// The token digest must never be used as a bearer or logged.
type EmailProofRequest struct {
	SubjectID   string
	ChallengeID string
	EventID     string
	TokenDigest []byte
}

type EmailProofDestination struct {
	CanonicalEmail string
	ContactVersion int64
}

// EmailProofPort can be implemented only by the owning Identity capability.
// Issue locks the current subject/alias, rotates old proofs and commits a
// durable identifier-only event in the SAME transaction as the new digest.
// Consume checks authority, current contact/version, expiry and replay under
// one transaction and activates an UNVERIFIED subject atomically.
type EmailProofPort interface {
	Issue(context.Context, EmailProofRequest) (EmailProofDestination, error)
	Consume(context.Context, []byte, string) error
}

// PreparedEmailProof must only be passed to a reviewed trusted email adapter.
// There is deliberately no JSON representation or public HTTP wiring.
// Losing this in-memory token after commit requires issuing a new proof.
type PreparedEmailProof struct {
	recipient string
	token     string
}

// ForTrustedDelivery deliberately reveals a short-lived secret only to the
// internal delivery adapter. Never include its result in logs/API responses.
func (p PreparedEmailProof) ForTrustedDelivery() (recipient, secret string) {
	return p.recipient, p.token
}

func (PreparedEmailProof) String() string   { return "[REDACTED email proof]" }
func (PreparedEmailProof) GoString() string { return "[REDACTED email proof]" }
func (PreparedEmailProof) MarshalJSON() ([]byte, error) {
	return nil, errors.New("email proof cannot be marshaled to JSON")
}

type EmailVerification struct {
	store  EmailProofPort
	random io.Reader
}

func NewEmailVerification(store EmailProofPort) (*EmailVerification, error) {
	if store == nil {
		return nil, ErrEmailProofUnavailable
	}
	return &EmailVerification{store: store, random: rand.Reader}, nil
}

func tokenDigest(secret string) []byte {
	digest := sha256.Sum256([]byte(proofPrefix + secret))
	return digest[:]
}

// Prepare is not callable from HTTP. Before public registration is enabled,
// its return value must be handed directly to a vetted delivery adapter with
// bounded issuance and resend/recovery behavior.
func (v *EmailVerification) Prepare(ctx context.Context, subjectID string) (PreparedEmailProof, error) {
	if v == nil || v.store == nil || len(subjectID) != 26 || subjectID[:4] != "sub_" {
		return PreparedEmailProof{}, ErrEmailProofUnavailable
	}
	bytes := make([]byte, 32)
	if _, err := io.ReadFull(v.random, bytes); err != nil {
		return PreparedEmailProof{}, ErrEmailProofUnavailable
	}
	secret := base64.RawURLEncoding.EncodeToString(bytes)
	challengeID, err := newOpaqueID(v.random, "vch_")
	if err != nil {
		return PreparedEmailProof{}, ErrEmailProofUnavailable
	}
	eventID, err := newOpaqueID(v.random, "evt_")
	if err != nil {
		return PreparedEmailProof{}, ErrEmailProofUnavailable
	}
	target, err := v.store.Issue(ctx, EmailProofRequest{
		SubjectID: subjectID, ChallengeID: challengeID,
		EventID: eventID, TokenDigest: tokenDigest(secret),
	})
	if err != nil || target.ContactVersion < 1 || target.CanonicalEmail == "" {
		return PreparedEmailProof{}, ErrEmailProofUnavailable
	}
	return PreparedEmailProof{recipient: target.CanonicalEmail, token: secret}, nil
}

// Confirm accepts exactly one canonical 32-byte random bearer, and does not
// issue a login/session token. All failures are generic at this layer.
func (v *EmailVerification) Confirm(ctx context.Context, secret string) error {
	if v == nil || v.store == nil || len(secret) != 43 {
		return ErrEmailProofInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != secret {
		return ErrEmailProofInvalid
	}
	eventID, err := newOpaqueID(v.random, "evt_")
	if err != nil {
		return ErrEmailProofUnavailable
	}
	if err := v.store.Consume(ctx, tokenDigest(secret), eventID); err != nil {
		return ErrEmailProofInvalid
	}
	return nil
}
