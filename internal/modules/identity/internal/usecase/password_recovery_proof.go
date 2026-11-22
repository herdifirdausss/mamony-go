package usecase

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
)

var (
	ErrPasswordRecoveryProofInvalid     = errors.New("password recovery proof invalid")
	ErrPasswordRecoveryProofUnavailable = errors.New("password recovery proof unavailable")
)

const (
	passwordRecoveryProofBytes  = 32
	passwordRecoveryProofLength = 43
	// Distinct from both email verification proofs and session/grant digests.
	// Future SQL consumers must use this exact v1 digest contract.
	passwordRecoveryProofDomain = "mamony:identity:password-recovery-proof:v1\x00"
)

// PasswordRecoveryProof is a 256-bit opaque bearer intended ONLY for a
// future, separately authorized account-recovery challenge. It is not a
// password verifier, authenticated session, identity assertion, recovery
// request, or permission to change credentials.
//
// This package intentionally provides NO public handler, issuance store,
// SQL role, email sender, redemption method or password-reset operation.
// A future implementation must atomically persist a digest with an expiry,
// a single-use fence and the verified subject/contact epoch before delivery.
type PasswordRecoveryProof struct {
	bearer string
	digest [32]byte
}

// NewPasswordRecoveryProof uses the operating system's cryptographically
// secure RNG. Production must not substitute deterministic test entropy.
func NewPasswordRecoveryProof() (PasswordRecoveryProof, error) {
	return newPasswordRecoveryProof(rand.Reader)
}

func newPasswordRecoveryProof(source io.Reader) (PasswordRecoveryProof, error) {
	if source == nil {
		return PasswordRecoveryProof{}, ErrPasswordRecoveryProofUnavailable
	}
	raw := make([]byte, passwordRecoveryProofBytes)
	if _, err := io.ReadFull(source, raw); err != nil {
		clear(raw)
		return PasswordRecoveryProof{}, ErrPasswordRecoveryProofUnavailable
	}
	// The token is encoded only for the future trusted delivery adapter,
	// never for HTTP responses or application logs.
	bearer := base64.RawURLEncoding.EncodeToString(raw)
	digest := passwordRecoveryDigest(raw)
	clear(raw)
	return PasswordRecoveryProof{bearer: bearer, digest: digest}, nil
}

// ForTrustedDelivery is the ONLY accessor for raw bearer material. Callers
// must establish their own encrypted, durable, authenticated delivery channel
// and may never return this value to a recovery requester.
func (p PasswordRecoveryProof) ForTrustedDelivery() string { return p.bearer }

// Digest returns a copy of the domain-separated SHA-256 token digest. Only
// a vetted, purpose-bound, once-only database challenge may consume it.
func (p PasswordRecoveryProof) Digest() [32]byte { return p.digest }

// PasswordRecoveryProofDigest strictly parses a candidate and derives its
// domain-separated digest, without making any claim that a challenge exists,
// is fresh, belongs to the requester, or is eligible for account recovery.
func PasswordRecoveryProofDigest(bearer string) ([32]byte, error) {
	var empty [32]byte
	if len(bearer) != passwordRecoveryProofLength {
		return empty, ErrPasswordRecoveryProofInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(bearer)
	if err != nil || len(raw) != passwordRecoveryProofBytes ||
		base64.RawURLEncoding.EncodeToString(raw) != bearer {
		return empty, ErrPasswordRecoveryProofInvalid
	}
	digest := passwordRecoveryDigest(raw)
	clear(raw)
	return digest, nil
}

func passwordRecoveryDigest(raw []byte) [32]byte {
	h := sha256.New()
	_, _ = h.Write([]byte(passwordRecoveryProofDomain))
	_, _ = h.Write(raw)
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

// Never let general-purpose formatting, logging, or JSON/text serialization
// accidentally publish a password-reset bearer. The explicit trusted
// delivery accessor above is intentionally narrow and visually conspicuous.
const redactedPasswordRecoveryProof = "[REDACTED password recovery proof]"

func (PasswordRecoveryProof) String() string   { return redactedPasswordRecoveryProof }
func (PasswordRecoveryProof) GoString() string { return redactedPasswordRecoveryProof }
func (PasswordRecoveryProof) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, redactedPasswordRecoveryProof)
}
func (PasswordRecoveryProof) LogValue() slog.Value {
	return slog.StringValue(redactedPasswordRecoveryProof)
}
func (PasswordRecoveryProof) MarshalJSON() ([]byte, error) {
	return nil, ErrPasswordRecoveryProofInvalid
}
func (PasswordRecoveryProof) MarshalText() ([]byte, error) {
	return nil, ErrPasswordRecoveryProofInvalid
}
