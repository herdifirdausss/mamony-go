package postgres

import "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"

// validVerifier reports whether a password verifier matches the persisted KDF format.
func validVerifier(v domain.PasswordVerifier) bool {
	return v.Version == 1 && v.MemoryKiB == 65536 && v.Iterations == 3 &&
		v.Parallelism == 1 && len(v.Salt) == 16 && len(v.Digest) == 32
}

// isCanonicalEmail reports whether email is already in canonical form.
func isCanonicalEmail(email string) bool {
	canonical, err := domain.CanonicalEmail(email)
	return err == nil && canonical == email
}
