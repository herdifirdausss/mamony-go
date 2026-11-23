package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// EnrollmentBacklogSnapshot intentionally contains only capped aggregate
// observations. Counts are NOT total population counts. EligibleSampled is
// capped at 26; StalePresent indicates at least one >24h unissued subject.
// Neither value includes subject/contact/token information.
type EnrollmentBacklogSnapshot struct {
	EligibleSampled int
	StalePresent    bool
}

// ReadEnrollmentBacklog uses the existing read-only verifier privileges.
// The same eligibility predicate as the reconciliation candidate scan is
// evaluated against a single PostgreSQL clock sample. Query output cannot
// be used to select a subject or to approve new verification issuance.
func (s *ProofStore) ReadEnrollmentBacklog(ctx context.Context) (EnrollmentBacklogSnapshot, error) {
	var snapshot EnrollmentBacklogSnapshot
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil {
		return snapshot, ErrStorageUnavailable
	}
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `WITH stamp AS MATERIALIZED (
				SELECT clock_timestamp() AS moment
			), eligible AS (
				SELECT s.subject_id
				FROM mamony_identity.subjects s
				JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
				CROSS JOIN stamp
				WHERE s.status='UNVERIFIED' AND a.verified_at IS NULL
				  AND a.contact_version=1
				  AND s.created_at <= stamp.moment - interval '2 minutes'
				  AND s.created_at > stamp.moment - interval '24 hours'
				  AND NOT EXISTS (
					SELECT 1 FROM mamony_identity.email_verification_challenges c
					WHERE c.subject_id=s.subject_id
				  )
				LIMIT 26
			), stale AS (
				SELECT s.subject_id
				FROM mamony_identity.subjects s
				JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
				CROSS JOIN stamp
				WHERE s.status='UNVERIFIED' AND a.verified_at IS NULL
				  AND a.contact_version=1
				  AND s.created_at <= stamp.moment - interval '24 hours'
				  AND NOT EXISTS (
					SELECT 1 FROM mamony_identity.email_verification_challenges c
					WHERE c.subject_id=s.subject_id
				  )
				LIMIT 1
			)
			SELECT (SELECT count(*)::int FROM eligible),
			       EXISTS (SELECT 1 FROM stale)`).
			Scan(&snapshot.EligibleSampled, &snapshot.StalePresent)
	})
	if err != nil || snapshot.EligibleSampled < 0 || snapshot.EligibleSampled > 26 {
		return EnrollmentBacklogSnapshot{}, ErrStorageUnavailable
	}
	return snapshot, nil
}
