package domain

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"io"

	"golang.org/x/crypto/argon2"
)

var (
	ErrKDFUnavailable = errors.New("password processing unavailable")
	ErrVerifierInvalid = errors.New("stored password verifier is not supported")
)

// Parameters are a versioned allowlist, not attacker-provided KDF settings.
// v1 is a benchmark candidate; capacity approval is still required.
const (
	passwordVersion uint16 = 1
	passwordMemoryKiB uint32 = 64 * 1024
	passwordIterations uint32 = 3
	passwordParallelism uint8 = 1
	passwordSaltBytes = 16
	passwordHashBytes = 32
)

// PasswordVerifier holds no plaintext password or reversible secret. Its
// exported fields are persistence data, NOT a trusted request contract.
type PasswordVerifier struct {
	Version uint16
	MemoryKiB uint32
	Iterations uint32
	Parallelism uint8
	Salt []byte
	Digest []byte
}

// PasswordHasher bounds concurrent memory-hard operations. The admission
// budget must be measured at the actual deployment size before any public use.
type PasswordHasher struct {
	slots chan struct{}
	random io.Reader
}

func NewPasswordHasher(maxConcurrent int) (*PasswordHasher, error) {
	if maxConcurrent < 1 || maxConcurrent > 4 {
		return nil, ErrKDFUnavailable
	}
	return &PasswordHasher{slots: make(chan struct{}, maxConcurrent), random: rand.Reader}, nil
}

func (h *PasswordHasher) acquire(ctx context.Context) error {
	if h == nil || h.slots == nil {
		return ErrKDFUnavailable
	}
	select {
	case h.slots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-h.slots
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Hash expects a policy-validated, NFC-normalized password.
func (h *PasswordHasher) Hash(ctx context.Context, normalized string) (PasswordVerifier, error) {
	if _, err := NormalizeForVerification(normalized); err != nil {
		return PasswordVerifier{}, err
	}
	if err := h.acquire(ctx); err != nil {
		return PasswordVerifier{}, err
	}
	defer func(){ <-h.slots }()
	salt := make([]byte, passwordSaltBytes)
	if _, err := io.ReadFull(h.random, salt); err != nil {
		return PasswordVerifier{}, ErrKDFUnavailable
	}
	digest := argon2.IDKey([]byte(normalized), salt, passwordIterations, passwordMemoryKiB, passwordParallelism, passwordHashBytes)
	return PasswordVerifier{Version: passwordVersion, MemoryKiB: passwordMemoryKiB, Iterations: passwordIterations, Parallelism: passwordParallelism, Salt: salt, Digest: digest}, nil
}

// Verify rejects corrupted/oversized DB parameters BEFORE any expensive KDF.
func (h *PasswordHasher) Verify(ctx context.Context, password string, stored PasswordVerifier) (bool, error) {
	if stored.Version != passwordVersion || stored.MemoryKiB != passwordMemoryKiB || stored.Iterations != passwordIterations ||
		stored.Parallelism != passwordParallelism || len(stored.Salt) != passwordSaltBytes || len(stored.Digest) != passwordHashBytes {
		return false, ErrVerifierInvalid
	}
	normalized, err := NormalizeForVerification(password)
	if err != nil {
		return false, err
	}
	if err := h.acquire(ctx); err != nil {
		return false, err
	}
	defer func(){ <-h.slots }()
	got := argon2.IDKey([]byte(normalized), stored.Salt, stored.Iterations, stored.MemoryKiB, stored.Parallelism, passwordHashBytes)
	return subtle.ConstantTimeCompare(got, stored.Digest) == 1, nil
}
