// Package webauthn adapts a pinned, maintained WebAuthn protocol library.
// It is independent of Identity HTTP, PostgreSQL and the binding authority.
package webauthn

import (
	"context"
	"encoding/base64"

	"github.com/go-webauthn/webauthn/protocol"
	gowebauthn "github.com/go-webauthn/webauthn/webauthn"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

const maxRegistrationJSON = 128 << 10

// RegistrationVerifier ONLY validates WebAuthn protocol evidence. Neither
// credential persistence nor registration authorization exists in this adapter.
type RegistrationVerifier struct {
	rp     usecase.PasskeyRelyingParty
	engine *gowebauthn.WebAuthn
}

// NewRegistrationVerifier requires the same strictly pinned HTTPS RP scope as
// v30/v31 DB issuance. The caller cannot supply a per-request origin or RP ID.
func NewRegistrationVerifier(rp usecase.PasskeyRelyingParty) (*RegistrationVerifier, error) {
	if !rp.Configured() {
		return nil, usecase.ErrPasskeyProtocolUnavailable
	}
	engine, err := gowebauthn.New(&gowebauthn.Config{
		RPID:          rp.RPID(),
		RPOrigins:     []string{rp.Origin()},
		RPDisplayName: "Mamony",
		// Deliberately no related origins, opaque origins, cross-origin
		// contexts, optional UV, or relaxed unsolicited extensions.
		RPAllowCrossOrigin: false,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationRequired,
		},
	})
	if err != nil {
		return nil, usecase.ErrPasskeyProtocolUnavailable
	}
	return &RegistrationVerifier{rp: rp, engine: engine}, nil
}

// VerifyRegistration validates the whole browser response, not just the
// clientDataJSON challenge. The synthetic User is derived ONLY from the
// server-session's DB subject, never a client supplied user handle.
//
// v30 Begin sends only a nonce, not library-generated creation options. The
// trusted SessionData assembled below is an explicit, reviewed subset:
// nonce, exact RP, subject handle, UV required, approved algorithm list and
// no extension requests. Future Begin options MUST match these same values.
// Empty Expires is intentional because DB v31 already enforces the nonce TTL
// and a separate atomic finish MUST recheck it after this computation.
func (v *RegistrationVerifier) VerifyRegistration(ctx context.Context,
	expected usecase.PasskeyRegistrationEvidence, responseJSON []byte) (usecase.PasskeyRegistrationCandidate, error) {
	if v == nil || v.engine == nil || !v.rp.Configured() ||
		ctx == nil || ctx.Err() != nil ||
		len(responseJSON) == 0 || len(responseJSON) > maxRegistrationJSON ||
		expected.Subject == "" || expected.Epoch <= 0 ||
		expected.Nonce == ([32]byte{}) || expected.PolicyGeneration != 1 ||
		expected.RPID != v.rp.RPID() || expected.Origin != v.rp.Origin() {
		return usecase.PasskeyRegistrationCandidate{}, usecase.ErrPasskeyProtocolDenied
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(responseJSON)
	if err != nil || parsed == nil {
		return usecase.PasskeyRegistrationCandidate{}, usecase.ErrPasskeyProtocolDenied
	}
	user := ceremonyUser{subject: expected.Subject}
	session := gowebauthn.SessionData{
		Challenge:        base64.RawURLEncoding.EncodeToString(expected.Nonce[:]),
		RelyingPartyID:   expected.RPID,
		Origin:           expected.Origin,
		UserID:           user.WebAuthnID(),
		UserVerification: protocol.VerificationRequired,
		Mediation:        protocol.MediationDefault,
		CredParams:       gowebauthn.CredentialParametersRecommendedL3(),
	}
	credential, err := v.engine.CreateCredential(user, session, parsed)
	if err != nil || credential == nil || ctx.Err() != nil {
		// Do not propagate vendor error strings to logs, HTTP or callers.
		return usecase.PasskeyRegistrationCandidate{}, usecase.ErrPasskeyProtocolDenied
	}
	if !credential.Flags.UserPresent || !credential.Flags.UserVerified ||
		len(credential.ID) == 0 || len(credential.PublicKey) == 0 ||
		len(credential.Authenticator.AAGUID) != 16 {
		return usecase.PasskeyRegistrationCandidate{}, usecase.ErrPasskeyProtocolDenied
	}
	return usecase.PasskeyRegistrationCandidate{
		CredentialID:       append([]byte(nil), credential.ID...),
		PublicKeyCOSE:      append([]byte(nil), credential.PublicKey...),
		AAGUID:             append([]byte(nil), credential.Authenticator.AAGUID...),
		AttestationFormat:  credential.AttestationFormat,
		AttestationType:    credential.AttestationType,
		UserPresent:        credential.Flags.UserPresent,
		UserVerified:       credential.Flags.UserVerified,
		BackupEligible:     credential.Flags.BackupEligible,
		BackupState:        credential.Flags.BackupState,
		AuthenticatorCount: credential.Authenticator.SignCount,
	}, nil
}

type ceremonyUser struct{ subject string }

func (u ceremonyUser) WebAuthnID() []byte                  { return []byte(u.subject) }
func (u ceremonyUser) WebAuthnName() string                { return "Mamony identity" }
func (u ceremonyUser) WebAuthnDisplayName() string         { return "Mamony identity" }
func (u ceremonyUser) WebAuthnCredentials() []gowebauthn.Credential { return nil }
