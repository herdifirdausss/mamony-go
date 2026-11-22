package postgres

import (
	"context"
	"errors"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// OpaqueSessionStore MUST use the separately provisioned session runtime.
// That role has EXECUTE on grant-consume/resolve/revoke functions and no
// direct table SELECT, INSERT, UPDATE or DELETE privileges.
type OpaqueSessionStore struct {
	db Transactor
}

func NewOpaqueSessionStore(db Transactor) (*OpaqueSessionStore, error) {
	if db == nil {
		return nil, usecase.ErrSessionUnavailable
	}
	return &OpaqueSessionStore{db: db}, nil
}

func (s *OpaqueSessionStore) Issue(ctx context.Context, grantDigest, digest []byte, event string) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		len(digest) != 32 || len(grantDigest) != 32 || len(event) != 26 {
		return false, usecase.ErrSessionUnavailable
	}
	var allowed bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT mamony_identity.issue_opaque_session_with_grant($1,$2,$3)",
			grantDigest, digest, event).Scan(&allowed)
	})
	if err != nil {
		// Includes unknown outcome after COMMIT: no bearer can be released.
		return false, usecase.ErrSessionUnavailable
	}
	return allowed, nil
}

func (s *OpaqueSessionStore) Resolve(ctx context.Context, digest []byte) (string, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil || len(digest) != 32 {
		return "", usecase.ErrSessionUnavailable
	}
	var subject string
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		// COALESCE hides unknown/expired/revoked/suspended session detail.
		return tx.QueryRow(ctx,
			"SELECT COALESCE(mamony_identity.resolve_opaque_session($1),'')",
			digest).Scan(&subject)
	})
	if err != nil {
		return "", usecase.ErrSessionUnavailable
	}
	return subject, nil
}

func (s *OpaqueSessionStore) Revoke(ctx context.Context, digest []byte, event string) error {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		len(digest) != 32 || len(event) != 26 {
		return usecase.ErrSessionUnavailable
	}
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		var ok bool
		if err := tx.QueryRow(ctx, "SELECT mamony_identity.revoke_opaque_session($1,$2)",
			digest, event).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return errors.New("invalid internal session revoke input")
		}
		return nil
	})
	if err != nil {
		return usecase.ErrSessionUnavailable
	}
	return nil
}
