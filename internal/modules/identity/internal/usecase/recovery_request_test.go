package usecase

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"testing"
)

type recoveryBudgetFake struct {
	allow  bool
	fail   bool
	calls  int
	action AnonymousAction
	bins   []AdmissionBucket
}

func (f *recoveryBudgetFake) Spend(_ context.Context, action AnonymousAction, bins []AdmissionBucket) (bool, error) {
	f.calls++
	f.action = action
	f.bins = append([]AdmissionBucket(nil), bins...)
	if f.fail {
		return false, errors.New("synthetic storage failure with private details")
	}
	return f.allow, nil
}

type recoveryIssuerFake struct {
	calls int
	last  RecoveryIssue
	allow bool
	fail  bool
}

func (f *recoveryIssuerFake) Issue(_ context.Context, req RecoveryIssue) (bool, error) {
	f.calls++
	f.last = req
	if f.fail {
		return false, errors.New("synthetic SQL commit unknown")
	}
	return f.allow, nil
}

type recoverySealFake struct {
	calls int
	email string
	token string
	fail  bool
}

func (f *recoverySealFake) SealRecovery(email, challenge, token string) (string, []byte, error) {
	f.calls++
	f.email, f.token = email, token
	if len(challenge) != 26 || challenge[:4] != "rcv_" || len(token) != 43 {
		return "", nil, errors.New("synthetic malformed input")
	}
	if f.fail {
		return "", nil, errors.New("synthetic unavailable encryption")
	}
	return "recovery-test-key", bytes.Repeat([]byte{0xad}, 76), nil
}

func TestRecoveryAdmissionPurposeAndLimits(t *testing.T) {
	store := &recoveryBudgetFake{allow: true}
	gate, err := NewRecoveryAdmission(store, bytes.Repeat([]byte{0xc7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	ok, err := gate.Admit(context.Background(), "candidate@example.test",
		netip.MustParseAddr("2001:db8::12"))
	if err != nil || !ok || store.calls != 1 || store.action != AnonymousRecovery {
		t.Fatal("recovery gate must charge the dedicated action once")
	}
	expectedScopes := []string{"GLOBAL_MINUTE", "CLIENT_MINUTE", "CLIENT_HOUR", "EMAIL_HOUR", "EMAIL_DAY"}
	expectedLimits := []int32{60, 5, 40, 2, 4}
	if len(store.bins) != 5 {
		t.Fatal("recovery gate omitted mandatory global/client/alias quotas")
	}
	for i, b := range store.bins {
		if b.Scope != expectedScopes[i] || b.Limit != expectedLimits[i] ||
			b.Key == ([32]byte{}) {
			t.Fatal("recovery gate passed an unexpected action-specific budget")
		}
	}
	a := store.bins[1].Key
	_, _ = gate.Admit(context.Background(), "candidate@example.test",
		netip.MustParseAddr("2001:db8::100"))
	if a != store.bins[1].Key {
		t.Fatal("IPv6 rotation inside /64 bypassed recovery client budget")
	}
	if _, err := gate.Admit(context.Background(), "INVALID", netip.MustParseAddr("192.0.2.9")); !errors.Is(err, ErrRecoveryAdmissionInvalid) {
		t.Fatal("invalid alias accepted")
	}
	if _, err := NewRecoveryAdmission(nil, make([]byte, 32)); err == nil {
		t.Fatal("missing database admission accepted")
	}
}

func TestRecoveryRequestAdmitsAndSealsForUnknownAccounts(t *testing.T) {
	store := &recoveryBudgetFake{allow: true}
	gate, err := NewRecoveryAdmission(store, bytes.Repeat([]byte{0xa1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	issuer := &recoveryIssuerFake{allow: false} // unknown alias
	sealer := &recoverySealFake{}
	flow, err := NewRecoveryRequest(gate, issuer, sealer)
	if err != nil {
		t.Fatal(err)
	}
	peer := netip.MustParseAddr("192.0.2.100")
	if err := flow.Request(context.Background(), "unknown@example.test", peer); err != nil {
		t.Fatal("unknown account must not produce externally distinguishable result")
	}
	if store.calls != 1 || sealer.calls != 1 || issuer.calls != 1 ||
		issuer.last.CanonicalEmail != "unknown@example.test" ||
		issuer.last.TokenDigest == ([32]byte{}) ||
		len(issuer.last.SealedToken) != 76 {
		t.Fatal("admitted unknown account skipped invariant cryptographic pipeline")
	}
	digest, err := PasswordRecoveryProofDigest(sealer.token)
	if err != nil || issuer.last.TokenDigest != digest {
		t.Fatal("sealed raw proof and stored digest did not match")
	}
	issuer.allow = true
	if err := flow.Request(context.Background(), "unknown@example.test", peer); err != nil {
		t.Fatal("eligible account should use the same API result")
	}
	if issuer.calls != 2 {
		t.Fatal("eligible account issuance was not attempted")
	}
	store.allow = false
	if err := flow.Request(context.Background(), "unknown@example.test", peer); err != nil ||
		issuer.calls != 2 || sealer.calls != 2 {
		t.Fatal("denied admission must block all issuance and still look generic")
	}
	store.fail = true
	if err := flow.Request(context.Background(), "unknown@example.test", peer); !errors.Is(err, ErrRecoveryRequestUnavailable) {
		t.Fatal("admission infrastructure failure must fail closed")
	}
	store.fail = false
	store.allow = true
	sealer.fail = true
	if err := flow.Request(context.Background(), "unknown@example.test", peer); !errors.Is(err, ErrRecoveryRequestUnavailable) {
		t.Fatal("encryption failure incorrectly issued a proof")
	}
	sealer.fail = false
	issuer.fail = true
	if err := flow.Request(context.Background(), "unknown@example.test", peer); !errors.Is(err, ErrRecoveryRequestUnavailable) {
		t.Fatal("SQL ambiguous commit incorrectly reported a known result")
	}
	if err := flow.Request(context.Background(), "INVALID", peer); !errors.Is(err, ErrRecoveryAdmissionInvalid) {
		t.Fatal("invalid canonical input admitted")
	}
	if _, err := NewRecoveryRequest(gate, nil, sealer); err == nil {
		t.Fatal("missing issuer authority accepted")
	}
}
