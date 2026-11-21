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
	// Internal-only admission outcome. A future public handler MUST collapse it
	// to the same outward response used for missing/active accounts.
	ErrEmailProofThrottled = errors.New("email proof issuance limited")
)

const proofPrefix = "mamony:email-proof:v1:"

// Internal defaults for first-party Profile A email proof issuance. All
// issuance (first proof and reissue) passes through the same DB-time budgets.
// These are NOT IP or anonymous-account controls: ingress protection remains
// a separate prerequisite before any public endpoint is enabled.
const (
	EmailProofCooldownSeconds = 60
	EmailProofHourlyLimit     = 3
	EmailProofDailyLimit      = 5
)

// EmailProofRequest is a trusted, single-transaction repository command.
// The token digest must never be used as a bearer or logged.
type EmailProofRequest struct {
	OnlyIfFirst   bool // recovery must never replace an existing historical proof
	SubjectID     string
	ChallengeID   string
	EventID       string
	TokenDigest   []byte
	EnvelopeKeyID string
	SealedToken   []byte
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

// ProofEnvelopeSealer keeps the provider-ready bearer recoverable for
// controlled retries without persisting a plaintext token. Its key must be
// supplied by a trusted secret source outside PostgreSQL and the repository.
type ProofEnvelopeSealer interface {
	Seal(subjectID, challengeID, bearer string) (keyID string, ciphertext []byte, err error)
}

type EmailVerification struct {
	store  EmailProofPort
	sealer ProofEnvelopeSealer
	random io.Reader
}

func NewEmailVerification(store EmailProofPort, sealer ProofEnvelopeSealer) (*EmailVerification, error) {
	if store == nil || sealer == nil {
		return nil, ErrEmailProofUnavailable
	}
	return &EmailVerification{store: store, sealer: sealer, random: rand.Reader}, nil
}

func tokenDigest(secret string) []byte {
	digest := sha256.Sum256([]byte(proofPrefix + secret))
	return digest[:]
}

// Prepare is not callable from HTTP. Before public registration is enabled,
// its return value must be handed directly to a vetted delivery adapter with
// bounded issuance and resend/recovery behavior.
func (v *EmailVerification) Prepare(ctx context.Context, subjectID string) (PreparedEmailProof, error) {
	return v.prepare(ctx, subjectID, false)
}

// FirstIfMissing is reserved for bounded reconciliation. The repository
// rechecks that NO previous challenge exists under the subject lock, so a
// stale read-only finder result cannot trigger token rotation.
func (v *EmailVerification) FirstIfMissing(ctx context.Context, subjectID string) error {
	_, err := v.prepare(ctx, subjectID, true)
	return err
}

func (v *EmailVerification) prepare(ctx context.Context, subjectID string, onlyIfFirst bool) (PreparedEmailProof, error) {
	if v == nil || v.store == nil || v.sealer == nil || len(subjectID) != 26 || subjectID[:4] != "sub_" {
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
	keyID, sealed, err := v.sealer.Seal(subjectID, challengeID, secret)
	if err != nil || len(keyID) == 0 || len(sealed) < 60 || len(sealed) > 256 {
		return PreparedEmailProof{}, ErrEmailProofUnavailable
	}
	target, err := v.store.Issue(ctx, EmailProofRequest{
		SubjectID: subjectID, ChallengeID: challengeID, OnlyIfFirst: onlyIfFirst,
		EventID: eventID, TokenDigest: tokenDigest(secret),
		EnvelopeKeyID: keyID, SealedToken: sealed,
	})
	if errors.Is(err, ErrEmailProofThrottled) {
		// Caller is an already-authorized internal issuer, never an HTTP
		// endpoint. Public handlers must not expose this distinct outcome.
		return PreparedEmailProof{}, ErrEmailProofThrottled
	}
	if err != nil || target.ContactVersion < 1 || target.CanonicalEmail == "" {
		return PreparedEmailProof{}, ErrEmailProofUnavailable
	}
	return PreparedEmailProof{recipient: target.CanonicalEmail, token: secret}, nil
}

// Resend always creates a NEW single-use token, revoking the old token only
// on successful commit. No token is returned to its caller: only the trusted
// encrypted delivery outbox receives the proof. This is internal-only; the
// future ingress must authenticate/authorize the subject and normalize errors.
func (v *EmailVerification) Resend(ctx context.Context, subjectID string) error {
	_, err := v.Prepare(ctx, subjectID)
	return err
}

// Confirm accepts exactly one canonical 32-byte random bearer and never
// creates a session. Only an authoritative negative proof decision returns
// ErrEmailProofInvalid; all failures of the owning verifier authority remain
// ErrEmailProofUnavailable without leaking their underlying error chains.
func (v *EmailVerification) Confirm(ctx context.Context, secret string) error {
	if v == nil || v.store == nil || ctx == nil || ctx.Err() != nil {
		return ErrEmailProofUnavailable
	}
	if len(secret) != 43 {
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
		if errors.Is(err, ErrEmailProofInvalid) {
			return ErrEmailProofInvalid
		}
		return ErrEmailProofUnavailable
	}
	return nil
}
