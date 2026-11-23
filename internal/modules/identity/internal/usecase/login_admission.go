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
	ErrLoginAdmissionUnavailable = errors.New("login admission unavailable")
	ErrLoginAdmissionInvalid     = errors.New("login admission invalid")
)

// LoginAdmission charges a database-global budget and independent client and
// alias budgets BEFORE credential lookup or memory-hard password verification.
// It never consults the account table or determines whether an email exists.
// A production caller must also enforce edge limits to protect the DB itself.
type LoginAdmission struct {
	store AnonymousAdmissionPort
	key   [32]byte
}

func NewLoginAdmission(store AnonymousAdmissionPort, secret []byte) (*LoginAdmission, error) {
	if store == nil || len(secret) != 32 {
		return nil, ErrLoginAdmissionUnavailable
	}
	gate := &LoginAdmission{store: store}
	copy(gate.key[:], secret)
	return gate, nil
}

func (a *LoginAdmission) digest(scope, value string) [32]byte {
	mac := hmac.New(sha256.New, a.key[:])
	_, _ = mac.Write([]byte("mamony:identity:login-admission:v1\x00"))
	_, _ = mac.Write([]byte(scope))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(value))
	var key [32]byte
	copy(key[:], mac.Sum(nil))
	return key
}

// Admit is independent of the current existence or status of the alias.
// Keys are HMAC-SHA256 of a global constant, IPv4 address / IPv6 /64 prefix,
// or canonical email. Plaintext emails and IPs never enter Spend() or SQL.
//
// Candidate limits are intentionally conservative and REQUIRE load/NAT review
// before public use. The global 80/min bucket is a shared DB hot row: a
// perimeter distributed limiter and workload capacity plan remain mandatory.
func (a *LoginAdmission) Admit(ctx context.Context, email string, peer netip.Addr) (bool, error) {
	if a == nil || a.store == nil || ctx == nil || ctx.Err() != nil {
		return false, ErrLoginAdmissionUnavailable
	}
	if !peer.IsValid() || peer.IsUnspecified() || peer.IsMulticast() ||
		peer.Zone() != "" {
		return false, ErrLoginAdmissionInvalid
	}
	canonical, err := domain.CanonicalEmail(email)
	if err != nil {
		return false, ErrLoginAdmissionInvalid
	}
	peer = peer.Unmap()
	client := peer.String()
	if peer.Is6() {
		client = netip.PrefixFrom(peer, 64).Masked().Addr().String()
	}
	buckets := []AdmissionBucket{
		{Scope: "GLOBAL_MINUTE", Key: a.digest("GLOBAL", "all"), WindowSeconds: 60, Limit: 80},
		{Scope: "CLIENT_MINUTE", Key: a.digest("CLIENT", client), WindowSeconds: 60, Limit: 8},
		{Scope: "CLIENT_HOUR", Key: a.digest("CLIENT", client), WindowSeconds: 3600, Limit: 80},
		{Scope: "EMAIL_HOUR", Key: a.digest("EMAIL", canonical), WindowSeconds: 3600, Limit: 5},
		{Scope: "EMAIL_DAY", Key: a.digest("EMAIL", canonical), WindowSeconds: 86400, Limit: 20},
	}
	// The PostgreSQL adapter validates the entire five-bucket action-specific
	// policy and charges it under one transaction and one DB clock sample.
	ok, err := a.store.Spend(ctx, AnonymousLogin, buckets)
	if err != nil {
		return false, ErrLoginAdmissionUnavailable
	}
	return ok, nil
}
