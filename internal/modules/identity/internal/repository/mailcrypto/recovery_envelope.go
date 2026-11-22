package mailcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"

	"strings"
)

var ErrRecoveryEnvelope = errors.New("recovery envelope unavailable")

// RecoveryKeyring intentionally has a separate type and secret configuration
// from registration verification envelopes. The same key is NOT supplied by
// the application wiring: recovery keys must come from separately scoped
// deployment secrets. The ciphertext has a recovery-specific AEAD AAD.
type RecoveryKeyring struct {
	active string
	keys   map[string]cipher.AEAD
}

func NewRecoveryKeyring(active string, material map[string][]byte) (*RecoveryKeyring, error) {
	if !validID(active) || len(material) == 0 || len(material) > 8 {
		return nil, ErrRecoveryEnvelope
	}
	keys := make(map[string]cipher.AEAD, len(material))
	for id, raw := range material {
		if !validID(id) || len(raw) != 32 {
			return nil, ErrRecoveryEnvelope
		}
		block, err := aes.NewCipher(raw)
		if err != nil {
			return nil, ErrRecoveryEnvelope
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, ErrRecoveryEnvelope
		}
		keys[id] = aead
	}
	if _, ok := keys[active]; !ok {
		return nil, ErrRecoveryEnvelope
	}
	return &RecoveryKeyring{active: active, keys: keys}, nil
}

// AAD input is already canonically validated by the usecase and persisted
// SQL. Keep this defensive syntax check local to the crypto adapter so the
// repository cannot import Identity domain (enforced by archlint).
func validRecoveryEnvelopeEmail(email string) bool {
	if len(email) < 3 || len(email) > 254 || strings.ToLower(email) != email ||
		strings.TrimSpace(email) != email || strings.Count(email, "@") != 1 {
		return false
	}
	local, host, ok := strings.Cut(email, "@")
	if !ok || len(local) == 0 || len(local) > 64 ||
		strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") ||
		strings.Contains(local, "..") || strings.ContainsAny(local, "()<>[]:;\\\" ,") ||
		!strings.Contains(host, ".") || strings.HasPrefix(host, ".") ||
		strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return false
	}
	for _, ch := range local {
		if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' ||
			strings.ContainsRune("!#$%&'*+-/=?^_`{|}~.", ch)) {
			return false
		}
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' ||
			label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
				return false
			}
		}
	}
	return true
}

func recoveryAAD(email, challenge string) ([]byte, error) {
	if !validRecoveryEnvelopeEmail(email) || len(challenge) != 26 ||
		challenge[:4] != "rcv_" {
		return nil, ErrRecoveryEnvelope
	}
	for _, ch := range challenge[4:] {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' ||
			ch >= '0' && ch <= '9' || ch == '_' || ch == '-') {
			return nil, ErrRecoveryEnvelope
		}
	}
	return []byte("mamony:identity:recovery-envelope:v1\x00" +
		email + "\x00" + challenge + "\x00PASSWORD_RECOVERY"), nil
}

// SealRecovery authenticates the exact verified alias, challenge and purpose.
// No plaintext bearer may enter SQL or logs. Output is nonce||AES-GCM blob.
func (k *RecoveryKeyring) SealRecovery(email, challenge, bearer string) (string, []byte, error) {
	if k == nil || len(bearer) != 43 {
		return "", nil, ErrRecoveryEnvelope
	}
	aad, err := recoveryAAD(email, challenge)
	if err != nil {
		return "", nil, ErrRecoveryEnvelope
	}
	raw, err := base64.RawURLEncoding.DecodeString(bearer)
	if err != nil || len(raw) != 32 ||
		base64.RawURLEncoding.EncodeToString(raw) != bearer {
		return "", nil, ErrRecoveryEnvelope
	}
	aead, ok := k.keys[k.active]
	if !ok {
		return "", nil, ErrRecoveryEnvelope
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", nil, ErrRecoveryEnvelope
	}
	encrypted := aead.Seal(nil, nonce, []byte(bearer), aad)
	return k.active, append(nonce, encrypted...), nil
}

// OpenRecovery fails closed for wrong recipient/challenge/purpose/key,
// truncated or tampered ciphertext. Never disclose returned token in logs.
func (k *RecoveryKeyring) OpenRecovery(email, challenge, keyID string, blob []byte) (string, error) {
	if k == nil || !validID(keyID) || len(blob) > 256 {
		return "", ErrRecoveryEnvelope
	}
	aad, err := recoveryAAD(email, challenge)
	if err != nil {
		return "", ErrRecoveryEnvelope
	}
	aead, ok := k.keys[keyID]
	if !ok || len(blob) != aead.NonceSize()+43+aead.Overhead() {
		return "", ErrRecoveryEnvelope
	}
	plain, err := aead.Open(nil, blob[:aead.NonceSize()], blob[aead.NonceSize():], aad)
	if err != nil {
		return "", ErrRecoveryEnvelope
	}
	raw, err := base64.RawURLEncoding.DecodeString(string(plain))
	if err != nil || len(raw) != 32 ||
		base64.RawURLEncoding.EncodeToString(raw) != string(plain) {
		return "", ErrRecoveryEnvelope
	}
	return string(plain), nil
}
