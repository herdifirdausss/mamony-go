package usecase

import (
	"bytes"
	"context"
	"errors"
)

var (
	ErrPasskeyAssertionDenied      = errors.New("passkey activation assertion denied")
	ErrPasskeyAssertionUnavailable = errors.New("passkey activation assertion verifier unavailable")
)

const maxPasskeyActivationAssertionResponseBytes = 128 << 10

// PasskeyActivationAssertionResult reports signature-verified assertion facts
// ONLY; it does not confer account access, credential activation, recovery
// assurance or permission to reuse a v34 nonce. The result must be paired
// with its original v34 evidence in a separately fenced DB commit.
type PasskeyActivationAssertionResult struct {
	CredentialID   []byte
	Counter        uint32
	UserPresent    bool
	UserVerified   bool
	BackupEligible bool
	BackupState    bool
}

// PasskeyActivationAssertionProtocol MUST check the full webauthn.get response
// against exact server-v34 nonce, subject handle, RP ID/origin and stored
// COSE public key with the pinned maintained WebAuthn library. It has NO
// database or binding capabilities.
type PasskeyActivationAssertionProtocol interface {
	VerifyAssertion(context.Context, PasskeyActivationEvidence, []byte) (PasskeyActivationAssertionResult, error)
}

// PasskeyActivationAssertionVerifier ONLY reads the pending server-owned
// v34 assertion context and verifies a client assertion. It cannot consume
// or activate anything, and offers no public HTTP or login surface.
type PasskeyActivationAssertionVerifier struct {
	expected *PasskeyActivationEvidenceReader
	protocol PasskeyActivationAssertionProtocol
}

func NewPasskeyActivationAssertionVerifier(expected *PasskeyActivationEvidenceReader,
	protocol PasskeyActivationAssertionProtocol) (*PasskeyActivationAssertionVerifier, error) {
	if expected == nil || protocol == nil {
		return nil, ErrPasskeyAssertionUnavailable
	}
	return &PasskeyActivationAssertionVerifier{expected: expected, protocol: protocol}, nil
}

// VerifyOnly returns one read-only verification result. A repeated valid
// response can verify AGAIN while the v34 challenge exists; ONLY a later
// atomic one-use v36 DB transaction can prevent a replay from committing.
// Do NOT expose this as an authenticated session or authorization decision.
func (v *PasskeyActivationAssertionVerifier) VerifyOnly(ctx context.Context,
	bearer, challengeID string, raw []byte) (PasskeyActivationAssertionResult, error) {
	_, proof, err := v.verifyForActivation(ctx, bearer, challengeID, raw)
	return proof, err
}

// verifyForActivation preserves EXACT v34 evidence used in the signature
// verification; no re-reading a replaced nonce between Verify and Commit.
// This is package-private because only a reviewed activation
// coordinator may pair it with an independently checked SQL fence.
func (v *PasskeyActivationAssertionVerifier) verifyForActivation(ctx context.Context,
	bearer, challengeID string, raw []byte) (
	PasskeyActivationEvidence, PasskeyActivationAssertionResult, error) {
	var zero PasskeyActivationEvidence
	var none PasskeyActivationAssertionResult
	if v == nil || v.expected == nil || v.protocol == nil ||
		ctx == nil || ctx.Err() != nil {
		return zero, none, ErrPasskeyAssertionUnavailable
	}
	if len(raw) == 0 || len(raw) > maxPasskeyActivationAssertionResponseBytes {
		return zero, none, ErrPasskeyAssertionDenied
	}
	expected, err := v.expected.Read(ctx, bearer, challengeID)
	if errors.Is(err, ErrPasskeyActivationDenied) {
		return zero, none, ErrPasskeyAssertionDenied
	}
	if err != nil {
		return zero, none, ErrPasskeyAssertionUnavailable
	}
	proof, err := v.protocol.VerifyAssertion(ctx, expected, raw)
	if errors.Is(err, ErrPasskeyAssertionDenied) {
		return zero, none, ErrPasskeyAssertionDenied
	}
	if err != nil || ctx.Err() != nil {
		return zero, none, ErrPasskeyAssertionUnavailable
	}
	if !bytes.Equal(proof.CredentialID, expected.CredentialID) ||
		!proof.UserPresent || !proof.UserVerified ||
		proof.BackupEligible != expected.BackupEligible ||
		(proof.BackupState && !proof.BackupEligible) ||
		((expected.SignCount != 0 || proof.Counter != 0) &&
			proof.Counter <= expected.SignCount) {
		return zero, none, ErrPasskeyAssertionDenied
	}
	proof.CredentialID = append([]byte(nil), proof.CredentialID...)
	return expected, proof, nil
}
