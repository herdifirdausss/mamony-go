package mailcrypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
)

func TestEnvelopeAuthenticationAndKeyRotation(t *testing.T) {
	current := bytes.Repeat([]byte{0x23}, 32)
	old := bytes.Repeat([]byte{0x45}, 32)
	a, err := NewKeyring("key-v2", map[string][]byte{"key-v1": old, "key-v2": current})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewKeyring("key-v1", map[string][]byte{"key-v1": old})
	if err != nil {
		t.Fatal(err)
	}
	subject := "sub_AAAAAAAAAAAAAAAAAAAAAA"
	challenge := "vch_BBBBBBBBBBBBBBBBBBBBBB"
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	key, blob, err := a.Seal(subject, challenge, token)
	if err != nil || key != "key-v2" || len(blob) != 71 || bytes.Contains(blob, []byte(token)) {
		t.Fatal("invalid sealed envelope")
	}
	_, blob2, err := a.Seal(subject, challenge, token)
	if err != nil || bytes.Equal(blob, blob2) {
		t.Fatal("nonce reused")
	}
	got, err := a.Open(subject, challenge, key, blob)
	if err != nil || got != token {
		t.Fatalf("envelope roundtrip: %v", err)
	}
	for _, tc := range []struct {
		subject, challenge, key string
		blob                    []byte
	}{
		{"sub_CCCCCCCCCCCCCCCCCCCCCC", challenge, key, blob},
		{subject, "vch_CCCCCCCCCCCCCCCCCCCCCC", key, blob},
		{subject, challenge, "key-v1", blob},
		{subject, challenge, "unknown", blob},
	} {
		if _, err := a.Open(tc.subject, tc.challenge, tc.key, tc.blob); !errors.Is(err, ErrEnvelope) {
			t.Fatal("wrong binding/key accepted")
		}
	}
	tampered := append([]byte(nil), blob...)
	tampered[len(tampered)-1] ^= 1
	if _, err := a.Open(subject, challenge, key, tampered); !errors.Is(err, ErrEnvelope) {
		t.Fatal("tampered envelope accepted")
	}
	if _, err := b.Open(subject, challenge, key, blob); !errors.Is(err, ErrEnvelope) {
		t.Fatal("unknown rotation key accepted")
	}
	oldID, oldBlob, err := b.Seal(subject, challenge, token)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := a.Open(subject, challenge, oldID, oldBlob); err != nil || got != token {
		t.Fatal("retired read key unavailable")
	}
}

func TestEnvelopeRejectsBrokenDeploymentConfig(t *testing.T) {
	for _, tc := range []struct {
		id  string
		raw []byte
	}{
		{"", bytes.Repeat([]byte{1}, 32)},
		{"has space", bytes.Repeat([]byte{1}, 32)},
		{"key-1", []byte("too-short")},
	} {
		if _, err := NewKeyring(tc.id, map[string][]byte{tc.id: tc.raw}); err == nil {
			t.Fatal("invalid key accepted")
		}
	}
	k, err := NewKeyring("key-1", map[string][]byte{"key-1": bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := k.Seal("sub_AAAAAAAAAAAAAAAAAAAAAA", "vch_BBBBBBBBBBBBBBBBBBBBBB", "not-a-token"); err == nil {
		t.Fatal("invalid bearer sealed")
	}
}
