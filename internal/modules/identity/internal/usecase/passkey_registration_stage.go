package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
)

var (
	ErrPasskeyStageDenied      = errors.New("passkey registration staging denied")
	ErrPasskeyStageUnavailable = errors.New("passkey registration staging unavailable")
)

// PasskeyRegistrationStageStore is restricted to the separate v33
// EXECUTE-only principal. Its command compares the exact v31 evidence with
// v30 challenge and v32 one-use password admission in ONE DB transaction.
// It cannot directly insert a credential, mark an authenticator active,
// issue login sessions or create an independent recovery factor.
type PasskeyRegistrationStageStore interface {
	Stage(context.Context, [32]byte, string,
		PasskeyRegistrationEvidence, PasskeyRegistrationCandidate, string) (bool, error)
}

// PasskeyRegistrationStager is intentionally unmounted. It ONLY stages
// inactive candidates after maintained-library protocol verification and
// previously committed v32 password reauthentication. Stage success does
// NOT mean the user owns the private key or can authenticate with it.
type PasskeyRegistrationStager struct {
	proof  *PasskeyRegistrationProofVerifier
	store  PasskeyRegistrationStageStore
	random io.Reader
}

func NewPasskeyRegistrationStager(proof *PasskeyRegistrationProofVerifier,
	store PasskeyRegistrationStageStore) (*PasskeyRegistrationStager, error) {
	if proof == nil || proof.expected == nil || proof.protocol == nil || store == nil {
		return nil, ErrPasskeyStageUnavailable
	}
	return &PasskeyRegistrationStager{
		proof: proof, store: store, random: rand.Reader,
	}, nil
}

// Stage consumes the same-session v30 challenge and v32 admission only if
// the v33 database command atomically stages a PENDING_ASSERTION candidate,
// an identifier-only audit event and a PENDING security notice. It returns
// no candidate or credential data. No automatic retry after an ambiguous
// commit: the nonce/admission may already be spent.
func (s *PasskeyRegistrationStager) Stage(ctx context.Context,
	bearer, intentID string, responseJSON []byte) error {
	if s == nil || s.proof == nil || s.store == nil || s.random == nil ||
		ctx == nil || ctx.Err() != nil {
		return ErrPasskeyStageUnavailable
	}
	evidence, candidate, err := s.proof.verifyForStage(
		ctx, bearer, intentID, responseJSON)
	if errors.Is(err, ErrPasskeyProtocolDenied) {
		return ErrPasskeyStageDenied
	}
	if err != nil {
		return ErrPasskeyStageUnavailable
	}
	// Separate bounds on untrusted vendor output before a privileged DB call.
	if len(candidate.CredentialID) < 16 || len(candidate.CredentialID) > 1023 ||
		len(candidate.PublicKeyCOSE) < 16 || len(candidate.PublicKeyCOSE) > 2048 ||
		len(candidate.AAGUID) != 16 || !candidate.UserPresent ||
		!candidate.UserVerified || candidate.BackupState && !candidate.BackupEligible ||
		len(candidate.AttestationFormat) == 0 || len(candidate.AttestationFormat) > 64 ||
		len(candidate.AttestationType) == 0 || len(candidate.AttestationType) > 64 {
		return ErrPasskeyStageDenied
	}
	sessionDigest, err := sessionTokenDigest(bearer)
	if err != nil {
		return ErrPasskeyStageDenied
	}
	event, err := newOpaqueID(s.random, "evt_")
	if err != nil {
		return ErrPasskeyStageUnavailable
	}
	ok, err := s.store.Stage(ctx, sessionDigest, intentID, evidence, candidate, event)
	if err != nil || ctx.Err() != nil {
		return ErrPasskeyStageUnavailable
	}
	if !ok {
		return ErrPasskeyStageDenied
	}
	return nil
}
