package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"testing"
)

type fakeAnonymousAdmissionStore struct {
	allowed bool
	err     error
	calls   int
	action  AnonymousAction
	buckets []AdmissionBucket
}

func (f *fakeAnonymousAdmissionStore) Spend(_ context.Context, action AnonymousAction, buckets []AdmissionBucket) (bool, error) {
	f.calls++
	f.action = action
	f.buckets = append([]AdmissionBucket(nil), buckets...)
	return f.allowed, f.err
}

func TestAnonymousAdmissionHMACAndWindows(t *testing.T) {
	store := &fakeAnonymousAdmissionStore{allowed: true}
	key := bytes.Repeat([]byte{0xa7}, 32)
	gate, err := NewAnonymousAdmission(store, key)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		action AnonymousAction
		email  string
		ip     string
	}{
		{AnonymousRegister, "ALICE@example.test", "2001:db8:1:1::1"},
		{AnonymousRegister, "alice@example.test", "2001:db8:1:1::2"},
		{AnonymousRegister, "alice@example.test", "198.51.100.10"},
		{AnonymousResend, "alice@example.test", "198.51.100.10"},
	} {
		ok, err := gate.Admit(context.Background(), tc.action, tc.email, netip.MustParseAddr(tc.ip))
		if err != nil || !ok || store.action != tc.action || len(store.buckets) != 4 {
			t.Fatalf("valid admission rejected: ok=%v err=%v", ok, err)
		}
		for _, bucket := range store.buckets {
			if bucket.Key == ([32]byte{}) || bucket.Limit < 1 || bucket.WindowSeconds < 1 {
				t.Fatal("admission bucket did not specify an opaque bounded key")
			}
		}
	}
	var a, b []AdmissionBucket
	_, _ = gate.Admit(context.Background(), AnonymousRegister, "ALICE@example.test", netip.MustParseAddr("2001:db8:1:1::1"))
	a = append([]AdmissionBucket(nil), store.buckets...)
	_, _ = gate.Admit(context.Background(), AnonymousRegister, "alice@example.test", netip.MustParseAddr("2001:db8:1:1::ffff"))
	b = append([]AdmissionBucket(nil), store.buckets...)
	for i := range a {
		if a[i].Key != b[i].Key {
			t.Fatal("canonical email or IPv6 /64 failed to share a budget")
		}
	}
	_, _ = gate.Admit(context.Background(), AnonymousRegister, "alice@example.test", netip.MustParseAddr("2001:db8:1:2::1"))
	if store.buckets[0].Key == a[0].Key || store.buckets[2].Key != a[2].Key {
		t.Fatal("IP prefix and email budgets must be independent")
	}
	_, _ = gate.Admit(context.Background(), AnonymousResend, "alice@example.test", netip.MustParseAddr("2001:db8:1:1::1"))
	if store.buckets[0].Key == a[0].Key || store.buckets[2].Key == a[2].Key {
		t.Fatal("register and resend must not share allowance keys")
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("alice@")) || bytes.Contains(raw, []byte("2001:db8")) {
		t.Fatal("raw email or address escaped into accounting")
	}
	if a[0].Limit != 8 || a[0].WindowSeconds != 60 ||
		a[1].Limit != 80 || a[1].WindowSeconds != 3600 ||
		a[2].Limit != 3 || a[2].WindowSeconds != 3600 ||
		a[3].Limit != 5 || a[3].WindowSeconds != 86400 {
		t.Fatal("static accounting policy drift")
	}
}

func TestAnonymousAdmissionFailClosedAndGeneric(t *testing.T) {
	if _, err := NewAnonymousAdmission(&fakeAnonymousAdmissionStore{}, nil); !errors.Is(err, ErrAnonymousAdmissionUnavailable) {
		t.Fatal("missing keyed identity authority accepted")
	}
	if _, err := NewAnonymousAdmission(nil, bytes.Repeat([]byte{1}, 32)); !errors.Is(err, ErrAnonymousAdmissionUnavailable) {
		t.Fatal("missing distributed budget authority accepted")
	}
	s := &fakeAnonymousAdmissionStore{}
	g, _ := NewAnonymousAdmission(s, bytes.Repeat([]byte{1}, 32))
	for _, tc := range []struct {
		op        AnonymousAction
		email, ip string
	}{
		{"OTHER", "alice@example.test", "127.0.0.1"},
		{AnonymousRegister, "", "127.0.0.1"},
		{AnonymousResend, "nope", "127.0.0.1"},
		{AnonymousRegister, "alice@example.test", "::"},
		{AnonymousResend, "alice@example.test", "ff02::1"},
	} {
		ok, err := g.Admit(context.Background(), tc.op, tc.email, netip.MustParseAddr(tc.ip))
		if ok || !errors.Is(err, ErrAnonymousAdmissionInvalid) {
			t.Fatalf("bad ingress admitted: %v %v", ok, err)
		}
	}
	if s.calls != 0 {
		t.Fatal("invalid input reached persistence")
	}
	s.err = errors.New("sensitive SQL DSN with password")
	ok, err := g.Admit(context.Background(), AnonymousRegister, "alice@example.test", netip.MustParseAddr("127.0.0.1"))
	if ok || !errors.Is(err, ErrAnonymousAdmissionUnavailable) || err.Error() != ErrAnonymousAdmissionUnavailable.Error() {
		t.Fatal("raw SQL error leaked from anonymous admission")
	}
	s.err = nil
	s.allowed = false
	ok, err = g.Admit(context.Background(), AnonymousRegister, "alice@example.test", netip.MustParseAddr("127.0.0.1"))
	if ok || err != nil {
		t.Fatal("denied budget did not return harmless internal false")
	}
}
