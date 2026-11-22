package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrMaintenanceUnavailable = errors.New("identity retention unavailable")

// RetentionStore uses ONLY the limited maintenance role. PostgreSQL exposes
// two constant-parameter SECURITY DEFINER functions; this role cannot SELECT,
// UPDATE or DELETE any Identity table directly.
type RetentionStore struct {
	db Transactor
}

func NewRetentionStore(db Transactor) (*RetentionStore, error) {
	if db == nil {
		return nil, ErrMaintenanceUnavailable
	}
	return &RetentionStore{db: db}, nil
}

// PurgeOneBatch applies a max-100 envelope erasure plus max-100 old counter
// deletion in ONE transaction. A partial failure rolls back BOTH operations.
func (s *RetentionStore) PurgeOneBatch(ctx context.Context) (int, int, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil {
		return 0, 0, ErrMaintenanceUnavailable
	}
	var envelopes, counters int
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT mamony_identity.purge_expired_email_envelopes()").Scan(&envelopes); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "SELECT mamony_identity.purge_expired_anonymous_buckets()").Scan(&counters); err != nil {
			return err
		}
		if envelopes < 0 || envelopes > 100 || counters < 0 || counters > 100 {
			return ErrMaintenanceUnavailable
		}
		return nil
	})
	if err != nil {
		return 0, 0, ErrMaintenanceUnavailable
	}
	return envelopes, counters, nil
}


// PurgeOneBatchWithSessions is the v10 operator path: the original 100
// encrypted envelopes, 100 expired login buckets and 100 expired session
// digests are removed together, or all rolled back on any error. The SQL
// functions have independent grants: only the maintenance role can call them.
func (s *RetentionStore) PurgeOneBatchWithSessions(ctx context.Context) (int, int, int, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil {
		return 0, 0, 0, ErrMaintenanceUnavailable
	}
	var envelopes, counters, sessions int
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			"SELECT mamony_identity.purge_expired_email_envelopes()").Scan(&envelopes); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			"SELECT mamony_identity.purge_expired_anonymous_buckets()").Scan(&counters); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx,
			"SELECT mamony_identity.purge_expired_opaque_sessions()").Scan(&sessions); err != nil {
			return err
		}
		if envelopes < 0 || envelopes > 100 || counters < 0 || counters > 100 ||
			sessions < 0 || sessions > 100 {
			return ErrMaintenanceUnavailable
		}
		return nil
	})
	if err != nil {
		return 0, 0, 0, ErrMaintenanceUnavailable
	}
	return envelopes, counters, sessions, nil
}
