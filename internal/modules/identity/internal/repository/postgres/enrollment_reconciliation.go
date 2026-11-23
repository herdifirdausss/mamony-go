package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// FindUnissuedPending uses only SELECT rights already granted to the verifier
// runtime. The minimum age avoids racing the normal first-proof issuance;
// the maximum age forbids surprising delivery to stale, unverified accounts.
// A subject with ANY historical challenge is intentionally excluded, even
// when its challenge is expired or revoked. That case belongs to admitted
// customer resend, not automated first-proof recovery.
func (s *ProofStore) FindUnissuedPending(ctx context.Context, limit int) ([]string, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		limit < 1 || limit > 25 {
		return nil, ErrStorageUnavailable
	}
	subjects := make([]string, 0, limit)
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT s.subject_id
			FROM mamony_identity.subjects s
			JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
			WHERE s.status='UNVERIFIED' AND a.verified_at IS NULL
			  AND a.contact_version=1
			  AND s.created_at <= clock_timestamp()-interval '2 minutes'
			  AND s.created_at > clock_timestamp()-interval '24 hours'
			  AND NOT EXISTS (
			    SELECT 1 FROM mamony_identity.email_verification_challenges c
			    WHERE c.subject_id=s.subject_id
			  )
			ORDER BY s.created_at, s.subject_id LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var subject string
			if err := rows.Scan(&subject); err != nil {
				return err
			}
			subjects = append(subjects, subject)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, ErrStorageUnavailable
	}
	return subjects, nil
}
