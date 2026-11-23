package webauthn

import (
	"bytes"
	"context"
	"encoding/base64"

	"github.com/go-webauthn/webauthn/protocol"
	gowebauthn "github.com/go-webauthn/webauthn/webauthn"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

const maxAssertionJSON = 128 << 10

// ActivationAssertionVerifier validates a real WebAuthn *authentication*
// ceremony against only the original server-v34 pending credential.
//
// This adapter has NO database access, challenge consumption, credential
// activation, passkey login, session issuance or recovery-factor authority.
type ActivationAssertionVerifier struct {
	rp     usecase.PasskeyRelyingParty
	engine *gowebauthn.WebAuthn
}

func NewActivationAssertionVerifier(rp usecase.PasskeyRelyingParty) (*ActivationAssertionVerifier, error) {
	if !rp.Configured() {
		return nil, usecase.ErrPasskeyAssertionUnavailable
	}
	engine, err := gowebauthn.New(&gowebauthn.Config{
		RPID:               rp.RPID(),
		RPOrigins:          []string{rp.Origin()},
		RPDisplayName:      "Mamony",
		RPAllowCrossOrigin: false,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationRequired,
		},
	})
	if err != nil {
		return nil, usecase.ErrPasskeyAssertionUnavailable
	}
	return &ActivationAssertionVerifier{rp: rp, engine: engine}, nil
}

// VerifyAssertion is cryptographic but SIDE-EFFECT FREE. It assembles
// SessionData from the original DB-v34 challenge, NOT from client input.
// The only allowed credential is the exact staged credential owned by
// the DB-derived subject, with the original COSE public key. The verifier
// requires UP+UV and checks clone/counter-warning and BE consistency.
//
// Since v34 Begin does not yet publish browser assertion options, the
// explicitly constructed SessionData is a reviewed subset; a future
// public Begin must exactly match these values. DB v34 enforces expiry,
// and v36 must recheck it under write locks. Zero Expires is intentional.
func (v *ActivationAssertionVerifier) VerifyAssertion(ctx context.Context,
	expected usecase.PasskeyActivationEvidence, responseJSON []byte) (
	usecase.PasskeyActivationAssertionResult, error) {
	var denied usecase.PasskeyActivationAssertionResult
	if v == nil || v.engine == nil || !v.rp.Configured() ||
		ctx == nil || ctx.Err() != nil || len(responseJSON) == 0 ||
		len(responseJSON) > maxAssertionJSON ||
		expected.Subject == "" || expected.Epoch <= 0 ||
		expected.Nonce == ([32]byte{}) || expected.PolicyGeneration != 1 ||
		expected.RPID != v.rp.RPID() || expected.Origin != v.rp.Origin() ||
		len(expected.CredentialID) < 16 || len(expected.CredentialID) > 1023 ||
		len(expected.PublicKeyCOSE) < 16 || len(expected.PublicKeyCOSE) > 2048 ||
		(expected.BackupState && !expected.BackupEligible) {
		return denied, usecase.ErrPasskeyAssertionDenied
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(responseJSON)
	if err != nil || parsed == nil {
		return denied, usecase.ErrPasskeyAssertionDenied
	}
	if !bytes.Equal(parsed.RawID, expected.CredentialID) {
		return denied, usecase.ErrPasskeyAssertionDenied
	}
	flags := protocol.FlagUserPresent | protocol.FlagUserVerified
	if expected.BackupEligible {
		flags |= protocol.FlagBackupEligible
	}
	if expected.BackupState {
		flags |= protocol.FlagBackupState
	}
	credential := gowebauthn.Credential{
		ID:        append([]byte(nil), expected.CredentialID...),
		PublicKey: append([]byte(nil), expected.PublicKeyCOSE...),
		Flags:     gowebauthn.NewCredentialFlags(flags),
		Authenticator: gowebauthn.Authenticator{
			AAGUID:    append([]byte(nil), expected.AAGUID[:]...),
			SignCount: expected.SignCount,
		},
		AttestationFormat: "none", // No claimed verified manufacturer provenance.
		AttestationType:   "none",
	}
	user := activationAssertionUser{
		subject:    expected.Subject,
		credential: credential,
	}
	session := gowebauthn.SessionData{
		Challenge:      base64.RawURLEncoding.EncodeToString(expected.Nonce[:]),
		RelyingPartyID: expected.RPID,
		Origin:         expected.Origin,
		UserID:         user.WebAuthnID(),
		AllowedCredentialIDs: [][]byte{
			append([]byte(nil), expected.CredentialID...),
		},
		UserVerification: protocol.VerificationRequired,
		Mediation:        protocol.MediationDefault,
	}
	verified, err := v.engine.ValidateLogin(user, session, parsed)
	if err != nil || verified == nil || ctx.Err() != nil ||
		!bytes.Equal(verified.ID, expected.CredentialID) ||
		verified.Authenticator.CloneWarning {
		// Never return vendor detail or a clone-warning result to callers.
		return denied, usecase.ErrPasskeyAssertionDenied
	}
	rawFlags := parsed.Response.AuthenticatorData.Flags
	counter := parsed.Response.AuthenticatorData.Counter
	if !rawFlags.HasUserPresent() || !rawFlags.HasUserVerified() ||
		rawFlags.HasBackupEligible() != expected.BackupEligible ||
		(rawFlags.HasBackupState() && !rawFlags.HasBackupEligible()) ||
		((expected.SignCount != 0 || counter != 0) &&
			counter <= expected.SignCount) {
		return denied, usecase.ErrPasskeyAssertionDenied
	}
	return usecase.PasskeyActivationAssertionResult{
		CredentialID:   append([]byte(nil), verified.ID...),
		Counter:        counter,
		UserPresent:    true,
		UserVerified:   true,
		BackupEligible: rawFlags.HasBackupEligible(),
		BackupState:    rawFlags.HasBackupState(),
	}, nil
}

type activationAssertionUser struct {
	subject    string
	credential gowebauthn.Credential
}

func (u activationAssertionUser) WebAuthnID() []byte          { return []byte(u.subject) }
func (u activationAssertionUser) WebAuthnName() string        { return "Mamony identity" }
func (u activationAssertionUser) WebAuthnDisplayName() string { return "Mamony identity" }
func (u activationAssertionUser) WebAuthnCredentials() []gowebauthn.Credential {
	return []gowebauthn.Credential{u.credential}
}
