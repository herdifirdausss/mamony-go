package postgres

import (
	"context"
	"errors"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// DeliveryStore uses ONLY a separately credentialed Identity delivery pool.
// It may claim encrypted envelopes; it cannot mutate subject/contact state.
type DeliveryStore struct {
	db Transactor
}

func NewDeliveryStore(db Transactor) (*DeliveryStore, error) {
	if db==nil{return nil,ErrStorageUnavailable}
	return &DeliveryStore{db:db},nil
}

// Claim uses SKIP LOCKED to allow multiple workers without sending the same
// challenge simultaneously. Leases recover after crashes, but cannot
// promise exactly-once external delivery.
func (s *DeliveryStore) Claim(ctx context.Context, leaseID string) (usecase.ProofDeliveryJob,bool,error){
	var job usecase.ProofDeliveryJob
	if s==nil || s.db==nil || len(leaseID)!=26 || leaseID[:4]!="lse_"{
		return job,false,ErrStorageUnavailable
	}
	err:=s.db.WithTx(ctx,func(tx pgx.Tx)error{
		return tx.QueryRow(ctx,`WITH candidate AS (
			SELECT d.challenge_id FROM mamony_identity.email_proof_deliveries d
			JOIN mamony_identity.email_verification_challenges c USING (challenge_id)
			JOIN mamony_identity.subjects s ON s.subject_id = c.subject_id
			JOIN mamony_identity.email_aliases a ON a.subject_id = c.subject_id
			WHERE d.status='PENDING' AND d.attempts < 5
			  AND d.next_attempt_at <= clock_timestamp()
			  AND (d.leased_until IS NULL OR d.leased_until < clock_timestamp())
			  AND c.purpose='EMAIL_VERIFY'
			  AND c.consumed_at IS NULL AND c.revoked_at IS NULL
			  AND c.expires_at > clock_timestamp() + interval '15 seconds'
			  AND s.status='UNVERIFIED' AND a.verified_at IS NULL
			  AND a.canonical_email=c.canonical_email
			  AND a.contact_version=c.contact_version
			ORDER BY d.next_attempt_at,d.challenge_id
			FOR UPDATE OF d SKIP LOCKED LIMIT 1
		)
		UPDATE mamony_identity.email_proof_deliveries d
		SET lease_id=$1, leased_until=clock_timestamp()+interval '45 seconds',
		    updated_at=clock_timestamp()
		FROM candidate
		JOIN mamony_identity.email_verification_challenges c USING(challenge_id)
		JOIN mamony_identity.email_aliases a ON a.subject_id=c.subject_id
		WHERE d.challenge_id=candidate.challenge_id
		RETURNING d.challenge_id,c.subject_id,a.canonical_email,
		    d.key_id,d.sealed_token,c.expires_at`,leaseID).
			Scan(&job.ChallengeID,&job.SubjectID,&job.Recipient,
				&job.KeyID,&job.SealedToken,&job.ExpiresAt)
	})
	if errors.Is(err,pgx.ErrNoRows){return usecase.ProofDeliveryJob{},false,nil}
	if err!=nil{return usecase.ProofDeliveryJob{},false,ErrStorageUnavailable}
	job.LeaseID=leaseID
	return job,true,nil
}

// Finalize uses the claim fencing token and lease deadline; a restarted
// worker cannot acknowledge a lease owned by a newer worker. Temporary
// failures retry after 1,2,4,8 minutes; fifth attempt is terminal.
func (s *DeliveryStore) Finalize(ctx context.Context,challengeID,leaseID string,outcome usecase.ProofDeliveryOutcome)error{
	if s==nil || s.db==nil || len(challengeID)!=26 || len(leaseID)!=26 ||
		(outcome!=usecase.DeliveryAccepted && outcome!=usecase.DeliveryTemporary && outcome!=usecase.DeliveryPermanent){
		return ErrStorageUnavailable
	}
	return s.db.WithTx(ctx,func(tx pgx.Tx)error{
		tag,err:=tx.Exec(ctx,`UPDATE mamony_identity.email_proof_deliveries d
			SET status=CASE
			    WHEN $3='ACCEPTED' THEN 'ACCEPTED'
			    WHEN $3='PERMANENT' OR d.attempts >= 4 THEN 'DEAD'
			    ELSE 'PENDING' END,
			    attempts=d.attempts+1,
			    next_attempt_at=CASE WHEN $3='TEMPORARY' THEN
			        clock_timestamp() + CASE d.attempts
			           WHEN 0 THEN interval '1 minute'
			           WHEN 1 THEN interval '2 minutes'
			           WHEN 2 THEN interval '4 minutes'
			           ELSE interval '8 minutes' END
			        ELSE d.next_attempt_at END,
			    accepted_at=CASE WHEN $3='ACCEPTED' THEN clock_timestamp() ELSE NULL END,
			    lease_id=NULL, leased_until=NULL,
			    updated_at=clock_timestamp()
			WHERE d.challenge_id=$1 AND d.lease_id=$2
			  AND d.leased_until > clock_timestamp()
			  AND d.status='PENDING' AND d.attempts<5`,
			challengeID,leaseID,string(outcome))
		if err!=nil || tag.RowsAffected()!=1{return ErrStorageUnavailable}
		return nil
	})
}
