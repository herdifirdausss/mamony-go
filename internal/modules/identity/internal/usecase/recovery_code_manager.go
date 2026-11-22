package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

var (
	ErrRecoveryCodeManagerDenied      = errors.New("recovery code change denied")
	ErrRecoveryCodeManagerUnavailable = errors.New("recovery code change unavailable")
)

const recoveryCodeEnrollmentApprovalDomain = "mamony:identity:recovery-code-enrollment-approval:v1\x00"

// RecoveryEnrollmentApprovalDigest recognizes a separately issued, random
// 256-bit independent proof. No producer/issuer of this proof is implemented;
// default-deny SQL guarantees that a session+password cannot bootstrap codes.
func RecoveryEnrollmentApprovalDigest(raw string) ([32]byte, error) {
	var zero [32]byte
	if len(raw) != 43 {
		return zero, ErrRecoveryCodeManagerDenied
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 32 ||
		base64.RawURLEncoding.EncodeToString(decoded) != raw {
		clear(decoded)
		return zero, ErrRecoveryCodeManagerDenied
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(recoveryCodeEnrollmentApprovalDomain))
	_, _ = hasher.Write(decoded)
	copy(zero[:], hasher.Sum(nil))
	clear(decoded)
	return zero, nil
}

// RecoveryCodeSet is returned ONLY on a certain committed enrollment.
// No public transport or JSON serializer may render raw codes. The eventual
// first-party code-display UI must use a separately reviewed trusted channel.
type RecoveryCodeSet struct {
	codes [5]OfflineRecoveryCode
}

func (s RecoveryCodeSet) ForTrustedDisplay() []string {
	result := make([]string, len(s.codes))
	for i, code := range s.codes {
		result[i] = code.TrustedOfflineDisplay()
	}
	return result
}

const recoveryCodeSetRedaction = "[REDACTED offline recovery code set]"

func (RecoveryCodeSet) String() string { return recoveryCodeSetRedaction }
func (RecoveryCodeSet) GoString() string { return recoveryCodeSetRedaction }
func (RecoveryCodeSet) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, recoveryCodeSetRedaction)
}
func (RecoveryCodeSet) LogValue() slog.Value {
	return slog.StringValue(recoveryCodeSetRedaction)
}
func (RecoveryCodeSet) MarshalJSON() ([]byte, error) {
	return nil, ErrRecoveryCodeManagerDenied
}
func (RecoveryCodeSet) MarshalText() ([]byte, error) {
	return nil, ErrRecoveryCodeManagerDenied
}

// RecoveryCodeManagerStore is restricted to one EXECUTE-only transaction.
type RecoveryCodeManagerStore interface {
	Replace(context.Context, [32]byte, [32]byte, [32]byte, [5][32]byte, string) (bool, error)
}

// AuthenticatedRecoveryCodeManager reuses the same durable pre-Argon2id login
// admission and password verifier as authenticated password change, but
// issues a DISTINCT RECOVERY_CODE_ROTATION 30-second grant. The independent
// bootstrap proof is neither a cookie nor another password. Existing-code
// rotation requires one older unconsumed saved offline recovery code.
type AuthenticatedRecoveryCodeManager struct {
	sessions *OpaqueSessionAuthority
	current  *PasswordLoginFlow
	grants   *RecoveryCodeGrantAuthority
	store    RecoveryCodeManagerStore
	random   io.Reader
}

func NewAuthenticatedRecoveryCodeManager(sessions *OpaqueSessionAuthority,
	current *PasswordLoginFlow, grants *RecoveryCodeGrantAuthority,
	store RecoveryCodeManagerStore) (*AuthenticatedRecoveryCodeManager, error) {
	if sessions == nil || current == nil || grants == nil || store == nil {
		return nil, ErrRecoveryCodeManagerUnavailable
	}
	return &AuthenticatedRecoveryCodeManager{
		sessions: sessions, current: current, grants: grants,
		store: store, random: rand.Reader,
	}, nil
}

// Enroll requires independent approval issued BEFORE code enrollment.
// No production issuer exists: synthetic privileged test fixtures only.
func (m *AuthenticatedRecoveryCodeManager) Enroll(ctx context.Context,
	session, email, currentPassword, independentApproval string,
	peer netip.Addr) (RecoveryCodeSet, error) {
	digest, err := RecoveryEnrollmentApprovalDigest(independentApproval)
	if err != nil {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerDenied
	}
	return m.replace(ctx, session, email, currentPassword, digest, peer)
}

// Rotate requires an already-enrolled offline code in addition to an active
// session and a NEW current-password proof. It is never a self-service reset.
func (m *AuthenticatedRecoveryCodeManager) Rotate(ctx context.Context,
	session, email, currentPassword, previousOfflineCode string,
	peer netip.Addr) (RecoveryCodeSet, error) {
	digest, err := OfflineRecoveryCodeDigest(previousOfflineCode)
	if err != nil {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerDenied
	}
	return m.replace(ctx, session, email, currentPassword, digest, peer)
}

func (m *AuthenticatedRecoveryCodeManager) replace(ctx context.Context,
	session, email, currentPassword string, independent [32]byte,
	peer netip.Addr) (RecoveryCodeSet, error) {
	if m == nil || m.sessions == nil || m.current == nil || m.grants == nil ||
		m.store == nil || m.random == nil || ctx == nil || ctx.Err() != nil {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerUnavailable
	}
	canonical, err := domain.CanonicalEmail(email)
	if err != nil || canonical != email {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerDenied
	}
	subject, err := m.sessions.Resolve(ctx, session)
	if errors.Is(err, ErrSessionInvalid) {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerDenied
	}
	if err != nil {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerUnavailable
	}
	candidate, err := m.current.AuthenticateWithEpoch(ctx, canonical, currentPassword, peer)
	if errors.Is(err, ErrPasswordAuthenticationDenied) ||
		errors.Is(err, ErrPasswordLoginThrottled) {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerDenied
	}
	if err != nil {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerUnavailable
	}
	if candidate.SubjectID != subject || candidate.SessionEpoch <= 0 {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerDenied
	}
	sessionDigest, err := sessionTokenDigest(session)
	if err != nil {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerDenied
	}

	// Generate all five distinct code candidates before staging an
	// authentication grant. Never expose a secret before SQL COMMIT.
	var next RecoveryCodeSet
	var digests [5][32]byte
	for i := range next.codes {
		code, err := newOfflineRecoveryCode(m.random)
		if err != nil {
			return RecoveryCodeSet{}, ErrRecoveryCodeManagerUnavailable
		}
		next.codes[i] = code
		digests[i] = code.Digest()
		for j := 0; j < i; j++ {
			if digests[i] == digests[j] {
				return RecoveryCodeSet{}, ErrRecoveryCodeManagerUnavailable
			}
		}
	}
	proof, err := m.grants.Issue(ctx, candidate.SubjectID, candidate.SessionEpoch)
	if errors.Is(err, ErrSessionDenied) {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerDenied
	}
	if err != nil {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerUnavailable
	}
	grantDigest, err := authenticationGrantDigest(proof.secret)
	if err != nil {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerUnavailable
	}
	event, err := newOpaqueID(m.random, "evt_")
	if err != nil {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerUnavailable
	}
	allowed, err := m.store.Replace(ctx, sessionDigest, grantDigest, independent, digests, event)
	if err != nil || ctx.Err() != nil {
		// An ambiguous COMMIT may have replaced every code. No retry and
		// absolutely no raw codes may leave this failed operation.
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerUnavailable
	}
	if !allowed {
		return RecoveryCodeSet{}, ErrRecoveryCodeManagerDenied
	}
	return next, nil
}
