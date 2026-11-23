package usecase

import (
	"context"
	"errors"
)

// PasskeyRegistrationEvidence is read-only, server-origin ceremony context.
// It does NOT mean a WebAuthn response was parsed or verified, and it cannot
// be used to grant, register, approve or persist any authenticator.
type PasskeyRegistrationEvidence struct {
	Subject          string
	Epoch            int64
	Nonce            [32]byte
	RPID             string
	Origin           string
	PolicyGeneration int16
}

// PasskeyRegistrationEvidenceStore must run ONLY under the purpose-specific
// EXECUTE-only PostgreSQL reader principal. The DB derives the account from
// the session digest; callers cannot select an arbitrary target subject.
type PasskeyRegistrationEvidenceStore interface {
	Read(context.Context, [32]byte, string) (PasskeyRegistrationEvidence, bool, error)
}

type PasskeyRegistrationEvidenceReader struct {
	sessions *OpaqueSessionAuthority
	store    PasskeyRegistrationEvidenceStore
	policy   PasskeyRelyingParty
}

func NewPasskeyRegistrationEvidenceReader(sessions *OpaqueSessionAuthority,
	store PasskeyRegistrationEvidenceStore, policy PasskeyRelyingParty) (*PasskeyRegistrationEvidenceReader, error) {
	if sessions == nil || store == nil || !policy.Configured() {
		return nil, ErrPasskeyRegistrationUnavailable
	}
	return &PasskeyRegistrationEvidenceReader{sessions: sessions, store: store, policy: policy}, nil
}

// Read retrieves the original DB-bound nonce only for the SAME live session,
// subject/epoch and exact configured RP policy. A future maintained WebAuthn
// library may use this as expected verification context, NEVER as evidence
// that the submitted credential is authorized to bind to this account.
//
// In particular, reading is not consumption: a future atomic finish command
// MUST independently recheck identity, session, nonce, RP policy, expiry,
// credential ownership, trusted binding admission and mandatory notice.
func (r *PasskeyRegistrationEvidenceReader) Read(ctx context.Context,
	bearer, intentID string) (PasskeyRegistrationEvidence, error) {
	if r == nil || r.sessions == nil || r.store == nil || !r.policy.Configured() ||
		ctx == nil || ctx.Err() != nil {
		return PasskeyRegistrationEvidence{}, ErrPasskeyRegistrationUnavailable
	}
	if len(intentID) != 26 || intentID[:4] != "wrg_" {
		return PasskeyRegistrationEvidence{}, ErrPasskeyRegistrationDenied
	}
	subject, err := r.sessions.Resolve(ctx, bearer)
	if errors.Is(err, ErrSessionInvalid) {
		return PasskeyRegistrationEvidence{}, ErrPasskeyRegistrationDenied
	}
	if err != nil {
		return PasskeyRegistrationEvidence{}, ErrPasskeyRegistrationUnavailable
	}
	digest, err := sessionTokenDigest(bearer)
	if err != nil {
		return PasskeyRegistrationEvidence{}, ErrPasskeyRegistrationDenied
	}
	evidence, found, err := r.store.Read(ctx, digest, intentID)
	if err != nil || ctx.Err() != nil {
		return PasskeyRegistrationEvidence{}, ErrPasskeyRegistrationUnavailable
	}
	if !found || !validSessionSubject.MatchString(evidence.Subject) ||
		evidence.Subject != subject || evidence.Epoch <= 0 ||
		evidence.Nonce == ([32]byte{}) || evidence.PolicyGeneration != 1 ||
		evidence.RPID != r.policy.RPID() || evidence.Origin != r.policy.Origin() {
		return PasskeyRegistrationEvidence{}, ErrPasskeyRegistrationDenied
	}
	return evidence, nil
}
