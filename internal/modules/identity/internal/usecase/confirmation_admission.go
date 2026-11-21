package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"net/netip"
)

// ConfirmationAdmission enforces independent account-agnostic, HMAC-keyed
// shared PostgreSQL budgets BEFORE a verifier-role token lookup. This is
// unauthenticated abuse admission, never an authentication decision.
// Per-client fixed windows are a local bound; perimeter/global controls
// and legitimate NAT retry UX remain separate rollout prerequisites.
type ConfirmationAdmission struct {
	store AnonymousAdmissionPort
	key   [32]byte
}

func NewConfirmationAdmission(store AnonymousAdmissionPort, key []byte) (*ConfirmationAdmission, error) {
	if store == nil || len(key) != 32 {
		return nil, ErrAnonymousAdmissionUnavailable
	}
	a := &ConfirmationAdmission{store: store}
	copy(a.key[:], key)
	return a, nil
}

// Admit never accepts or persists a bearer token, subject ID or email.
// IPv6 /64 network accounting resists simple address rotation but may
// throttle legitimate users sharing a network. Invalid peers fail closed.
func (a *ConfirmationAdmission) Admit(ctx context.Context, client netip.Addr) (bool, error) {
	if a == nil || a.store == nil || ctx == nil || ctx.Err() != nil ||
		!client.IsValid() || client.Zone() != "" || client.IsUnspecified() || client.IsMulticast() {
		return false, ErrAnonymousAdmissionUnavailable
	}
	client = client.Unmap()
	address := client.String()
	if client.Is6() {
		address = netip.PrefixFrom(client, 64).Masked().Addr().String()
	}
	mac := hmac.New(sha256.New, a.key[:])
	_, _ = mac.Write([]byte("mamony:identity:confirmation-client:v1\x00"))
	_, _ = mac.Write([]byte(address))
	var digest [32]byte
	copy(digest[:], mac.Sum(nil))
	if digest == ([32]byte{}) {
		return false, ErrAnonymousAdmissionUnavailable
	}
	// These are deliberately separate from REGISTER/RESEND. The repository
	// accepts only the exact CONFIRM, CLIENT_MINUTE+CLIENT_HOUR pair.
	budgets := []AdmissionBucket{
		{Scope: "CLIENT_MINUTE", Key: digest, WindowSeconds: 60, Limit: 8},
		{Scope: "CLIENT_HOUR", Key: digest, WindowSeconds: 3600, Limit: 80},
	}
	ok, err := a.store.Spend(ctx, AnonymousConfirm, budgets)
	if err != nil {
		return false, ErrAnonymousAdmissionUnavailable
	}
	return ok, nil
}

// The compiler asserts that the public handler can only depend on this
// narrowly scoped network admission method, not password/contact lookup.
var _ interface {
	Admit(context.Context, netip.Addr) (bool, error)
} = (*ConfirmationAdmission)(nil)

