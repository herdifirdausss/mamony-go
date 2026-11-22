package usecase

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
)

var (
	ErrOfflineRecoveryCodeInvalid = errors.New("offline recovery code invalid")
	ErrOfflineRecoveryCodeUnavailable = errors.New("offline recovery code unavailable")
)

const offlineRecoveryCodeDomain = "mamony:identity:offline-recovery-code:v1\x00"
const offlineRecoveryCodeLength = 43

// OfflineRecoveryCode is 32 bytes of CSPRNG-generated account recovery
// evidence to be saved OFFLINE before a later password-recovery request.
// No live enrollment ceremony is implemented; a factory alone does not
// provision anything. NEVER treat an emailed code as this factor.
type OfflineRecoveryCode struct {
	bearer string
	digest [32]byte
}

// NewOfflineRecoveryCode only generates offline material. It does not
// enroll/approve it and must never appear in an anonymous HTTP response.
func NewOfflineRecoveryCode() (OfflineRecoveryCode, error) {
	return newOfflineRecoveryCode(rand.Reader)
}

func newOfflineRecoveryCode(source io.Reader) (OfflineRecoveryCode, error) {
	if source == nil {
		return OfflineRecoveryCode{}, ErrOfflineRecoveryCodeUnavailable
	}
	secret := make([]byte, 32)
	if _, err := io.ReadFull(source, secret); err != nil {
		clear(secret)
		return OfflineRecoveryCode{}, ErrOfflineRecoveryCodeUnavailable
	}
	code := OfflineRecoveryCode{
		bearer: base64.RawURLEncoding.EncodeToString(secret),
		digest: offlineRecoveryDigest(secret),
	}
	clear(secret)
	return code, nil
}

func offlineRecoveryDigest(secret []byte) [32]byte {
	h := sha256.New()
	_, _ = h.Write([]byte(offlineRecoveryCodeDomain))
	_, _ = h.Write(secret)
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

// OfflineRecoveryCodeDigest accepts only the exact canonical encoding of
// 256 random bits. Ownership, enrollment and single-use are SQL decisions.
func OfflineRecoveryCodeDigest(bearer string) ([32]byte, error) {
	var zero [32]byte
	if len(bearer) != offlineRecoveryCodeLength {
		return zero, ErrOfflineRecoveryCodeInvalid
	}
	secret, err := base64.RawURLEncoding.DecodeString(bearer)
	if err != nil || len(secret) != 32 ||
		base64.RawURLEncoding.EncodeToString(secret) != bearer {
		clear(secret)
		return zero, ErrOfflineRecoveryCodeInvalid
	}
	digest := offlineRecoveryDigest(secret)
	clear(secret)
	return digest, nil
}

// TrustedOfflineDisplay may be used ONLY by a separately reviewed
// enrollment ceremony, never by account-recovery requests or logging.
func (c OfflineRecoveryCode) TrustedOfflineDisplay() string { return c.bearer }
func (c OfflineRecoveryCode) Digest() [32]byte { return c.digest }

const offlineRecoveryCodeRedaction = "[REDACTED offline recovery code]"

func (OfflineRecoveryCode) String() string { return offlineRecoveryCodeRedaction }
func (OfflineRecoveryCode) GoString() string { return offlineRecoveryCodeRedaction }
func (OfflineRecoveryCode) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, offlineRecoveryCodeRedaction)
}
func (OfflineRecoveryCode) LogValue() slog.Value {
	return slog.StringValue(offlineRecoveryCodeRedaction)
}
func (OfflineRecoveryCode) MarshalJSON() ([]byte, error) {
	return nil, ErrOfflineRecoveryCodeInvalid
}
func (OfflineRecoveryCode) MarshalText() ([]byte, error) {
	return nil, ErrOfflineRecoveryCodeInvalid
}
