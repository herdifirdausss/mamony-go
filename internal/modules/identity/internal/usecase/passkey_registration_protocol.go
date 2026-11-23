package usecase

import (
	"context"
	"errors"
)

var (
	ErrPasskeyProtocolDenied      = errors.New("WebAuthn registration proof denied")
	ErrPasskeyProtocolUnavailable = errors.New("WebAuthn registration verifier unavailable")
)

const maxPasskeyRegistrationResponseBytes = 128 << 10

// PasskeyRegistrationCandidate contains client registration data that passed
// WebAuthn *protocol* checks against a server-origin DB nonce. It has NO
// enrollment, login, session, independent-factor, audit, or binding authority.
// A real finish MUST separately consume the intent and prove independently
// authorized account binding atomically before any credential can be stored.
type PasskeyRegistrationCandidate struct {
	CredentialID       []byte
	PublicKeyCOSE      []byte
	AAGUID             []byte
	AttestationFormat  string
	AttestationType    string
	UserPresent        bool
	UserVerified       bool
	BackupEligible     bool
	BackupState        bool
	AuthenticatorCount uint32
}

// PasskeyRegistrationProtocol is a vendor adapter. It must validate the
// entire browser response (challenge/type/origin, RP hash, UP/UV, attestation,
// COSE key and ID agreement) with a maintained WebAuthn library. It cannot
// read/alter DB state or grant registration authority.
type PasskeyRegistrationProtocol interface {
	VerifyRegistration(context.Context, PasskeyRegistrationEvidence, []byte) (PasskeyRegistrationCandidate, error)
}

// PasskeyRegistrationProofVerifier is an internal unmounted protocol-check
// orchestration; not an HTTP handler or an authenticator enrollment service.
type PasskeyRegistrationProofVerifier struct {
	expected *PasskeyRegistrationEvidenceReader
	protocol PasskeyRegistrationProtocol
}

func NewPasskeyRegistrationProofVerifier(expected *PasskeyRegistrationEvidenceReader,
	protocol PasskeyRegistrationProtocol) (*PasskeyRegistrationProofVerifier, error) {
	if expected == nil || protocol == nil {
		return nil, ErrPasskeyProtocolUnavailable
	}
	return &PasskeyRegistrationProofVerifier{expected: expected, protocol: protocol}, nil
}

// VerifyOnly is deliberately side-effect-free apart from the session/evidence
// readers. In particular no credential can be bound and no intent consumed.
// Rereading a pending intent is NOT one-use proof: atomic finish is missing.
func (v *PasskeyRegistrationProofVerifier) VerifyOnly(ctx context.Context,
	bearer, intentID string, responseJSON []byte) (PasskeyRegistrationCandidate, error) {
	if v == nil || v.expected == nil || v.protocol == nil ||
		ctx == nil || ctx.Err() != nil {
		return PasskeyRegistrationCandidate{}, ErrPasskeyProtocolUnavailable
	}
	if len(responseJSON) == 0 || len(responseJSON) > maxPasskeyRegistrationResponseBytes {
		return PasskeyRegistrationCandidate{}, ErrPasskeyProtocolDenied
	}
	expected, err := v.expected.Read(ctx, bearer, intentID)
	if errors.Is(err, ErrPasskeyRegistrationDenied) {
		return PasskeyRegistrationCandidate{}, ErrPasskeyProtocolDenied
	}
	if err != nil {
		return PasskeyRegistrationCandidate{}, ErrPasskeyProtocolUnavailable
	}
	result, err := v.protocol.VerifyRegistration(ctx, expected, responseJSON)
	if err != nil {
		if errors.Is(err, ErrPasskeyProtocolDenied) {
			return PasskeyRegistrationCandidate{}, ErrPasskeyProtocolDenied
		}
		return PasskeyRegistrationCandidate{}, ErrPasskeyProtocolUnavailable
	}
	if ctx.Err() != nil {
		return PasskeyRegistrationCandidate{}, ErrPasskeyProtocolUnavailable
	}
	if len(result.CredentialID) == 0 || len(result.CredentialID) > 1023 ||
		len(result.PublicKeyCOSE) == 0 || len(result.PublicKeyCOSE) > 2048 ||
		len(result.AAGUID) != 16 || !result.UserPresent || !result.UserVerified {
		return PasskeyRegistrationCandidate{}, ErrPasskeyProtocolDenied
	}
	// Do not alias the protocol library's potentially reused buffers.
	result.CredentialID = append([]byte(nil), result.CredentialID...)
	result.PublicKeyCOSE = append([]byte(nil), result.PublicKeyCOSE...)
	result.AAGUID = append([]byte(nil), result.AAGUID...)
	return result, nil
}
