package usecase

import (
	"context"
	"errors"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

var (
	// ErrPasswordAuthenticationDenied covers absent users, incorrect passwords,
	// unverified contacts and non-active subjects. It contains no subject ID.
	ErrPasswordAuthenticationDenied = errors.New("invalid credentials")
	// ErrPasswordAuthenticationUnavailable covers SQL, KDF, corrupt verifiers
	// or canceled operations; never expose the underlying reason.
	ErrPasswordAuthenticationUnavailable = errors.New("password authentication unavailable")
)

// AuthenticationCredential is a DB-owned snapshot, not a login session or an
// authorization grant. The owning repository must never expose it to HTTP.
type AuthenticationCredential struct {
	SubjectID     string
	SessionEpoch  int64
	Status        string
	EmailVerified bool
	Verifier      domain.PasswordVerifier
}

// AuthenticatedSubject is a password-authenticated INTERNAL candidate bound to
// the exact session_epoch read alongside its verifier before Argon2id.
// It is not a browser session, identity proof or authorization grant.
type AuthenticatedSubject struct {
	SubjectID    string
	SessionEpoch int64
}

// AuthenticationCredentialReader runs only with the separate read-only
// authenticator role, not registration or email-verifier credentials.
// A missing alias must return found=false, nil without leaking existence.
type AuthenticationCredentialReader interface {
	LookupCredential(context.Context, string) (record AuthenticationCredential, found bool, err error)
}

// PasswordCandidateVerifier consumes bounded memory-hard KDF capacity.
// Never run this path without pre-KDF distributed admission in a future route.
type PasswordCandidateVerifier interface {
	Verify(context.Context, string, domain.PasswordVerifier) (bool, error)
}

// PasswordAuthentication checks one password without issuing a token,
// session, cookie or capability. It is deliberately not an HTTP handler.
type PasswordAuthentication struct {
	reader AuthenticationCredentialReader
	kdf    PasswordCandidateVerifier
}

func NewPasswordAuthentication(reader AuthenticationCredentialReader, kdf PasswordCandidateVerifier) (*PasswordAuthentication, error) {
	if reader == nil || kdf == nil {
		return nil, ErrPasswordAuthenticationUnavailable
	}
	return &PasswordAuthentication{reader: reader, kdf: kdf}, nil
}

// A static, well-formed Argon2id verifier forces the same cost class for a
// validly shaped unknown alias. Its digest is intentionally NOT a password
// secret: no session can be created from a missing/denied subject. Do not
// replace this with a fast fake hash; static salt is acceptable ONLY because
// this verifier never represents any real identity.
func dummyAuthenticationVerifier() domain.PasswordVerifier {
	return domain.PasswordVerifier{
		Version: 1, MemoryKiB: 65536, Iterations: 3, Parallelism: 1,
		Salt: []byte{
			0x91, 0x13, 0xa2, 0x4b, 0xc8, 0x6d, 0x7a, 0xe0,
			0x0f, 0x57, 0xd1, 0x2c, 0xb4, 0x68, 0x33, 0x9e,
		},
		Digest: []byte{
			0xd7, 0x3b, 0xca, 0x45, 0x12, 0xe1, 0x6f, 0x9c,
			0x84, 0x22, 0xb0, 0x7d, 0x19, 0xc6, 0x53, 0xa8,
			0x3d, 0x6e, 0x11, 0x97, 0x4a, 0xce, 0x8f, 0x20,
			0xb3, 0x55, 0x68, 0xaa, 0x29, 0x01, 0x7c, 0x5e,
		},
	}
}

// Authenticate returns a trusted internal subject candidate ONLY after a
// matched credential, ACTIVE status and independently verified contact.
// This result alone is NOT authentication to an API or resource: a future
// session issuer must independently require step-up/MFA/session controls.
// Malformed input fails generically; valid syntactic unknown accounts spend
// one real Argon2id verification to reduce KDF-based user enumeration.
func (a *PasswordAuthentication) Authenticate(ctx context.Context, email, password string) (string, error) {
	candidate, err := a.AuthenticateWithEpoch(ctx, email, password)
	return candidate.SubjectID, err
}

func (a *PasswordAuthentication) AuthenticateWithEpoch(ctx context.Context, email, password string) (AuthenticatedSubject, error) {
	if a == nil || a.reader == nil || a.kdf == nil || ctx == nil || ctx.Err() != nil {
		return AuthenticatedSubject{}, ErrPasswordAuthenticationUnavailable
	}
	canonical, err := domain.CanonicalEmail(email)
	if err != nil {
		return AuthenticatedSubject{}, ErrPasswordAuthenticationDenied
	}
	normalized, err := domain.NormalizeForVerification(password)
	if err != nil || normalized == "" {
		return AuthenticatedSubject{}, ErrPasswordAuthenticationDenied
	}
	record, found, err := a.reader.LookupCredential(ctx, canonical)
	if err != nil {
		return AuthenticatedSubject{}, ErrPasswordAuthenticationUnavailable
	}
	verifier := dummyAuthenticationVerifier()
	if found {
		verifier = record.Verifier
	}
	matches, err := a.kdf.Verify(ctx, normalized, verifier)
	if err != nil {
		// An invalid stored verifier is an integrity/availability failure,
		// not a hint that the caller supplied bad credentials.
		return AuthenticatedSubject{}, ErrPasswordAuthenticationUnavailable
	}
	if !found || !matches || record.SubjectID == "" ||
		record.Status != "ACTIVE" || !record.EmailVerified || record.SessionEpoch <= 0 {
		return AuthenticatedSubject{}, ErrPasswordAuthenticationDenied
	}
	return AuthenticatedSubject{SubjectID: record.SubjectID, SessionEpoch: record.SessionEpoch}, nil
}
