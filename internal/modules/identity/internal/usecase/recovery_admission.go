package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"net/netip"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

var (
	ErrRecoveryAdmissionUnavailable = errors.New("recovery admission unavailable")
	ErrRecoveryAdmissionInvalid     = errors.New("recovery admission invalid")
)

const AnonymousRecovery AnonymousAction = "RECOVERY"

// RecoveryAdmission is a separate, purpose-keyed durable pre-lookup budget.
// This gate must run even for non-existent or ineligible emails, before any
// contact lookup, proof generation, envelope sealing or database issuance.
type RecoveryAdmission struct {
	store AnonymousAdmissionPort
	key   [32]byte
}

func NewRecoveryAdmission(store AnonymousAdmissionPort, key []byte) (*RecoveryAdmission, error) {
	if store == nil || len(key) != 32 {
		return nil, ErrRecoveryAdmissionUnavailable
	}
	var k [32]byte
	copy(k[:], key)
	return &RecoveryAdmission{store: store, key: k}, nil
}

func (a *RecoveryAdmission) digest(scope, value string) [32]byte {
	mac := hmac.New(sha256.New, a.key[:])
	_, _ = mac.Write([]byte("mamony:identity:recovery-admission:v1\x00"))
	_, _ = mac.Write([]byte(scope))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(value))
	var result [32]byte
	copy(result[:], mac.Sum(nil))
	return result
}

// Admit charges five separate global/client/email quotas in one transaction.
// The PostgreSQL store has its own strict, action-specific bucket allowlist.
func (a *RecoveryAdmission) Admit(ctx context.Context, rawEmail string, peer netip.Addr) (bool, error) {
	if a == nil || a.store == nil || ctx == nil || ctx.Err() != nil {
		return false, ErrRecoveryAdmissionUnavailable
	}
	email, err := domain.CanonicalEmail(rawEmail)
	if err != nil || email != rawEmail || !peer.IsValid() || peer.IsUnspecified() ||
		peer.IsMulticast() || peer.Zone() != "" {
		return false, ErrRecoveryAdmissionInvalid
	}
	peer = peer.Unmap()
	client := peer.String()
	if peer.Is6() {
		client = netip.PrefixFrom(peer, 64).Masked().Addr().String()
	}
	buckets := []AdmissionBucket{
		{Scope: "GLOBAL_MINUTE", Key: a.digest("GLOBAL", "all"), WindowSeconds: 60, Limit: 60},
		{Scope: "CLIENT_MINUTE", Key: a.digest("CLIENT", client), WindowSeconds: 60, Limit: 5},
		{Scope: "CLIENT_HOUR", Key: a.digest("CLIENT", client), WindowSeconds: 3600, Limit: 40},
		{Scope: "EMAIL_HOUR", Key: a.digest("EMAIL", email), WindowSeconds: 3600, Limit: 2},
		{Scope: "EMAIL_DAY", Key: a.digest("EMAIL", email), WindowSeconds: 86400, Limit: 4},
	}
	ok, err := a.store.Spend(ctx, AnonymousRecovery, buckets)
	if err != nil {
		return false, ErrRecoveryAdmissionUnavailable
	}
	return ok, nil
}
