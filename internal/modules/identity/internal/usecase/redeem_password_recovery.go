package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"io"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

var (
	ErrPasswordRecoveryDenied      = errors.New("password recovery denied")
	ErrPasswordRecoveryUnavailable = errors.New("password recovery unavailable")
)

// PasswordRecoveryRedemptionStore has no authority to issue proofs,
// fetch account subjects or read raw password verifiers. A separate SQL
// role may EXECUTE one owner-controlled, purpose-bound recovery transaction.
// The supplied email is ONLY a policy context and DB equality fence: the
// stored token digest determines the subject, never the caller-selected alias.
type PasswordRecoveryRedemptionStore interface {
	Redeem(context.Context, [32]byte, string, domain.PasswordVerifier, string) (bool, error)
}

// PasswordRecoveryRedemption is not mounted as HTTP and has no grant issuer.
// SQL requires a separate default-deny assurance grant supplied by a future
// independent, high-assurance method; no such production grant writer exists.
type PasswordRecoveryRedemption struct {
	store  PasswordRecoveryRedemptionStore
	hasher PasswordHasher
	policy domain.PasswordPolicy
	random io.Reader
}

func NewPasswordRecoveryRedemption(store PasswordRecoveryRedemptionStore,
	hasher PasswordHasher, policy domain.PasswordPolicy) (*PasswordRecoveryRedemption, error) {
	if store == nil || hasher == nil || policy.IsBlocked == nil {
		return nil, ErrPasswordRecoveryUnavailable
	}
	return &PasswordRecoveryRedemption{
		store: store, hasher: hasher, policy: policy, random: rand.Reader,
	}, nil
}

// Redeem does NOT create a session. It performs strict token/policy/KDF
// validation and submits exactly one transactional redemption attempt.
// Unknown COMMIT is not success or a reliable rejection: NEVER auto-retry.
// No external caller should differentiate an invalid proof from an account
// not permitted by the separate assurance and subject/contact rules.
func (r *PasswordRecoveryRedemption) Redeem(ctx context.Context,
	recoveryBearer, canonicalEmail, nextPassword string) error {
	if r == nil || r.store == nil || r.hasher == nil || r.random == nil ||
		ctx == nil || ctx.Err() != nil {
		return ErrPasswordRecoveryUnavailable
	}
	email, err := domain.CanonicalEmail(canonicalEmail)
	if err != nil || email != canonicalEmail {
		return ErrPasswordRecoveryDenied
	}
	digest, err := PasswordRecoveryProofDigest(recoveryBearer)
	if err != nil {
		return ErrPasswordRecoveryDenied
	}
	normalized, err := r.policy.NormalizeAndValidate(nextPassword, email)
	if err != nil {
		return ErrPasswordRecoveryDenied
	}
	verifier, err := r.hasher.Hash(ctx, normalized)
	if err != nil || verifier.Version != 1 || verifier.MemoryKiB != 65536 ||
		verifier.Iterations != 3 || verifier.Parallelism != 1 ||
		len(verifier.Salt) != 16 || len(verifier.Digest) != 32 {
		return ErrPasswordRecoveryUnavailable
	}
	event, err := newOpaqueID(r.random, "evt_")
	if err != nil {
		return ErrPasswordRecoveryUnavailable
	}
	allowed, err := r.store.Redeem(ctx, digest, email, verifier, event)
	if err != nil {
		return ErrPasswordRecoveryUnavailable
	}
	if !allowed {
		return ErrPasswordRecoveryDenied
	}
	return nil
}
