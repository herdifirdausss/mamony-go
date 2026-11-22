package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"net/netip"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

var ErrRecoveryRequestUnavailable = errors.New("password recovery request unavailable")

// RecoveryIssuerPort returns an internal-only subject for a currently verified
// eligible alias. It never grants authentication and is never a public API.
// Issue rechecks subject/contact status, version/epoch and DB-time quotas
// under locks before inserting digest and encrypted delivery atomically.
type RecoveryIssuerPort interface {
	FindEligible(context.Context, string) (string, error)
	Issue(context.Context, RecoveryIssue) (bool, error)
}

type RecoveryIssue struct {
	SubjectID      string
	CanonicalEmail string
	ChallengeID    string
	TokenDigest    [32]byte
	KeyID          string
	SealedToken    []byte
}

// RecoveryEnvelopeSealer is deliberately distinct from email verification
// encryption. A production implementation needs an independently scoped
// AEAD key and AAD binding recovery purpose, subject and challenge.
type RecoveryEnvelopeSealer interface {
	SealRecovery(subjectID, challengeID, bearer string) (keyID string, sealed []byte, err error)
}

// RecoveryRequest orchestrates only admitted internal issuance. No public
// endpoint can infer whether the subject exists, is active or is throttled.
// Token material is never returned to a request caller, persisted plaintext,
// or made available to a login/session/verification path.
type RecoveryRequest struct {
	gate   *RecoveryAdmission
	issuer RecoveryIssuerPort
	sealer RecoveryEnvelopeSealer
}

func NewRecoveryRequest(gate *RecoveryAdmission, issuer RecoveryIssuerPort, sealer RecoveryEnvelopeSealer) (*RecoveryRequest, error) {
	if gate == nil || gate.store == nil || issuer == nil || sealer == nil {
		return nil, ErrRecoveryRequestUnavailable
	}
	return &RecoveryRequest{gate: gate, issuer: issuer, sealer: sealer}, nil
}

func (r *RecoveryRequest) Request(ctx context.Context, rawEmail string, peer netip.Addr) error {
	if r == nil || r.gate == nil || r.issuer == nil || r.sealer == nil ||
		ctx == nil || ctx.Err() != nil {
		return ErrRecoveryRequestUnavailable
	}
	email, err := domain.CanonicalEmail(rawEmail)
	if err != nil || email != rawEmail {
		return ErrRecoveryAdmissionInvalid
	}
	allowed, err := r.gate.Admit(ctx, email, peer)
	if err != nil {
		return ErrRecoveryRequestUnavailable
	}
	// A budget denial gives the same internal nil result as an unknown or
	// ineligible subject. A future public handler must still use a single,
	// timing-normalized generic response on ALL syntactically valid requests.
	if !allowed {
		return nil
	}
	subject, err := r.issuer.FindEligible(ctx, email)
	if err != nil {
		return ErrRecoveryRequestUnavailable
	}
	if subject == "" {
		return nil
	}
	proof, err := NewPasswordRecoveryProof()
	if err != nil {
		return ErrRecoveryRequestUnavailable
	}
	challengeID, err := newOpaqueID(rand.Reader, "rcv_")
	if err != nil {
		return ErrRecoveryRequestUnavailable
	}
	key, sealed, err := r.sealer.SealRecovery(subject, challengeID, proof.ForTrustedDelivery())
	if err != nil || len(key) == 0 || len(key) > 32 ||
		len(sealed) < 60 || len(sealed) > 256 {
		return ErrRecoveryRequestUnavailable
	}
	_, err = r.issuer.Issue(ctx, RecoveryIssue{
		SubjectID: subject, CanonicalEmail: email, ChallengeID: challengeID,
		TokenDigest: proof.Digest(), KeyID: key, SealedToken: sealed,
	})
	if err != nil {
		// Includes ambiguous COMMIT. No auto-retry, no claim that a token
		// was or was not issued, and never expose internal database errors.
		return ErrRecoveryRequestUnavailable
	}
	return nil
}
