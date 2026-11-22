package mailcrypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
)

func TestRecoveryEnvelopeIsolationAndRotation(t *testing.T) {
	oldKey := bytes.Repeat([]byte{0x42}, 32)
	newKey := bytes.Repeat([]byte{0x62}, 32)
	ring, err := NewRecoveryKeyring("recovery-new", map[string][]byte{
		"recovery-old": oldKey, "recovery-new": newKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	old, err := NewRecoveryKeyring("recovery-old", map[string][]byte{
		"recovery-old": oldKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	bearer := base64.RawURLEncoding.EncodeToString(raw)
	email := "owner@example.test"
	challenge := "rcv_AAAAAAAAAAAAAAAAAAAAAA"
	key, blob, err := ring.SealRecovery(email, challenge, bearer)
	if err != nil || key != "recovery-new" || len(blob) != 71 ||
		bytes.Contains(blob, []byte(bearer)) {
		t.Fatal("recovery proof was not encrypted with expected key")
	}
	if got, err := ring.OpenRecovery(email, challenge, key, blob); err != nil || got != bearer {
		t.Fatal("recovery AEAD roundtrip failed")
	}
	_, second, err := ring.SealRecovery(email, challenge, bearer)
	if err != nil || bytes.Equal(blob, second) {
		t.Fatal("recovery envelope nonce was reused")
	}
	for _, tc := range []struct{ email, id, key string }{
		{"attacker@example.test", challenge, key},
		{email, "rcv_BBBBBBBBBBBBBBBBBBBBBB", key},
		{email, challenge, "recovery-old"},
		{email, challenge, "missing"},
		{"owner@EXAMPLE.test", challenge, key},
	} {
		if _, err := ring.OpenRecovery(tc.email, tc.id, tc.key, blob); !errors.Is(err, ErrRecoveryEnvelope) {
			t.Fatal("recovery token escaped bound alias/challenge/key")
		}
	}
	tampered := append([]byte(nil), blob...)
	tampered[len(tampered)-1] ^= 1
	if _, err := ring.OpenRecovery(email, challenge, key, tampered); !errors.Is(err, ErrRecoveryEnvelope) {
		t.Fatal("tampered ciphertext was accepted")
	}
	registration, err := NewKeyring("recovery-new", map[string][]byte{"recovery-new": newKey})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registration.Open("sub_AAAAAAAAAAAAAAAAAAAAAA",
		"vch_BBBBBBBBBBBBBBBBBBBBBB", key, blob); !errors.Is(err, ErrEnvelope) {
		t.Fatal("recovery ciphertext was accepted as email verification proof")
	}
	oldID, oldBlob, err := old.SealRecovery(email, challenge, bearer)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ring.OpenRecovery(email, challenge, oldID, oldBlob); err != nil || got != bearer {
		t.Fatal("retained read key could not drain prior recovery queue")
	}
	if _, err := old.OpenRecovery(email, challenge, key, blob); !errors.Is(err, ErrRecoveryEnvelope) {
		t.Fatal("unrecognized new key was accepted by old-only worker")
	}
}

func TestRecoveryEnvelopeRejectsInvalidMaterials(t *testing.T) {
	if _, err := NewRecoveryKeyring("", map[string][]byte{"x": make([]byte, 32)}); err == nil {
		t.Fatal("missing active recovery key accepted")
	}
	if _, err := NewRecoveryKeyring("x", map[string][]byte{"x": make([]byte, 31)}); err == nil {
		t.Fatal("short recovery key accepted")
	}
	if _, err := NewRecoveryKeyring("x", map[string][]byte{"y": make([]byte, 32)}); err == nil {
		t.Fatal("unknown active recovery key accepted")
	}
	k, err := NewRecoveryKeyring("x", map[string][]byte{"x": bytes.Repeat([]byte{0x13}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ email, challenge, bearer string }{
		{"BAD", "rcv_AAAAAAAAAAAAAAAAAAAAAA", "bad"},
		{"owner@example.test", "vch_AAAAAAAAAAAAAAAAAAAAAA", "bad"},
		{"owner@example.test", "rcv_AAAAAAAAAAAAAAAAAAAAAA", "bad"},
		{"owner@example.test", "rcv_AAAAAAAAAAAAAAAAAAAAAA", ""},
	} {
		if _, _, err := k.SealRecovery(tc.email, tc.challenge, tc.bearer); !errors.Is(err, ErrRecoveryEnvelope) {
			t.Fatal("malformed recovery proof or AAD accepted")
		}
	}
}
