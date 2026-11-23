package identity

import (
	"context"
	"errors"

	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
)

var ErrRecoveryAssuranceRuntime = errors.New("recovery assurance runtime unavailable")

// ApproveOfflineRecoveryCode is an explicitly invoked, trusted-only
// capability. It opens a distinct role, never a public HTTP route or a
// recovery-code enrollment ceremony. Production is fail-closed because no
// ordinary runtime is permitted to enroll saved offline codes.
func ApproveOfflineRecoveryCode(ctx context.Context, dsn, recoveryBearer, savedCode string) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrRecoveryAssuranceRuntime
	}
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: dsn, RuntimeRole: "mamony_identity_recovery_assurance_runtime", MaxConns: 1,
	})
	if err != nil {
		return ErrRecoveryAssuranceRuntime
	}
	defer pool.Close()
	store, err := identitypg.NewRecoveryAssuranceStore(pool)
	if err != nil {
		return ErrRecoveryAssuranceRuntime
	}
	var port usecase.RecoveryAssuranceStore = store
	verifier, err := usecase.NewRecoveryOfflineCodeAssurance(port)
	if err != nil {
		return ErrRecoveryAssuranceRuntime
	}
	if err := verifier.Approve(ctx, recoveryBearer, savedCode); err != nil {
		return ErrRecoveryAssuranceRuntime
	}
	return nil
}
