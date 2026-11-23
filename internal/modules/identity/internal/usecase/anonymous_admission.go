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
	ErrAnonymousAdmissionUnavailable = errors.New("identity admission unavailable")
	ErrAnonymousAdmissionInvalid     = errors.New("identity admission invalid")
)

// AnonymousAction is an untrusted initiation category, never an authorized
// Identity subject or proof of ownership of an email address.
type AnonymousAction string

const (
	AnonymousRegister AnonymousAction = "REGISTER"
	AnonymousResend   AnonymousAction = "RESEND"
	AnonymousConfirm  AnonymousAction = "CONFIRM"
	AnonymousLogin    AnonymousAction = "LOGIN"
)

// AdmissionBucket is an allowlisted database accounting obligation. Keys are
// HMAC-SHA256 digests, not plaintext contact or IP addresses.
type AdmissionBucket struct {
	Scope         string
	Key           [32]byte
	WindowSeconds int64
	Limit         int32
}

type AnonymousAdmissionPort interface {
	Spend(context.Context, AnonymousAction, []AdmissionBucket) (bool, error)
}

// AnonymousAdmission limits unauthenticated initiation, BEFORE any account
// lookup/password hashing, but it does not authorize registering or resending.
type AnonymousAdmission struct {
	store AnonymousAdmissionPort
	key   [32]byte
}

func NewAnonymousAdmission(store AnonymousAdmissionPort, key []byte) (*AnonymousAdmission, error) {
	if store == nil || len(key) != 32 {
		return nil, ErrAnonymousAdmissionUnavailable
	}
	a := &AnonymousAdmission{store: store}
	copy(a.key[:], key)
	return a, nil
}

func (a *AnonymousAdmission) digest(action AnonymousAction, scope, value string) [32]byte {
	mac := hmac.New(sha256.New, a.key[:])
	_, _ = mac.Write([]byte("mamony:identity:anonymous-admission:v1\x00"))
	_, _ = mac.Write([]byte(action))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(scope))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(value))
	var result [32]byte
	copy(result[:], mac.Sum(nil))
	return result
}

// Admit charges shared database budgets even for emails that do not exist.
// It must be invoked before any account-existence lookup or expensive KDF.
// All callers with valid syntax get the same response shape regardless of
// this boolean; the bool is trusted internal data only.
func (a *AnonymousAdmission) Admit(ctx context.Context, action AnonymousAction, rawEmail string, client netip.Addr) (bool, error) {
	if a == nil || a.store == nil {
		return false, ErrAnonymousAdmissionUnavailable
	}
	if action != AnonymousRegister && action != AnonymousResend {
		return false, ErrAnonymousAdmissionInvalid
	}
	email, err := domain.CanonicalEmail(rawEmail)
	if err != nil || !client.IsValid() || client.Zone() != "" ||
		client.IsUnspecified() || client.IsMulticast() {
		return false, ErrAnonymousAdmissionInvalid
	}
	client = client.Unmap()
	// Share one prefix budget among an IPv6 /64 (rotation resistant) and
	// use exact IPv4 addresses. NAT and shared prefixes can limit bystanders;
	// the policy requires product review before public rollout.
	address := client.String()
	if client.Is6() {
		address = netip.PrefixFrom(client, 64).Masked().Addr().String()
	}
	// Deterministic global order prevents cyclic lock waits between callers
	// that share an email but have different remote addresses.
	buckets := []AdmissionBucket{
		{Scope: "CLIENT_MINUTE", Key: a.digest(action, "CLIENT", address), WindowSeconds: 60, Limit: 8},
		{Scope: "CLIENT_HOUR", Key: a.digest(action, "CLIENT", address), WindowSeconds: 3600, Limit: 80},
		{Scope: "EMAIL_HOUR", Key: a.digest(action, "EMAIL", email), WindowSeconds: 3600, Limit: 3},
		{Scope: "EMAIL_DAY", Key: a.digest(action, "EMAIL", email), WindowSeconds: 86400, Limit: 5},
	}
	ok, err := a.store.Spend(ctx, action, buckets)
	if err != nil {
		return false, ErrAnonymousAdmissionUnavailable
	}
	return ok, nil
}
