package usecase

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
)

var (
	ErrPasskeyActivationCommitDenied      = errors.New("passkey possession commit denied")
	ErrPasskeyActivationCommitUnavailable = errors.New("passkey possession commit unavailable")
)

// PasskeyActivationFinishRequest is the ONE cryptographically verified v35
// snapshot paired with its original v34 nonce. It is not an approval, session,
// or independent recovery factor. The isolated SQL authority MUST compare
// every expected value to freshly locked server rows before any transition.
type PasskeyActivationFinishRequest struct {
	SessionDigest [32]byte
	ChallengeID   string
	Expected      PasskeyActivationEvidence
	Verified      PasskeyActivationAssertionResult
	EventID       string
}

// PasskeyActivationFinishStore is deliberately absent from HTTP/runtime wiring.
// The separately privileged SQL adapter in draft PR #81 implements nonce consumption,
// credential state/counter transition, identifier-only audit and mandatory
// notice as ONE transaction. No caller may implement this with independent
// UPDATEs or use its boolean result as a login/recovery authorization.
type PasskeyActivationFinishStore interface {
	Finish(context.Context, PasskeyActivationFinishRequest) (bool, error)
}

// PasskeyActivationCommitter prepares an exact signature-to-SQL handoff.
// It remains unmounted. Draft migration 000035 and a concrete SQL adapter
// can persist POSSESSION_VERIFIED only, with no ACTIVE/login authority.
// Neither draft component is approved for deployment or public use.
type PasskeyActivationCommitter struct {
	proof  *PasskeyActivationAssertionVerifier
	store  PasskeyActivationFinishStore
	random io.Reader
}

func NewPasskeyActivationCommitter(
	proof *PasskeyActivationAssertionVerifier,
	store PasskeyActivationFinishStore,
) (*PasskeyActivationCommitter, error) {
	if proof == nil || proof.expected == nil || proof.protocol == nil || store == nil {
		return nil, ErrPasskeyActivationCommitUnavailable
	}
	return &PasskeyActivationCommitter{proof: proof, store: store, random: rand.Reader}, nil
}

// Finish never accepts client-asserted subject, credential key, flags or event
// ID. The verifier returns ORIGINAL v34 DB evidence alongside verified signed
// WebAuthn assertion facts. The session digest is domain-separated, and the
// audit event has independent server-side entropy.
//
// The store is required to reject stale/overwritten evidence under fresh SQL
// locks, including nonce ID+bytes, COSE key, RP, epoch, session, flags and
// previous counter. This method does NOT implement that database fence.
// Unknown COMMIT outcome always maps to UNAVAILABLE and is never retried.
func (c *PasskeyActivationCommitter) Finish(ctx context.Context,
	bearer, challengeID string, rawAssertion []byte) error {
	if c == nil || c.proof == nil || c.store == nil || c.random == nil ||
		ctx == nil || ctx.Err() != nil {
		return ErrPasskeyActivationCommitUnavailable
	}

	expected, verified, err := c.proof.verifyForActivation(
		ctx, bearer, challengeID, rawAssertion)
	if errors.Is(err, ErrPasskeyAssertionDenied) {
		return ErrPasskeyActivationCommitDenied
	}
	if err != nil || ctx.Err() != nil {
		return ErrPasskeyActivationCommitUnavailable
	}

	// Defence-in-depth before handing the *signed* facts to the separately
	// privileged transaction. A compromised or replaced store is not an
	// independent cryptographic verifier (Profile A residual risk).
	if expected.Subject == "" || expected.Epoch <= 0 ||
		expected.Nonce == ([32]byte{}) || expected.PolicyGeneration != 1 ||
		len(expected.CredentialID) < 16 || len(expected.CredentialID) > 1023 ||
		len(expected.PublicKeyCOSE) < 16 || len(expected.PublicKeyCOSE) > 2048 ||
		!bytes.Equal(expected.CredentialID, verified.CredentialID) ||
		!verified.UserPresent || !verified.UserVerified ||
		verified.BackupEligible != expected.BackupEligible ||
		(verified.BackupState && !verified.BackupEligible) ||
		((expected.SignCount != 0 || verified.Counter != 0) &&
			verified.Counter <= expected.SignCount) {
		return ErrPasskeyActivationCommitDenied
	}

	digest, err := sessionTokenDigest(bearer)
	if err != nil {
		return ErrPasskeyActivationCommitDenied
	}
	eventID, err := newOpaqueID(c.random, "evt_")
	if err != nil {
		return ErrPasskeyActivationCommitUnavailable
	}

	// Deep copy every mutable byte slice. The store never receives raw
	// assertion bytes, a bearer, or caller-derived account authority.
	expected.CredentialID = append([]byte(nil), expected.CredentialID...)
	expected.PublicKeyCOSE = append([]byte(nil), expected.PublicKeyCOSE...)
	verified.CredentialID = append([]byte(nil), verified.CredentialID...)

	ok, err := c.store.Finish(ctx, PasskeyActivationFinishRequest{
		SessionDigest: digest,
		ChallengeID:   challengeID,
		Expected:      expected,
		Verified:      verified,
		EventID:       eventID,
	})
	if err != nil || ctx.Err() != nil {
		// Includes uncertain SQL COMMIT acknowledgement. Never replay here.
		return ErrPasskeyActivationCommitUnavailable
	}
	if !ok {
		return ErrPasskeyActivationCommitDenied
	}
	// This is only the future store's possession-commit result; it does not
	// confer login, a session, independent factor assurance or authorization.
	return nil
}
