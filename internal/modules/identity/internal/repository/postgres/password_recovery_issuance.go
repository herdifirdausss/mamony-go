package postgres

import (
	"context"
	"regexp"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

var recoveryEnvelopeKeyID = regexp.MustCompile("^[A-Za-z0-9_-]{1,32}$")
var recoveryChallengeID = regexp.MustCompile("^rcv_[A-Za-z0-9_-]{22}$")

// RecoveryIssuerStore is bound to the independently provisioned
// mamony_identity_recovery_issuer_runtime role. It has no table SELECT, no
// verifier read/write, no session/grant mint, and cannot claim deliveries.
// The only credential authority is EXECUTE on the v19 issue function.
type RecoveryIssuerStore struct{ db Transactor }

func NewRecoveryIssuerStore(db Transactor) (*RecoveryIssuerStore, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	return &RecoveryIssuerStore{db: db}, nil
}

// Issue produces only an internal eligible/ineligible bit. The SQL owner
// rechecks verified contact and current subject epoch under row locks and
// atomically stores a digest + sealed delivery. A public handler must never
// expose this bit or distinguish absent / suppressed / throttled accounts.
func (s *RecoveryIssuerStore) Issue(ctx context.Context, req usecase.RecoveryIssue) (bool, error) {
	if s == nil || s.db == nil || ctx == nil || ctx.Err() != nil ||
		!isCanonicalEmail(req.CanonicalEmail) ||
		!recoveryChallengeID.MatchString(req.ChallengeID) ||
		req.TokenDigest == ([32]byte{}) ||
		!recoveryEnvelopeKeyID.MatchString(req.KeyID) ||
		len(req.SealedToken) < 60 || len(req.SealedToken) > 256 {
		return false, ErrStorageUnavailable
	}
	var issued bool
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT mamony_identity.issue_password_recovery($1,$2,$3,$4,$5)`,
			req.CanonicalEmail, req.ChallengeID, req.TokenDigest[:],
			req.KeyID, req.SealedToken).Scan(&issued)
	})
	if err != nil {
		// Includes ambiguous commits. Never leak SQL errors to an endpoint or
		// auto-retry a change that may have committed and revoked old proofs.
		return false, ErrStorageUnavailable
	}
	return issued, nil
}
