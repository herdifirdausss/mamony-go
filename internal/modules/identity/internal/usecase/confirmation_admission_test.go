package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"testing"
)

func TestConfirmationAdmissionPrivacyAndNetworkBounds(t *testing.T) {
	store := &fakeAnonymousAdmissionStore{allowed: true}
	gate, err := NewConfirmationAdmission(store, bytes.Repeat([]byte{0x76}, 32))
	if err != nil {
		t.Fatal(err)
	}
	one := netip.MustParseAddr("2001:db8:abcd:1::10")
	two := netip.MustParseAddr("2001:db8:abcd:1::ffff")
	third := netip.MustParseAddr("2001:db8:abcd:2::1")
	check := func(ip netip.Addr) []AdmissionBucket {
		t.Helper()
		ok, err := gate.Admit(context.Background(), ip)
		if err != nil || !ok || store.action != AnonymousConfirm ||
			len(store.buckets) != 2 {
			t.Fatalf("confirmation gate rejected valid peer: %v %v", ok, err)
		}
		if store.buckets[0].Scope != "CLIENT_MINUTE" ||
			store.buckets[0].Limit != 8 || store.buckets[0].WindowSeconds != 60 ||
			store.buckets[1].Scope != "CLIENT_HOUR" ||
			store.buckets[1].Limit != 80 || store.buckets[1].WindowSeconds != 3600 ||
			store.buckets[0].Key != store.buckets[1].Key ||
			store.buckets[0].Key == ([32]byte{}) {
			t.Fatal("confirmation policy drifted or stored raw address")
		}
		return append([]AdmissionBucket(nil), store.buckets...)
	}
	a := check(one)
	b := check(two)
	c := check(third)
	if a[0].Key != b[0].Key || a[0].Key == c[0].Key {
		t.Fatal("IPv6 /64 rotation or prefix isolation failed")
	}
	d := check(netip.MustParseAddr("198.51.100.20"))
	e := check(netip.MustParseAddr("198.51.100.21"))
	if d[0].Key == e[0].Key {
		t.Fatal("different IPv4 clients share a budget")
	}
	encoded, err := json.Marshal(d)
	if err != nil || bytes.Contains(encoded, []byte("198.51.100")) ||
		bytes.Contains(encoded, []byte("2001:db8")) {
		t.Fatal("client IP leaked into persistent accounting command")
	}
	// A different anonymous action must have independent domain-separated
	// HMAC keys even when the same master secret and peer are used.
	anon, err := NewAnonymousAdmission(store, bytes.Repeat([]byte{0x76}, 32))
	if err != nil {
		t.Fatal(err)
	}
	_, err = anon.Admit(context.Background(), AnonymousResend, "alice@example.test",
		netip.MustParseAddr("198.51.100.20"))
	if err != nil || store.buckets[0].Key == d[0].Key {
		t.Fatal("confirmation reused resend limiter identity")
	}
}

func TestConfirmationAdmissionDeniesInvalidInputAndRedactsStorageFailures(t *testing.T) {
	if _, err := NewConfirmationAdmission(nil, bytes.Repeat([]byte{1}, 32)); !errors.Is(err, ErrAnonymousAdmissionUnavailable) {
		t.Fatal("missing distributed authority accepted")
	}
	if _, err := NewConfirmationAdmission(&fakeAnonymousAdmissionStore{}, []byte("short")); !errors.Is(err, ErrAnonymousAdmissionUnavailable) {
		t.Fatal("weak HMAC key accepted")
	}
	store := &fakeAnonymousAdmissionStore{allowed: true}
	gate, _ := NewConfirmationAdmission(store, bytes.Repeat([]byte{0x89}, 32))
	badPeers := []netip.Addr{
		{},
		netip.MustParseAddr("0.0.0.0"),
		netip.MustParseAddr("::"),
		netip.MustParseAddr("ff02::1"),
		netip.MustParseAddr("fe80::1%eth0"),
	}
	for _, bad := range badPeers {
		if ok, err := gate.Admit(context.Background(), bad); ok ||
			!errors.Is(err, ErrAnonymousAdmissionUnavailable) {
			t.Fatal("untrusted network peer admitted")
		}
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if ok, err := gate.Admit(ctx, netip.MustParseAddr("192.0.2.3")); ok ||
		!errors.Is(err, ErrAnonymousAdmissionUnavailable) {
		t.Fatal("canceled context accepted")
	}
	if store.calls != 0 {
		t.Fatal("bad peer reached database")
	}
	store.err = errors.New("sensitive DSN and password")
	if ok, err := gate.Admit(context.Background(), netip.MustParseAddr("192.0.2.3")); ok ||
		!errors.Is(err, ErrAnonymousAdmissionUnavailable) ||
		err.Error() != ErrAnonymousAdmissionUnavailable.Error() {
		t.Fatal("raw storage error escaped")
	}
	store.err = nil
	store.allowed = false
	if ok, err := gate.Admit(context.Background(), netip.MustParseAddr("192.0.2.3")); ok || err != nil {
		t.Fatal("exhausted budget did not return a safe false")
	}
}
