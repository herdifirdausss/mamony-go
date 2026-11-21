package identity

import (
	"context"
	"errors"

	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
)

var ErrRetentionRuntime = errors.New("identity retention runtime unavailable")

// RetentionConfig is a trusted operator input; never an HTTP request body.
type RetentionConfig struct {
	DSN string
}

// RetentionResult contains counts only, never identifiers, tokens or emails.
type RetentionResult struct {
	EnvelopesPurged int
	CountersPurged int
}

// PurgeRetention runs ONE bounded, atomic cleanup batch. The owner/operator
// must explicitly schedule future runs with an isolated maintenance identity.
// It does not launch a goroutine, deploy a scheduler, or authorize HTTP access.
func PurgeRetention(ctx context.Context, cfg RetentionConfig) (RetentionResult, error) {
	if ctx == nil || ctx.Err() != nil {
		return RetentionResult{}, ErrRetentionRuntime
	}
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: cfg.DSN, RuntimeRole: "mamony_identity_maintenance_runtime", MaxConns: 1,
	})
	if err != nil {
		return RetentionResult{}, ErrRetentionRuntime
	}
	defer pool.Close()
	store, err := identitypg.NewRetentionStore(pool)
	if err != nil {
		return RetentionResult{}, ErrRetentionRuntime
	}
	envelopes, counters, err := store.PurgeOneBatch(ctx)
	if err != nil {
		return RetentionResult{}, ErrRetentionRuntime
	}
	return RetentionResult{EnvelopesPurged: envelopes, CountersPurged: counters}, nil
}
