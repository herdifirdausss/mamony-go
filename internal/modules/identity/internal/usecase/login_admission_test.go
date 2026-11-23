package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"testing"
)

func TestLoginAdmissionHMACPrivacyAndIndependentBudgets(t *testing.T) {
	store := &fakeAnonymousAdmissionStore{allowed: true}
	secret := bytes.Repeat([]byte{0x86}, 32)
	login, err := NewLoginAdmission(store, secret)
	if err != nil {
		t.Fatal(err)
	}
	admit := func(email, address string) []AdmissionBucket {
		t.Helper()
		ok, err := login.Admit(context.Background(), email, netip.MustParseAddr(address))
		if err != nil || !ok || store.action != AnonymousLogin || len(store.buckets) != 5 {
			t.Fatalf("valid login admission was rejected: %t %v", ok, err)
		}
		wants := []struct {
			scope  string
			window int64
			limit  int32
		}{
			{"GLOBAL_MINUTE", 60, 80},
			{"CLIENT_MINUTE", 60, 8},
			{"CLIENT_HOUR", 3600, 80},
			{"EMAIL_HOUR", 3600, 5},
			{"EMAIL_DAY", 86400, 20},
		}
		for i, want := range wants {
			b := store.buckets[i]
			if b.Scope != want.scope || b.WindowSeconds != want.window ||
				b.Limit != want.limit || b.Key == ([32]byte{}) {
				t.Fatalf("login admission static policy drifted at index %d", i)
			}
		}
		copied := append([]AdmissionBucket(nil), store.buckets...)
		raw, err := json.Marshal(copied)
		if err != nil || bytes.Contains(raw, []byte(email)) ||
			bytes.Contains(raw, []byte("198.51.100")) ||
			bytes.Contains(raw, []byte("2001:db8")) {
			t.Fatal("raw email/network escaped into persistent LOGIN accounting")
		}
		return copied
	}
	a := admit("ALICE@EXAMPLE.TEST", "2001:db8:2:3::1")
	b := admit("alice@example.test", "2001:db8:2:3::ffff")
	c := admit("bob@example.test", "2001:db8:2:3::1")
	d := admit("alice@example.test", "2001:db8:2:4::1")
	e := admit("alice@example.test", "198.51.100.10")
	f := admit("alice@example.test", "198.51.100.11")
	for _, comparison := range [][]AdmissionBucket{b, c, d, e, f} {
		if comparison[0].Key != a[0].Key {
			t.Fatal("GLOBAL budget key differed across valid clients or emails")
		}
	}
	if a[1].Key != b[1].Key || a[3].Key != b[3].Key ||
		a[1].Key != c[1].Key || a[3].Key == c[3].Key ||
		a[1].Key == d[1].Key || a[3].Key != d[3].Key ||
		e[1].Key == f[1].Key || a[3].Key != e[3].Key ||
		a[1].Key != a[2].Key || a[3].Key != a[4].Key {
		// The same IP/alias keys are intentionally shared between windows,
		// while scope and action are explicit columns in the database.
		t.Fatal("canonical alias or client prefix key separation drifted")
	}
	// Existing REGISTER/RESEND/CONFIRM budgets must never be silently shared
	// with LOGIN even if one master key is reused by a test fixture.
	other, err := NewAnonymousAdmission(store, secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Admit(context.Background(), AnonymousResend, "alice@example.test",
		netip.MustParseAddr("198.51.100.10")); err != nil {
		t.Fatal(err)
	}
	if store.buckets[0].Key == e[1].Key || store.buckets[2].Key == e[3].Key {
		t.Fatal("login HMAC keys reused registration/resend accounting namespace")
	}
}

func TestLoginAdmissionInvalidFailClosedAndRedacted(t *testing.T) {
	if _, err := NewLoginAdmission(nil, bytes.Repeat([]byte{0xaa}, 32)); !errors.Is(err, ErrLoginAdmissionUnavailable) {
		t.Fatal("nil durable authority accepted")
	}
	if _, err := NewLoginAdmission(&fakeAnonymousAdmissionStore{}, []byte("short")); !errors.Is(err, ErrLoginAdmissionUnavailable) {
		t.Fatal("unkeyed login HMAC accepted")
	}
	s := &fakeAnonymousAdmissionStore{allowed: true}
	g, _ := NewLoginAdmission(s, bytes.Repeat([]byte{0xbb}, 32))
	for _, tc := range []struct {
		email string
		peer  netip.Addr
	}{
		{"nope", netip.MustParseAddr("198.51.100.2")},
		{"alice@example.test", netip.Addr{}},
		{"alice@example.test", netip.MustParseAddr("::")},
		{"alice@example.test", netip.MustParseAddr("0.0.0.0")},
		{"alice@example.test", netip.MustParseAddr("ff02::1")},
		{"alice@example.test", netip.MustParseAddr("fe80::1%test0")},
	} {
		if ok, err := g.Admit(context.Background(), tc.email, tc.peer); ok ||
			!errors.Is(err, ErrLoginAdmissionInvalid) {
			t.Fatal("invalid alias/peer reached admission")
		}
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if ok, err := g.Admit(ctx, "alice@example.test", netip.MustParseAddr("198.51.100.2")); ok ||
		!errors.Is(err, ErrLoginAdmissionUnavailable) {
		t.Fatal("canceled login admission reached store")
	}
	if s.calls != 0 {
		t.Fatal("invalid input triggered database")
	}
	s.err = errors.New("private PostgreSQL DSN and email details")
	if ok, err := g.Admit(context.Background(), "alice@example.test", netip.MustParseAddr("198.51.100.2")); ok ||
		!errors.Is(err, ErrLoginAdmissionUnavailable) ||
		err.Error() != ErrLoginAdmissionUnavailable.Error() {
		t.Fatal("login storage outage leaked internal details")
	}
	s.err = nil
	s.allowed = false
	if ok, err := g.Admit(context.Background(), "alice@example.test", netip.MustParseAddr("198.51.100.2")); ok || err != nil {
		t.Fatal("exhausted login budget did not fail closed")
	}
	var nilGate *LoginAdmission
	if ok, err := nilGate.Admit(context.Background(), "alice@example.test", netip.MustParseAddr("198.51.100.2")); ok ||
		!errors.Is(err, ErrLoginAdmissionUnavailable) {
		t.Fatal("nil login admission incorrectly accepted")
	}
}
