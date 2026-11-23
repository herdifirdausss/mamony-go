package usecase

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

var (
	ErrRecoveryFactorDenied      = errors.New("recovery factor proof denied")
	ErrRecoveryFactorUnavailable = errors.New("recovery factor verifier unavailable")
)

const recoveryFactorChallengeDomain = "mamony:identity:recovery-code-factor-challenge:ed25519:v1\x00"

// RecoveryFactorChallenge is an authenticated, short-lived purpose-specific
// challenge. It contains PUBLIC nonce/signing material, never an authenticator
// private key. The key was provisioned before the challenge through a distinct
// trusted process (no user self-enrollment in this PR).
type RecoveryFactorChallenge struct {
	id      string
	subject string
	epoch   int64
	nonce   [32]byte
}

func (c RecoveryFactorChallenge) ChallengeID() string { return c.id }

func (c RecoveryFactorChallenge) ForTrustedAuthenticator() []byte {
	return recoveryFactorSignedPayload(c.id, c.subject, c.epoch, c.nonce)
}

// Fixed length account identifiers avoid delimiter ambiguity; domain
// separation forbids this signature from authorizing another action.
func recoveryFactorSignedPayload(id, subject string, epoch int64, nonce [32]byte) []byte {
	if len(id) != 26 || len(subject) != 26 || epoch <= 0 {
		return nil
	}
	data := make([]byte, 0, len(recoveryFactorChallengeDomain)+26+26+8+32)
	data = append(data, recoveryFactorChallengeDomain...)
	data = append(data, id...)
	data = append(data, subject...)
	var epochBytes [8]byte
	binary.BigEndian.PutUint64(epochBytes[:], uint64(epoch))
	data = append(data, epochBytes[:]...)
	return append(data, nonce[:]...)
}

// IndependentEnrollmentApproval is only returned after an Ed25519 signature
// succeeds and the database commits a five-minute, one-use approval.
// Normal log, fmt or JSON paths must never expose the bearer.
type IndependentEnrollmentApproval struct{ secret string }

func (p IndependentEnrollmentApproval) ForTrustedEnrollment() string { return p.secret }
func (IndependentEnrollmentApproval) String() string                 { return "[REDACTED factor approval]" }
func (IndependentEnrollmentApproval) GoString() string               { return "[REDACTED factor approval]" }
func (IndependentEnrollmentApproval) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "[REDACTED factor approval]")
}
func (IndependentEnrollmentApproval) LogValue() slog.Value {
	return slog.StringValue("[REDACTED factor approval]")
}
func (IndependentEnrollmentApproval) MarshalJSON() ([]byte, error) {
	return nil, ErrRecoveryFactorDenied
}
func (IndependentEnrollmentApproval) MarshalText() ([]byte, error) {
	return nil, ErrRecoveryFactorDenied
}

type RecoveryFactorEvidence struct {
	Subject   string
	Epoch     int64
	Nonce     [32]byte
	PublicKey [32]byte
}

// Different SQL principals must back Issue vs Read/Fail/Complete. These
// interfaces never grant public-key enrollment or read account secrets.
type RecoveryFactorIssueStore interface {
	Issue(context.Context, [32]byte, [32]byte, string, [32]byte) (bool, error)
}
type RecoveryFactorVerifierStore interface {
	Read(context.Context, string) (RecoveryFactorEvidence, bool, error)
	Fail(context.Context, string) error
	Complete(context.Context, string, [32]byte, [32]byte, string) (bool, error)
}

type RecoveryFactorChallengeIssuer struct {
	sessions *OpaqueSessionAuthority
	current  *PasswordLoginFlow
	grants   *RecoveryCodeGrantAuthority
	store    RecoveryFactorIssueStore
	random   io.Reader
}

func NewRecoveryFactorChallengeIssuer(sessions *OpaqueSessionAuthority,
	current *PasswordLoginFlow, grants *RecoveryCodeGrantAuthority,
	store RecoveryFactorIssueStore) (*RecoveryFactorChallengeIssuer, error) {
	if sessions == nil || current == nil || grants == nil || store == nil {
		return nil, ErrRecoveryFactorUnavailable
	}
	return &RecoveryFactorChallengeIssuer{
		sessions: sessions, current: current, grants: grants, store: store, random: rand.Reader,
	}, nil
}

// Issue demands a live session and a FRESH Argon2id current-password check,
// with the repo's existing global/client/alias admission BEFORE the KDF.
// The database consumes that 30-second reauth grant when issuing the nonce.
func (i *RecoveryFactorChallengeIssuer) Issue(ctx context.Context,
	session, email, password string, peer netip.Addr) (RecoveryFactorChallenge, error) {
	if i == nil || i.sessions == nil || i.current == nil || i.grants == nil ||
		i.store == nil || i.random == nil || ctx == nil || ctx.Err() != nil {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorUnavailable
	}
	canonical, err := domain.CanonicalEmail(email)
	if err != nil || canonical != email {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorDenied
	}
	subject, err := i.sessions.Resolve(ctx, session)
	if errors.Is(err, ErrSessionInvalid) {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorDenied
	}
	if err != nil {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorUnavailable
	}
	account, err := i.current.AuthenticateWithEpoch(ctx, canonical, password, peer)
	if errors.Is(err, ErrPasswordAuthenticationDenied) || errors.Is(err, ErrPasswordLoginThrottled) {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorDenied
	}
	if err != nil {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorUnavailable
	}
	if subject != account.SubjectID || account.SessionEpoch <= 0 {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorDenied
	}
	sessionDigest, err := sessionTokenDigest(session)
	if err != nil {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorDenied
	}
	var nonce [32]byte
	if _, err := io.ReadFull(i.random, nonce[:]); err != nil {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorUnavailable
	}
	id, err := newOpaqueID(i.random, "fac_")
	if err != nil {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorUnavailable
	}
	grant, err := i.grants.Issue(ctx, subject, account.SessionEpoch)
	if errors.Is(err, ErrSessionDenied) {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorDenied
	}
	if err != nil {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorUnavailable
	}
	grantDigest, err := authenticationGrantDigest(grant.secret)
	if err != nil {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorUnavailable
	}
	allowed, err := i.store.Issue(ctx, sessionDigest, grantDigest, id, nonce)
	if err != nil || ctx.Err() != nil {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorUnavailable
	}
	if !allowed {
		return RecoveryFactorChallenge{}, ErrRecoveryFactorDenied
	}
	return RecoveryFactorChallenge{id: id, subject: subject, epoch: account.SessionEpoch, nonce: nonce}, nil
}

type RecoveryFactorApprovalVerifier struct {
	store  RecoveryFactorVerifierStore
	random io.Reader
}

func NewRecoveryFactorApprovalVerifier(store RecoveryFactorVerifierStore) (*RecoveryFactorApprovalVerifier, error) {
	if store == nil {
		return nil, ErrRecoveryFactorUnavailable
	}
	return &RecoveryFactorApprovalVerifier{store: store, random: rand.Reader}, nil
}

// Verify uses the pre-provisioned public key, and never accepts an emailed
// code, current password, session token or self-signed software challenge as
// an independent factor. Five failed signatures burn this challenge.
func (v *RecoveryFactorApprovalVerifier) Verify(ctx context.Context,
	challengeID string, signature []byte) (IndependentEnrollmentApproval, error) {
	if v == nil || v.store == nil || v.random == nil || ctx == nil || ctx.Err() != nil {
		return IndependentEnrollmentApproval{}, ErrRecoveryFactorUnavailable
	}
	if len(challengeID) != 26 || challengeID[:4] != "fac_" {
		return IndependentEnrollmentApproval{}, ErrRecoveryFactorDenied
	}
	evidence, found, err := v.store.Read(ctx, challengeID)
	if err != nil {
		return IndependentEnrollmentApproval{}, ErrRecoveryFactorUnavailable
	}
	if !found {
		return IndependentEnrollmentApproval{}, ErrRecoveryFactorDenied
	}
	msg := recoveryFactorSignedPayload(challengeID, evidence.Subject, evidence.Epoch, evidence.Nonce)
	if msg == nil || len(signature) != ed25519.SignatureSize ||
		!ed25519.Verify(ed25519.PublicKey(evidence.PublicKey[:]), msg, signature) {
		if err := v.store.Fail(ctx, challengeID); err != nil {
			return IndependentEnrollmentApproval{}, ErrRecoveryFactorUnavailable
		}
		return IndependentEnrollmentApproval{}, ErrRecoveryFactorDenied
	}
	var raw [32]byte
	if _, err := io.ReadFull(v.random, raw[:]); err != nil {
		return IndependentEnrollmentApproval{}, ErrRecoveryFactorUnavailable
	}
	proof := base64.RawURLEncoding.EncodeToString(raw[:])
	clear(raw[:])
	digest, err := RecoveryEnrollmentApprovalDigest(proof)
	if err != nil {
		return IndependentEnrollmentApproval{}, ErrRecoveryFactorUnavailable
	}
	event, err := newOpaqueID(v.random, "evt_")
	if err != nil {
		return IndependentEnrollmentApproval{}, ErrRecoveryFactorUnavailable
	}
	allowed, err := v.store.Complete(ctx, challengeID, evidence.PublicKey, digest, event)
	if err != nil || ctx.Err() != nil {
		return IndependentEnrollmentApproval{}, ErrRecoveryFactorUnavailable
	}
	if !allowed {
		return IndependentEnrollmentApproval{}, ErrRecoveryFactorDenied
	}
	return IndependentEnrollmentApproval{secret: proof}, nil
}
