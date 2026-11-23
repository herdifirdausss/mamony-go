//go:build integration

package webauthn

import (
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"

	"github.com/fxamacker/cbor/v2"
)

// SyntheticRegistrationResponse is TEST-ONLY, compiled under -tags=integration.
// It supplies a deterministic, structurally valid ES256 fmt:none creation
// response for the full maintained-library/DB staging test. This attestation
// asserts NO device provenance or possession in a later authentication.
func SyntheticRegistrationResponse(nonce, credentialID []byte) ([]byte, error) {
	if len(nonce) != 32 || len(credentialID) < 16 || len(credentialID) > 65535 {
		return nil, errors.New("invalid synthetic WebAuthn ceremony inputs")
	}
	scalar := make([]byte, 32)
	scalar[31] = 7
	private, err := ecdh.P256().NewPrivateKey(scalar)
	if err != nil {
		return nil, err
	}
	pub := private.PublicKey().Bytes()
	cose, err := cbor.Marshal(map[int]any{
		1: 2, 3: -7, -1: 1, -2: pub[1:33], -3: pub[33:65],
	})
	if err != nil {
		return nil, err
	}
	rpHash := sha256.Sum256([]byte("login.example.test"))
	authData := append([]byte(nil), rpHash[:]...)
	authData = append(authData, 0x45) // UP + UV + AT
	authData = append(authData, make([]byte, 20)...) // count + AAGUID
	// Credential ID length is bounded before converting any width.
	var idLength uint16
	for range credentialID {
		idLength++
	}
	var encodedLength [2]byte
	binary.BigEndian.PutUint16(encodedLength[:], idLength)
	authData = append(authData, encodedLength[:]...)
	authData = append(authData, credentialID...)
	authData = append(authData, cose...)
	attestation, err := cbor.Marshal(map[string]any{
		"fmt": "none", "authData": authData, "attStmt": map[string]any{},
	})
	if err != nil {
		return nil, err
	}
	clientData, err := json.Marshal(map[string]any{
		"type": "webauthn.create",
		"origin": "https://login.example.test",
		"challenge": base64.RawURLEncoding.EncodeToString(nonce),
		"crossOrigin": false,
	})
	if err != nil {
		return nil, err
	}
	id := base64.RawURLEncoding.EncodeToString(credentialID)
	return json.Marshal(map[string]any{
		"id": id, "rawId": id, "type": "public-key",
		"response": map[string]any{
			"clientDataJSON": base64.RawURLEncoding.EncodeToString(clientData),
			"attestationObject": base64.RawURLEncoding.EncodeToString(attestation),
		},
	})
}
