package usecase

import (
	"context"
	"errors"
)

// PasskeyActivationEvidence is a READ-ONLY, unconsumed server-origin
// assertion context. The COSE public key is NOT a client-supplied key.
// It confers no cryptographic proof, activated credential or login authority.
type PasskeyActivationEvidence struct {
	Subject          string
	Epoch            int64
	Nonce            [32]byte
	CredentialID     []byte
	PublicKeyCOSE    []byte
	AAGUID           [16]byte
	SignCount        uint32
	BackupEligible   bool
	BackupState      bool
	RPID             string
	Origin           string
	PolicyGeneration int16
}

// Only the dedicated v34 EXECUTE-only evidence reader can read a currently
// live, same-session PENDING_ASSERTION credential. No table SELECT or write.
type PasskeyActivationEvidenceStore interface {
	Read(context.Context, [32]byte, string) (PasskeyActivationEvidence, bool, error)
}

type PasskeyActivationEvidenceReader struct {
	sessions *OpaqueSessionAuthority
	store    PasskeyActivationEvidenceStore
	policy   PasskeyRelyingParty
}

func NewPasskeyActivationEvidenceReader(sessions *OpaqueSessionAuthority,
	store PasskeyActivationEvidenceStore, policy PasskeyRelyingParty) (*PasskeyActivationEvidenceReader, error) {
	if sessions == nil || store == nil || !policy.Configured() {
		return nil, ErrPasskeyActivationUnavailable
	}
	return &PasskeyActivationEvidenceReader{
		sessions: sessions, store: store, policy: policy,
	}, nil
}

// Read returns a copy of the exact expected authenticator/nonce for a
// future maintained-library assertion verifier. This does not consume
// anything; a future SQL transaction MUST independently recheck every
// original identity/nonce/counter/credential-state fence after verifying
// the authenticator signature.
func (r *PasskeyActivationEvidenceReader) Read(ctx context.Context,
	bearer, challengeID string) (PasskeyActivationEvidence, error) {
	var empty PasskeyActivationEvidence
	if r == nil || r.sessions == nil || r.store == nil ||
		!r.policy.Configured() || ctx == nil || ctx.Err() != nil {
		return empty, ErrPasskeyActivationUnavailable
	}
	if len(challengeID) != 26 || challengeID[:4] != "pav_" {
		return empty, ErrPasskeyActivationDenied
	}
	subject, err := r.sessions.Resolve(ctx, bearer)
	if errors.Is(err, ErrSessionInvalid) {
		return empty, ErrPasskeyActivationDenied
	}
	if err != nil {
		return empty, ErrPasskeyActivationUnavailable
	}
	digest, err := sessionTokenDigest(bearer)
	if err != nil {
		return empty, ErrPasskeyActivationDenied
	}
	evidence, found, err := r.store.Read(ctx, digest, challengeID)
	if err != nil || ctx.Err() != nil {
		return empty, ErrPasskeyActivationUnavailable
	}
	if !found || !validSessionSubject.MatchString(evidence.Subject) ||
		evidence.Subject != subject || evidence.Epoch <= 0 ||
		evidence.Nonce == ([32]byte{}) ||
		len(evidence.CredentialID) < 16 || len(evidence.CredentialID) > 1023 ||
		len(evidence.PublicKeyCOSE) < 16 || len(evidence.PublicKeyCOSE) > 2048 ||
		evidence.BackupState && !evidence.BackupEligible ||
		evidence.PolicyGeneration != 1 || evidence.RPID != r.policy.RPID() ||
		evidence.Origin != r.policy.Origin() {
		return empty, ErrPasskeyActivationDenied
	}
	evidence.CredentialID = append([]byte(nil), evidence.CredentialID...)
	evidence.PublicKeyCOSE = append([]byte(nil), evidence.PublicKeyCOSE...)
	return evidence, nil
}
