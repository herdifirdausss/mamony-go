//go:build integration

package webauthn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"

	"github.com/fxamacker/cbor/v2"
)

// SyntheticActivationSigner is a TEST-ONLY real ES256 authenticator. It
// generates an ephemeral, nonproduction P-256 key and signs the exact
// CTAP2 authenticatorData || SHA256(clientDataJSON) assertion byte stream.
type SyntheticActivationSigner struct {
	private *ecdsa.PrivateKey
}

func NewSyntheticActivationSigner() (*SyntheticActivationSigner, error) {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return &SyntheticActivationSigner{private: private}, nil
}

func (s *SyntheticActivationSigner) PublicCOSE() ([]byte, error) {
	if s == nil || s.private == nil {
		return nil, errors.New("missing test key")
	}
	publicECDH, err := s.private.PublicKey.ECDH()
	if err != nil {
		return nil, err
	}
	pub := publicECDH.Bytes()
	return cbor.Marshal(map[int]any{
		1: 2, 3: -7, -1: 1, -2: pub[1:33], -3: pub[33:65],
	})
}

// SignedResponse uses the exact v34 expected challenge and DB-derived
// user handle for a real cryptographic assertion, not a mocked return.
func (s *SyntheticActivationSigner) SignedResponse(
	challenge [32]byte, credentialID []byte, subject string, counter uint32) ([]byte, error) {
	return s.SignedResponseWithFlags(challenge, credentialID, subject, counter, 0x05)
}

// SignedResponseWithFlags signs arbitrary authenticator UP/UV/BE/BS flags
// using the REAL test-only ES256 private key. It deliberately permits
// invalid flags so adversarial negative cases reach the actual verifier.
func (s *SyntheticActivationSigner) SignedResponseWithFlags(
	challenge [32]byte, credentialID []byte, subject string, counter uint32,
	flags byte) ([]byte, error) {
	if s == nil || s.private == nil || len(credentialID) < 16 ||
		challenge == ([32]byte{}) || subject == "" {
		return nil, errors.New("invalid test assertion inputs")
	}
	client, err := json.Marshal(map[string]any{
		"type":        "webauthn.get",
		"origin":      "https://login.example.test",
		"challenge":   base64.RawURLEncoding.EncodeToString(challenge[:]),
		"crossOrigin": false,
	})
	if err != nil {
		return nil, err
	}
	rpHash := sha256.Sum256([]byte("login.example.test"))
	authData := append([]byte(nil), rpHash[:]...)
	authData = append(authData, flags) // signed authenticator flags, including BE/BS
	var count [4]byte
	binary.BigEndian.PutUint32(count[:], counter)
	authData = append(authData, count[:]...)
	clientHash := sha256.Sum256(client)
	signed := append(append([]byte(nil), authData...), clientHash[:]...)
	hash := sha256.Sum256(signed)
	sig, err := ecdsa.SignASN1(rand.Reader, s.private, hash[:])
	if err != nil {
		return nil, err
	}
	id := base64.RawURLEncoding.EncodeToString(credentialID)
	return json.Marshal(map[string]any{
		"id": id, "rawId": id, "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    base64.RawURLEncoding.EncodeToString(client),
			"authenticatorData": base64.RawURLEncoding.EncodeToString(authData),
			"signature":         base64.RawURLEncoding.EncodeToString(sig),
			"userHandle":        base64.RawURLEncoding.EncodeToString([]byte(subject)),
		},
	})
}
