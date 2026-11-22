package identity

import (
	"context"
	"errors"

	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
)

// ErrNoticeHealthUnavailable contains no SQL query, credential or recipient.
var ErrNoticeHealthUnavailable = errors.New("identity security notice health unavailable")

// SecurityNoticeHealthSnapshot is an aggregate observation only. A low count
// does not guarantee inbox delivery or provider webhook completeness.
type SecurityNoticeHealthSnapshot struct {
	PendingDue       int64
	Dead             int64
	SubmittedUnknown int64
	Delivered        int64
	ProviderFailures int64
	UnmatchedEvents  int64
}

// ReadSecurityNoticeHealth is a one-shot count-only operator read using
// mamony_identity_maintenance_runtime. It does not schedule monitoring,
// publish a dashboard, send alerts or mount a public HTTP endpoint.
func ReadSecurityNoticeHealth(ctx context.Context, dsn string) (SecurityNoticeHealthSnapshot, error) {
	var empty SecurityNoticeHealthSnapshot
	if ctx == nil || ctx.Err() != nil {
		return empty, ErrNoticeHealthUnavailable
	}
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: dsn, RuntimeRole: "mamony_identity_maintenance_runtime", MaxConns: 1,
	})
	if err != nil {
		return empty, ErrNoticeHealthUnavailable
	}
	defer pool.Close()
	store, err := identitypg.NewSecurityNoticeHealthStore(pool)
	if err != nil {
		return empty, ErrNoticeHealthUnavailable
	}
	result, err := store.Read(ctx)
	if err != nil {
		return empty, ErrNoticeHealthUnavailable
	}
	return SecurityNoticeHealthSnapshot{
		PendingDue: result.PendingDue,
		Dead: result.Dead,
		SubmittedUnknown: result.SubmittedUnknown,
		Delivered: result.Delivered,
		ProviderFailures: result.ProviderFailures,
		UnmatchedEvents: result.UnmatchedEvents,
	}, nil
}
