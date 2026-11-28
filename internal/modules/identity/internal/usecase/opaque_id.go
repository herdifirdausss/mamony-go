package usecase

import (
	"encoding/base64"
	"errors"
	"io"
)

var errOpaqueIDUnavailable = errors.New("identity opaque ID unavailable")

// newOpaqueID creates a caller-prefixed opaque ID from 128 random bits.
// Unpadded base64url encoding keeps the generated value URL-safe.
func newOpaqueID(r io.Reader, prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", errOpaqueIDUnavailable
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}
