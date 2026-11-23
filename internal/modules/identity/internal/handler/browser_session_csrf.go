package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"time"
)

const (
	csrfValidity = 15 * time.Minute
	csrfDomain   = "mamony:identity:browser-csrf:v1\x00"
)

type sessionCSRF struct {
	key    [32]byte
	origin string
	clock  func() time.Time
}

func newSessionCSRF(key []byte, origin string) (*sessionCSRF, error) {
	if len(key) != 32 || origin == "" {
		return nil, ErrBrowserSessionHTTPConfig
	}
	c := &sessionCSRF{origin: origin, clock: time.Now}
	copy(c.key[:], key)
	return c, nil
}

// CSRF token: eight-byte big-endian Unix expiry and a 32-byte HMAC.
// The HMAC binds the token to the current opaque browser session and trusted
// origin. The raw bearer never enters the CSRF token or response. A separate
// production-managed secret is mandatory; no default or static dev key exists.
func (c *sessionCSRF) issue(session string) string {
	if c == nil || c.clock == nil || !canonicalSessionBearer(session) {
		return ""
	}
	var token [40]byte
	now := c.clock().UTC()
	// Encode a signed Unix timestamp directly: no unchecked int64-to-uint64
	// conversion. The 8-byte buffer uses the standard big-endian wire form.
	if err := binary.Write(bytes.NewBuffer(token[:0]), binary.BigEndian, now.Add(csrfValidity).Unix()); err != nil {
		return ""
	}
	mac := c.signature(token[:8], session)
	copy(token[8:], mac[:])
	return base64.RawURLEncoding.EncodeToString(token[:])
}

func (c *sessionCSRF) verify(session, presented string) bool {
	if c == nil || c.clock == nil || !canonicalSessionBearer(session) ||
		len(presented) != base64.RawURLEncoding.EncodedLen(40) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(presented)
	if err != nil || len(decoded) != 40 ||
		base64.RawURLEncoding.EncodeToString(decoded) != presented {
		return false
	}
	now := c.clock().UTC().Unix()
	var expires int64
	if err := binary.Read(bytes.NewReader(decoded[:8]), binary.BigEndian, &expires); err != nil {
		return false
	}
	if expires <= now || expires > now+int64(csrfValidity/time.Second) {
		return false
	}
	expected := c.signature(decoded[:8], session)
	return subtle.ConstantTimeCompare(expected[:], decoded[8:]) == 1
}

func (c *sessionCSRF) signature(expiry []byte, session string) [32]byte {
	mac := hmac.New(sha256.New, c.key[:])
	_, _ = mac.Write([]byte(csrfDomain))
	_, _ = mac.Write(expiry)
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(c.origin))
	_, _ = mac.Write([]byte{0})
	// The session bearer is never disclosed in the signed token. SHA-256 of
	// the uniformly random bearer is a safe fixed-size binding input.
	sessionDigest := sha256.Sum256([]byte(session))
	_, _ = mac.Write(sessionDigest[:])
	var result [32]byte
	copy(result[:], mac.Sum(nil))
	return result
}
