//go:build integration

package fixture

import (
	"bytes"
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

// EnrollmentHasher returns the synthetic verifier used by enrollment tests.
type EnrollmentHasher struct{}

func (EnrollmentHasher) Hash(_ context.Context, _ string) (domain.PasswordVerifier, error) {
	return domain.PasswordVerifier{
		Version: 1, MemoryKiB: 65536, Iterations: 3, Parallelism: 1,
		Salt: bytes.Repeat([]byte{0x27}, 16), Digest: bytes.Repeat([]byte{0x48}, 32),
	}, nil
}
