package postgres

import (
	"context"
	"errors"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Store struct {
	db Transactor
}

func NewStore(db Transactor) (*Store, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &Store{db: db}, nil
}

// CreatePendingWithObligation inserts subject, alias, KDF verifier and a
// durable identifier-only event in ONE transaction. No session or email
// verification challenge is issued. Every failed insert rolls everything back.
func (s *Store) CreatePendingWithObligation(ctx context.Context, r usecase.PendingRegistration) error {
	if s == nil || s.db == nil || r.Status != "UNVERIFIED" || r.CreatedAt.IsZero() ||
		r.SubjectID == "" || r.EventID == "" || r.CanonicalEmail == "" ||
		!isCanonicalEmail(r.CanonicalEmail) || !validVerifier(r.PasswordVerifier) {
		return ErrStorageUnavailable
	}
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO mamony_identity.subjects (subject_id, created_at)
			 VALUES ($1, $2)`, r.SubjectID, r.CreatedAt.UTC()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO mamony_identity.email_aliases
			 (canonical_email, subject_id, created_at)
			 VALUES ($1, $2, $3)`,
			r.CanonicalEmail, r.SubjectID, r.CreatedAt.UTC()); err != nil {
			return err
		}
		v := r.PasswordVerifier
		if _, err := tx.Exec(ctx,
			`INSERT INTO mamony_identity.password_verifiers
			 (subject_id, kdf_version, memory_kib, iterations, parallelism, salt, digest, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			r.SubjectID, v.Version, v.MemoryKiB, v.Iterations,
			v.Parallelism, v.Salt, v.Digest, r.CreatedAt.UTC()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO mamony_identity.security_outbox
			 (event_id, subject_id, occurred_at)
			 VALUES ($1, $2, $3)`,
			r.EventID, r.SubjectID, r.CreatedAt.UTC())
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		pgErr.ConstraintName == "email_aliases_pkey" {
		return usecase.ErrAliasAlreadyReserved
	}
	if err != nil {
		return ErrStorageUnavailable
	}
	return nil
}
