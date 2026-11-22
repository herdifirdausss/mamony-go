package postgres

import (
	"context"
	"errors"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// AuthenticationStore has a dedicated SELECT-only credential. It must never
// be wired using the registration runtime or the email verifier role.
type AuthenticationStore struct {
	db Transactor
}

func NewAuthenticationStore(db Transactor) (*AuthenticationStore, error) {
	if db == nil {
		return nil, usecase.ErrPasswordAuthenticationUnavailable
	}
	return &AuthenticationStore{db: db}, nil
}

// LookupCredential is the only password lookup supported by this adapter.
// It fetches minimal subject/status/verified state and allowlisted Argon2id
// parameters. No session, challenge, token or account list is returned.
// pgx.ErrNoRows means an unknown account; EVERY other database failure is
// redacted as unavailable, including a wrong role or missing privileges.
func (s *AuthenticationStore) LookupCredential(ctx context.Context, canonical string) (usecase.AuthenticationCredential, bool, error) {
	var out usecase.AuthenticationCredential
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil || !isCanonicalEmail(canonical) {
		return out, false, usecase.ErrPasswordAuthenticationUnavailable
	}
	var version, parallelism int16
	var memory, iterations int32
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT
				s.subject_id, s.session_epoch, s.status, a.verified_at IS NOT NULL,
				p.kdf_version, p.memory_kib, p.iterations, p.parallelism, p.salt, p.digest
			FROM mamony_identity.email_aliases a
			JOIN mamony_identity.subjects s ON s.subject_id=a.subject_id
			JOIN mamony_identity.password_verifiers p ON p.subject_id=s.subject_id
			WHERE a.canonical_email=$1`, canonical).
			Scan(&out.SubjectID, &out.SessionEpoch, &out.Status, &out.EmailVerified,
				&version, &memory, &iterations, &parallelism,
				&out.Verifier.Salt, &out.Verifier.Digest)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.AuthenticationCredential{}, false, nil
	}
	if err != nil {
		return usecase.AuthenticationCredential{}, false, usecase.ErrPasswordAuthenticationUnavailable
	}
	// Never cast hostile/corrupted SQL values into smaller unsigned types
	// and accidentally admit a wrapped, low-cost KDF configuration.
	if version != 1 || memory != 65536 || iterations != 3 || parallelism != 1 ||
		len(out.Verifier.Salt) != 16 || len(out.Verifier.Digest) != 32 {
		return usecase.AuthenticationCredential{}, false, usecase.ErrPasswordAuthenticationUnavailable
	}
	out.Verifier = domain.PasswordVerifier{
		Version: uint16(version), MemoryKiB: uint32(memory),
		Iterations: uint32(iterations), Parallelism: uint8(parallelism),
		Salt: out.Verifier.Salt, Digest: out.Verifier.Digest,
	}
	if out.SubjectID == "" || out.SessionEpoch <= 0 {
		return usecase.AuthenticationCredential{}, false, usecase.ErrPasswordAuthenticationUnavailable
	}
	return out, true, nil
}
