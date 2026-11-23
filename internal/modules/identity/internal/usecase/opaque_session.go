package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"regexp"
)

var (
	ErrSessionDenied      = errors.New("session issuance denied")
	ErrSessionInvalid     = errors.New("invalid or inactive session")
	ErrSessionUnavailable = errors.New("session authority unavailable")
)

var validSessionSubject = regexp.MustCompile("^sub_[A-Za-z0-9_-]{22}$")

const sessionDigestDomain = "mamony:identity:session-token:v1\x00"

// OpaqueSession is an unlogged, unmarshalable single-issue bearer. The only
// extraction method is explicitly intended for a future trusted TLS transport.
// This is not a cookie, a public response, or an authorization grant.
type OpaqueSession struct {
	secret string
}

func (s OpaqueSession) ForTrustedTransport() string { return s.secret }
func (OpaqueSession) String() string                { return "[opaque session redacted]" }
func (OpaqueSession) GoString() string              { return "[opaque session redacted]" }
func (OpaqueSession) MarshalJSON() ([]byte, error)  { return nil, ErrSessionUnavailable }
func (OpaqueSession) MarshalText() ([]byte, error)  { return nil, ErrSessionUnavailable }

// SessionAuthorityStore consumes ONLY a restricted, one-use authentication
// grant digest and a random session digest. It no longer accepts a caller-
// selected subject; PostgreSQL resolves the subject from its grant authority.
// A compromised grant issuer runtime still defeats this Profile A boundary.
type SessionAuthorityStore interface {
	Issue(context.Context, []byte, []byte, string) (bool, error)
	Resolve(context.Context, []byte) (string, error)
	Revoke(context.Context, []byte, string) error
}

type OpaqueSessionAuthority struct {
	store  SessionAuthorityStore
	random io.Reader
}

func NewOpaqueSessionAuthority(store SessionAuthorityStore) (*OpaqueSessionAuthority, error) {
	if store == nil {
		return nil, ErrSessionUnavailable
	}
	return &OpaqueSessionAuthority{store: store, random: rand.Reader}, nil
}

func sessionTokenDigest(secret string) ([32]byte, error) {
	var digest [32]byte
	if len(secret) != 43 {
		return digest, ErrSessionInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != secret {
		return digest, ErrSessionInvalid
	}
	// 256 bits of entropy: a plain domain-separated SHA-256 is sufficient to
	// prevent offline recovery; no low-entropy password hashes are stored here.
	material := make([]byte, 0, len(sessionDigestDomain)+32)
	material = append(material, sessionDigestDomain...)
	material = append(material, decoded...)
	digest = sha256.Sum256(material)
	return digest, nil
}

func (a *OpaqueSessionAuthority) Issue(ctx context.Context, grant AuthenticationGrant) (OpaqueSession, error) {
	if a == nil || a.store == nil || a.random == nil || ctx == nil || ctx.Err() != nil {
		return OpaqueSession{}, ErrSessionUnavailable
	}
	grantDigest, err := authenticationGrantDigest(grant.secret)
	if err != nil {
		return OpaqueSession{}, ErrSessionDenied
	}
	var token [32]byte
	var event [16]byte
	if _, err := io.ReadFull(a.random, token[:]); err != nil {
		return OpaqueSession{}, ErrSessionUnavailable
	}
	if _, err := io.ReadFull(a.random, event[:]); err != nil {
		return OpaqueSession{}, ErrSessionUnavailable
	}
	secret := base64.RawURLEncoding.EncodeToString(token[:])
	digest, _ := sessionTokenDigest(secret)
	allowed, err := a.store.Issue(ctx, grantDigest[:], digest[:], "evt_"+base64.RawURLEncoding.EncodeToString(event[:]))
	if err != nil {
		// Including ambiguous commit: the bearer is deliberately discarded;
		// NEVER reissue/replay automatically or leak token through an error.
		return OpaqueSession{}, ErrSessionUnavailable
	}
	if !allowed {
		return OpaqueSession{}, ErrSessionDenied
	}
	return OpaqueSession{secret: secret}, nil
}

func (a *OpaqueSessionAuthority) Resolve(ctx context.Context, secret string) (string, error) {
	if a == nil || a.store == nil || ctx == nil || ctx.Err() != nil {
		return "", ErrSessionUnavailable
	}
	digest, err := sessionTokenDigest(secret)
	if err != nil {
		return "", ErrSessionInvalid
	}
	subject, err := a.store.Resolve(ctx, digest[:])
	if err != nil {
		return "", ErrSessionUnavailable
	}
	if !validSessionSubject.MatchString(subject) {
		return "", ErrSessionInvalid
	}
	return subject, nil
}

func (a *OpaqueSessionAuthority) Revoke(ctx context.Context, secret string) error {
	if a == nil || a.store == nil || a.random == nil || ctx == nil || ctx.Err() != nil {
		return ErrSessionUnavailable
	}
	digest, err := sessionTokenDigest(secret)
	if err != nil {
		return ErrSessionInvalid
	}
	var event [16]byte
	if _, err := io.ReadFull(a.random, event[:]); err != nil {
		return ErrSessionUnavailable
	}
	if err := a.store.Revoke(ctx, digest[:], "evt_"+base64.RawURLEncoding.EncodeToString(event[:])); err != nil {
		return ErrSessionUnavailable
	}
	// Unknown and already-revoked tokens receive the SAME internal outcome.
	return nil
}

// SessionLogin may be wired only inside the Identity composition root.
// It chains durable pre-KDF admission -> Argon2id -> grant staging -> atomic
// grant consumption/session issuance; no public route invokes raw grant stage.
type SessionLogin struct {
	candidate *PasswordLoginFlow
	grant     *AuthenticationGrantAuthority
	issuer    SessionIssuer
}

// SessionIssuer is the only independently injected authority; the candidate
// must be the concrete, mandatory-admission PasswordLoginFlow from PR #40.
type SessionIssuer interface {
	Issue(context.Context, AuthenticationGrant) (OpaqueSession, error)
}

func NewSessionLogin(candidate *PasswordLoginFlow, grant *AuthenticationGrantAuthority, issuer SessionIssuer) (*SessionLogin, error) {
	if candidate == nil || grant == nil || issuer == nil {
		return nil, ErrSessionUnavailable
	}
	return &SessionLogin{candidate: candidate, grant: grant, issuer: issuer}, nil
}

func (s *SessionLogin) Authenticate(ctx context.Context, email, password string, peer netip.Addr) (OpaqueSession, error) {
	if s == nil || s.candidate == nil || s.grant == nil || s.issuer == nil || ctx == nil || ctx.Err() != nil {
		return OpaqueSession{}, ErrSessionUnavailable
	}
	candidate, err := s.candidate.AuthenticateWithEpoch(ctx, email, password, peer)
	if err != nil {
		if errors.Is(err, ErrPasswordAuthenticationDenied) {
			return OpaqueSession{}, ErrPasswordAuthenticationDenied
		}
		if errors.Is(err, ErrPasswordLoginThrottled) {
			return OpaqueSession{}, ErrPasswordLoginThrottled
		}
		return OpaqueSession{}, ErrSessionUnavailable
	}
	if !validSessionSubject.MatchString(candidate.SubjectID) || candidate.SessionEpoch <= 0 {
		return OpaqueSession{}, ErrSessionUnavailable
	}
	proof, err := s.grant.Issue(ctx, candidate.SubjectID, candidate.SessionEpoch)
	if errors.Is(err, ErrSessionDenied) {
		return OpaqueSession{}, ErrPasswordAuthenticationDenied
	}
	if err != nil {
		return OpaqueSession{}, ErrSessionUnavailable
	}
	session, err := s.issuer.Issue(ctx, proof)
	if errors.Is(err, ErrSessionDenied) {
		return OpaqueSession{}, ErrPasswordAuthenticationDenied
	}
	if err != nil || session.secret == "" {
		return OpaqueSession{}, ErrSessionUnavailable
	}
	return session, nil
}

// Explicitly prevent accidental exposure through unstructured formatting.
var _ fmt.Stringer = OpaqueSession{}
