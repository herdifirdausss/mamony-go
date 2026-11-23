package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

type ProofStore struct {
	db Transactor
}

func NewProofStore(db Transactor) (*ProofStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &ProofStore{db: db}, nil
}

// Issue checks the authoritative current contact, revokes earlier live
// challenges, inserts the digest and commits a security event atomically.
// It does not send email or persist the plaintext verification bearer.
func (s *ProofStore) Issue(ctx context.Context, r usecase.EmailProofRequest) (usecase.EmailProofDestination, error) {
	var dest usecase.EmailProofDestination
	if s == nil || s.db == nil || len(r.SubjectID) != 26 || len(r.ChallengeID) != 26 ||
		len(r.EventID) != 26 || len(r.TokenDigest) != 32 {
		return dest, ErrStorageUnavailable
	}
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		var status string
		var verifiedAt *time.Time
		err := tx.QueryRow(ctx, `SELECT s.status, a.canonical_email, a.contact_version, a.verified_at
			FROM mamony_identity.subjects s
			JOIN mamony_identity.email_aliases a ON a.subject_id = s.subject_id
			WHERE s.subject_id = $1 FOR UPDATE OF s, a`, r.SubjectID).
			Scan(&status, &dest.CanonicalEmail, &dest.ContactVersion, &verifiedAt)
		if err != nil {
			return err
		}
		if status != "UNVERIFIED" || verifiedAt != nil || dest.ContactVersion != 1 {
			return usecase.ErrEmailProofIneligible
		}
		if _, err := tx.Exec(ctx, `UPDATE mamony_identity.email_verification_challenges
			SET revoked_at = clock_timestamp()
			WHERE subject_id = $1 AND purpose = 'EMAIL_VERIFY'
			  AND consumed_at IS NULL AND revoked_at IS NULL`, r.SubjectID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO mamony_identity.email_verification_challenges
			(challenge_id, subject_id, canonical_email, contact_version, purpose,
			 token_digest, requested_at, expires_at)
			VALUES ($1, $2, $3, $4, 'EMAIL_VERIFY', $5,
			 clock_timestamp(), clock_timestamp() + interval '20 minutes')`,
			r.ChallengeID, r.SubjectID, dest.CanonicalEmail, dest.ContactVersion, r.TokenDigest); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO mamony_identity.security_outbox
			(event_id, subject_id, event_type, occurred_at)
			VALUES ($1, $2, 'EMAIL_PROOF_ISSUED', clock_timestamp())`,
			r.EventID, r.SubjectID)
		return err
	})
	if err != nil {
		return usecase.EmailProofDestination{}, ErrStorageUnavailable
	}
	return dest, nil
}

// Consume serializes activation with issuance by locking the subject and
// current alias BEFORE consuming a challenge, so a concurrent resend cannot
// make a previously revoked token valid. All state changes and mandatory
// outbox evidence commit or roll back together.
func (s *ProofStore) Consume(ctx context.Context, tokenDigest []byte, eventID string) error {
	if s == nil || s.db == nil || len(tokenDigest) != 32 || len(eventID) != 26 {
		return usecase.ErrEmailProofInvalid
	}
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		var subject, email string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT subject_id, canonical_email, contact_version
			FROM mamony_identity.email_verification_challenges
			WHERE token_digest = $1 AND purpose = 'EMAIL_VERIFY'`, tokenDigest).
			Scan(&subject, &email, &version); err != nil {
			return usecase.ErrEmailProofInvalid
		}
		var currentEmail, status string
		var currentVersion int64
		var verifiedAt *time.Time
		if err := tx.QueryRow(ctx, `SELECT s.status, a.canonical_email, a.contact_version, a.verified_at
			FROM mamony_identity.subjects s
			JOIN mamony_identity.email_aliases a ON a.subject_id = s.subject_id
			WHERE s.subject_id = $1 FOR UPDATE OF s, a`, subject).
			Scan(&status, &currentEmail, &currentVersion, &verifiedAt); err != nil {
			return usecase.ErrEmailProofInvalid
		}
		if status != "UNVERIFIED" || verifiedAt != nil ||
			currentEmail != email || currentVersion != version {
			return usecase.ErrEmailProofInvalid
		}
		var challengeID string
		if err := tx.QueryRow(ctx, `UPDATE mamony_identity.email_verification_challenges
			SET consumed_at = clock_timestamp()
			WHERE token_digest = $1 AND purpose = 'EMAIL_VERIFY'
			  AND subject_id = $2 AND canonical_email = $3 AND contact_version = $4
			  AND consumed_at IS NULL AND revoked_at IS NULL
			  AND expires_at > clock_timestamp()
			RETURNING challenge_id`, tokenDigest, subject, email, version).
			Scan(&challengeID); err != nil {
			return usecase.ErrEmailProofInvalid
		}
		tag, err := tx.Exec(ctx, `UPDATE mamony_identity.email_aliases
			SET verified_at = clock_timestamp()
			WHERE subject_id = $1 AND canonical_email = $2 AND contact_version = $3
			  AND verified_at IS NULL`, subject, email, version)
		if err != nil || tag.RowsAffected() != 1 {
			return usecase.ErrEmailProofInvalid
		}
		tag, err = tx.Exec(ctx, `UPDATE mamony_identity.subjects SET status = 'ACTIVE'
			WHERE subject_id = $1 AND status = 'UNVERIFIED'`, subject)
		if err != nil || tag.RowsAffected() != 1 {
			return usecase.ErrEmailProofInvalid
		}
		_, err = tx.Exec(ctx, `INSERT INTO mamony_identity.security_outbox
			(event_id, subject_id, event_type, occurred_at)
			VALUES ($1, $2, 'CONTACT_VERIFIED', clock_timestamp())`, eventID, subject)
		return err
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return usecase.ErrEmailProofUnavailable
		}
		return usecase.ErrEmailProofInvalid
	}
	return nil
}
