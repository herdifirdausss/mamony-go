package identity

import (
	"context"

	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
)

// EnrollmentBacklog reports only capped counts, not subject IDs, contact
// aliases, encrypted proofs, provider errors, or bearer tokens. A successful
// inspect is a point-in-time observation, not a delivery guarantee.
type EnrollmentBacklog struct {
	EligibleSampled int  // 0..26; 26 means at least 26 eligible
	StalePresent    bool // at least one older than 24h, never-issued subject
}

// InspectEnrollmentBacklog is a read-only one-shot operation. No HTTP route,
// background scheduler, email provider, or session is activated by this API.
// The deployment must provide a separately authorized verifier-role DSN.
func InspectEnrollmentBacklog(ctx context.Context, dsn string) (EnrollmentBacklog, error) {
	if ctx == nil || ctx.Err() != nil || dsn == "" {
		return EnrollmentBacklog{}, ErrEnrollmentReconciliation
	}
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: dsn, RuntimeRole: "mamony_identity_verifier_runtime", MaxConns: 1,
	})
	if err != nil {
		return EnrollmentBacklog{}, ErrEnrollmentReconciliation
	}
	defer pool.Close()
	store, err := identitypg.NewProofStore(pool)
	if err != nil {
		return EnrollmentBacklog{}, ErrEnrollmentReconciliation
	}
	snapshot, err := store.ReadEnrollmentBacklog(ctx)
	if err != nil {
		return EnrollmentBacklog{}, ErrEnrollmentReconciliation
	}
	return EnrollmentBacklog{
		EligibleSampled: snapshot.EligibleSampled, StalePresent: snapshot.StalePresent,
	}, nil
}
