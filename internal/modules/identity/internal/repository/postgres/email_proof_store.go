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

// FindUnverifiedSubject accepts a canonical email, and returns a server-owned
// subject only for an eligible unverified alias. No HTTP caller may supply a
// subject ID. Missing/active/verified aliases return ("", nil); SQL errors
// are redacted. The verifier role has SELECT-only authority for this lookup.
func (s *ProofStore) FindUnverifiedSubject(ctx context.Context, email string) (string, error) {
	if s == nil || s.db == nil || !isCanonicalEmail(email) {
		return "", ErrStorageUnavailable
	}
	var subject string
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT s.subject_id
			FROM mamony_identity.email_aliases a
			JOIN mamony_identity.subjects s ON s.subject_id=a.subject_id
			WHERE a.canonical_email=$1 AND a.verified_at IS NULL
			  AND a.contact_version=1 AND s.status='UNVERIFIED'`, email).
			Scan(&subject)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", ErrStorageUnavailable
	}
	return subject, nil
}

// Issue checks the authoritative current contact, revokes earlier live
// challenges, inserts the digest and commits a security event atomically.
// It does not send email or persist the plaintext verification bearer.
func (s *ProofStore) Issue(ctx context.Context, r usecase.EmailProofRequest) (usecase.EmailProofDestination, error) {
	var dest usecase.EmailProofDestination
	if s == nil || s.db == nil || len(r.SubjectID) != 26 || len(r.ChallengeID) != 26 ||
		len(r.EventID) != 26 || len(r.TokenDigest) != 32 ||
		len(r.EnvelopeKeyID) < 1 || len(r.EnvelopeKeyID) > 32 ||
		len(r.SealedToken) < 60 || len(r.SealedToken) > 256 {
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
		// Reconciliation must never rotate an existing proof, even when a
		// concurrent issuer committed after its advisory candidate scan.
		// This check shares the subject/alias lock with ordinary issuance.
		if r.OnlyIfFirst {
			var hadProof bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (
				SELECT 1 FROM mamony_identity.email_verification_challenges
				WHERE subject_id=$1 AND purpose='EMAIL_VERIFY'
			)`, r.SubjectID).Scan(&hadProof); err != nil {
				return err
			}
			if hadProof {
				return usecase.ErrEmailProofThrottled
			}
		}
		// The subject and alias row locks above serialize concurrent reissues
		// BEFORE the budget read. Count all historical attempts, including
		// revoked/expired challenges, using a single DB clock sample. A denied
		// request must never revoke an existing valid proof or enqueue a mail.
		var duringCooldown, inHour, inDay int
		err = tx.QueryRow(ctx, `WITH stamp AS MATERIALIZED (SELECT clock_timestamp() AS moment)
			SELECT
			  COUNT(c.challenge_id) FILTER (
			    WHERE c.requested_at > stamp.moment - make_interval(secs => $2::int)),
			  COUNT(c.challenge_id) FILTER (
			    WHERE c.requested_at > stamp.moment - interval '1 hour'),
			  COUNT(c.challenge_id)
			FROM stamp
			LEFT JOIN mamony_identity.email_verification_challenges c
			  ON c.subject_id=$1 AND c.purpose='EMAIL_VERIFY'
			 AND c.requested_at > stamp.moment - interval '24 hours'
			GROUP BY stamp.moment`, r.SubjectID, usecase.EmailProofCooldownSeconds).
			Scan(&duringCooldown, &inHour, &inDay)
		if err != nil {
			return err
		}
		if duringCooldown > 0 || inHour >= usecase.EmailProofHourlyLimit ||
			inDay >= usecase.EmailProofDailyLimit {
			return usecase.ErrEmailProofThrottled
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
		if _, err := tx.Exec(ctx, `INSERT INTO mamony_identity.email_proof_deliveries
			(challenge_id, key_id, sealed_token) VALUES ($1, $2, $3)`,
			r.ChallengeID, r.EnvelopeKeyID, r.SealedToken); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO mamony_identity.security_outbox
			(event_id, subject_id, event_type, occurred_at)
			VALUES ($1, $2, 'EMAIL_PROOF_ISSUED', clock_timestamp())`,
			r.EventID, r.SubjectID)
		return err
	})
	if errors.Is(err, usecase.ErrEmailProofThrottled) {
		return usecase.EmailProofDestination{}, usecase.ErrEmailProofThrottled
	}
	if err != nil {
		return usecase.EmailProofDestination{}, ErrStorageUnavailable
	}
	return dest, nil
}

// Consume serializes activation with issuance by locking the subject and
// current alias BEFORE consuming a challenge, so a concurrent resend cannot
// make a previously revoked token valid. All state changes and mandatory
// outbox evidence commit or roll back together.
//
// ErrEmailProofInvalid is returned ONLY for an authoritative negative proof
// decision: nonexistent token/subject, wrong contact or status, expired,
// revoked or already-consumed challenge. All SQL/commit/transport failures
// return the distinct, sanitized ErrEmailProofUnavailable. No raw pgx error,
// token digest, subject ID or email is returned to the HTTP boundary.
func (s *ProofStore) Consume(ctx context.Context, tokenDigest []byte, eventID string) error {
	if len(tokenDigest) != 32 || len(eventID) != 26 {
		return usecase.ErrEmailProofInvalid
	}
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil {
		return usecase.ErrEmailProofUnavailable
	}
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		var subject, email string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT subject_id, canonical_email, contact_version
			FROM mamony_identity.email_verification_challenges
			WHERE token_digest = $1 AND purpose = 'EMAIL_VERIFY'`, tokenDigest).
			Scan(&subject, &email, &version); err != nil {
			return confirmationScanError(err)
		}
		var currentEmail, status string
		var currentVersion int64
		var verifiedAt *time.Time
		if err := tx.QueryRow(ctx, `SELECT s.status, a.canonical_email, a.contact_version, a.verified_at
			FROM mamony_identity.subjects s
			JOIN mamony_identity.email_aliases a ON a.subject_id = s.subject_id
			WHERE s.subject_id = $1 FOR UPDATE OF s, a`, subject).
			Scan(&status, &currentEmail, &currentVersion, &verifiedAt); err != nil {
			return confirmationScanError(err)
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
			return confirmationScanError(err)
		}
		tag, err := tx.Exec(ctx, `UPDATE mamony_identity.email_aliases
			SET verified_at = clock_timestamp()
			WHERE subject_id = $1 AND canonical_email = $2 AND contact_version = $3
			  AND verified_at IS NULL`, subject, email, version)
		if err != nil {
			return usecase.ErrEmailProofUnavailable
		}
		if tag.RowsAffected() != 1 {
			// Rows missing after the subject and alias were locked is an
			// unexpected state/authority inconsistency, not a bad bearer.
			return usecase.ErrEmailProofUnavailable
		}
		tag, err = tx.Exec(ctx, `UPDATE mamony_identity.subjects SET status = 'ACTIVE'
			WHERE subject_id = $1 AND status = 'UNVERIFIED'`, subject)
		if err != nil {
			return usecase.ErrEmailProofUnavailable
		}
		if tag.RowsAffected() != 1 {
			// Rows missing after the subject and alias were locked is an
			// unexpected state/authority inconsistency, not a bad bearer.
			return usecase.ErrEmailProofUnavailable
		}
		if _, err := tx.Exec(ctx, `INSERT INTO mamony_identity.security_outbox
			(event_id, subject_id, event_type, occurred_at)
			VALUES ($1, $2, 'CONTACT_VERIFIED', clock_timestamp())`, eventID, subject); err != nil {
			return usecase.ErrEmailProofUnavailable
		}
		return nil
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, usecase.ErrEmailProofInvalid) {
		return usecase.ErrEmailProofInvalid
	}
	// Includes failed BEGIN/COMMIT, aborted transactions, canceled requests
	// and unexpected driver faults. Never report those as a bad token.
	return usecase.ErrEmailProofUnavailable
}

// pgx.ErrNoRows is an authoritative negative proof result at the three
// SELECT/UPDATE RETURNING steps; other driver failures are infrastructure
// errors. Keep this decision private to the owning PostgreSQL adapter.
func confirmationScanError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return usecase.ErrEmailProofInvalid
	}
	return usecase.ErrEmailProofUnavailable
}
