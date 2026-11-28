package usecase

import (
	"context"
	"errors"
	"net/netip"
)

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
