package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net/netip"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

var (
	ErrPasswordChangeDenied      = errors.New("password change denied")
	ErrPasswordChangeUnavailable = errors.New("password change unavailable")
)

// PasswordChangeStore uses an independent EXECUTE-only database credential.
// A compromised change-role credential cannot read verifiers, stage proofs
// or create sessions. The trusted Go layer MUST validate the old password.
type PasswordChangeStore interface {
	Rotate(context.Context, []byte, []byte, domain.PasswordVerifier, string) (bool, error)
}

// AuthenticatedPasswordChange is internal-only. It requires both an existing
// usable opaque session and one successful admission-limited old-password
// verification bound to the SAME subject and epoch before issuing a separate
// short-lived, one-use PASSWORD_CHANGE assertion.
type AuthenticatedPasswordChange struct {
	sessions *OpaqueSessionAuthority
	current  *PasswordLoginFlow
	grants   *AuthenticationGrantAuthority
	hasher   PasswordHasher
	store    PasswordChangeStore
	policy   domain.PasswordPolicy
	random   io.Reader
}

func NewAuthenticatedPasswordChange(sessions *OpaqueSessionAuthority, current *PasswordLoginFlow,
	grants *AuthenticationGrantAuthority, hasher PasswordHasher, store PasswordChangeStore,
	policy domain.PasswordPolicy) (*AuthenticatedPasswordChange, error) {
	if sessions == nil || current == nil || grants == nil || hasher == nil ||
		store == nil || policy.IsBlocked == nil {
		return nil, ErrPasswordChangeUnavailable
	}
	return &AuthenticatedPasswordChange{sessions: sessions, current: current, grants: grants, hasher: hasher, store: store, policy: policy, random: rand.Reader}, nil
}

// Change deliberately issues NO replacement session. The previously
// authenticated session becomes invalid when the v14 epoch trigger commits.
// No HTTP endpoint is mounted and no email notifications are sent yet.
func (c *AuthenticatedPasswordChange) Change(ctx context.Context,
	sessionSecret, canonicalEmail, currentPassword, nextPassword string,
	peer netip.Addr) error {
	if c == nil || c.sessions == nil || c.current == nil || c.grants == nil ||
		c.hasher == nil || c.store == nil || c.random == nil ||
		ctx == nil || ctx.Err() != nil {
		return ErrPasswordChangeUnavailable
	}
	alias, err := domain.CanonicalEmail(canonicalEmail)
	if err != nil || alias != canonicalEmail {
		return ErrPasswordChangeDenied
	}
	subject, err := c.sessions.Resolve(ctx, sessionSecret)
	if errors.Is(err, ErrSessionInvalid) {
		return ErrPasswordChangeDenied
	}
	if err != nil {
		return ErrPasswordChangeUnavailable
	}
	candidate, err := c.current.AuthenticateWithEpoch(ctx, alias, currentPassword, peer)
	if errors.Is(err, ErrPasswordAuthenticationDenied) || errors.Is(err, ErrPasswordLoginThrottled) {
		return ErrPasswordChangeDenied
	}
	if err != nil {
		return ErrPasswordChangeUnavailable
	}
	// A correct password for another account is NOT reauthentication for
	// the current bearer. Never pass user-selected subject identity to SQL.
	if subject != candidate.SubjectID || candidate.SessionEpoch <= 0 {
		return ErrPasswordChangeDenied
	}
	normalized, err := c.policy.NormalizeAndValidate(nextPassword, alias)
	if err != nil {
		return ErrPasswordChangeDenied
	}
	oldNormalized, err := domain.NormalizeForVerification(currentPassword)
	if err != nil || normalized == oldNormalized {
		return ErrPasswordChangeDenied
	}
	verifier, err := c.hasher.Hash(ctx, normalized)
	if err != nil {
		return ErrPasswordChangeUnavailable
	}
	if verifier.Version != 1 || verifier.MemoryKiB != 65536 ||
		verifier.Iterations != 3 || verifier.Parallelism != 1 ||
		len(verifier.Salt) != 16 || len(verifier.Digest) != 32 {
		return ErrPasswordChangeUnavailable
	}
	grant, err := c.grants.IssueChange(ctx, candidate.SubjectID, candidate.SessionEpoch)
	if errors.Is(err, ErrSessionDenied) {
		return ErrPasswordChangeDenied
	}
	if err != nil {
		return ErrPasswordChangeUnavailable
	}
	sessionDigest, err := sessionTokenDigest(sessionSecret)
	if err != nil {
		return ErrPasswordChangeDenied
	}
	grantDigest, err := authenticationGrantDigest(grant.secret)
	if err != nil {
		return ErrPasswordChangeUnavailable
	}
	event, err := newOpaqueID(c.random, "evt_")
	if err != nil {
		return ErrPasswordChangeUnavailable
	}
	allowed, err := c.store.Rotate(ctx, sessionDigest[:], grantDigest[:], verifier, event)
	if err != nil {
		// A commit-ambiguous rotation may have changed the password. Never
		// retry automatically or report a definite successful rotation.
		return ErrPasswordChangeUnavailable
	}
	if !allowed {
		return ErrPasswordChangeDenied
	}
	return nil
}
