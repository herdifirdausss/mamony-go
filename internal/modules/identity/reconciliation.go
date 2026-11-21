package identity

import (
	"context"
	"errors"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailcrypto"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
)

var ErrEnrollmentReconciliation = errors.New("identity enrollment reconciliation unavailable")

// EnrollmentReconciliationConfig is trusted operator configuration. Never
// accept these settings from an HTTP request or load from a business DB.
// The verifier runtime must not have access to password verifier rows.
type EnrollmentReconciliationConfig struct {
	DSN          string
	ActiveKeyID  string
	EnvelopeKeys map[string][]byte
}

// EnrollmentReconciliationResult contains counters only. No subject ID,
// contact address, proof token, ciphertext or provider response can escape.
type EnrollmentReconciliationResult struct {
	Candidates int
	Issued     int
	Skipped    int
}

// ReconcileEnrollmentOnce is deliberately NOT mounted, scheduled, or called
// from public HTTP. An authorized operator/supervisor must supply a bounded
// invocation and secure secret distribution. No mail is directly sent here;
// proof issuance commits encrypted delivery obligations for the separate
// disabled-by-default Mailgun worker.
func ReconcileEnrollmentOnce(ctx context.Context, cfg EnrollmentReconciliationConfig) (EnrollmentReconciliationResult, error) {
	var empty EnrollmentReconciliationResult
	if ctx == nil || ctx.Err() != nil {
		return empty, ErrEnrollmentReconciliation
	}
	keys, err := mailcrypto.NewKeyring(cfg.ActiveKeyID, cfg.EnvelopeKeys)
	if err != nil {
		return empty, ErrEnrollmentReconciliation
	}
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: cfg.DSN, RuntimeRole: "mamony_identity_verifier_runtime", MaxConns: 2,
	})
	if err != nil {
		return empty, ErrEnrollmentReconciliation
	}
	defer pool.Close()
	proofs, err := identitypg.NewProofStore(pool)
	if err != nil {
		return empty, ErrEnrollmentReconciliation
	}
	// Explicit owner-defined ports preserve the repository-to-usecase boundary.
	var proofPort usecase.EmailProofPort = proofs
	var sealPort usecase.ProofEnvelopeSealer = keys
	issuer, err := usecase.NewEmailVerification(proofPort, sealPort)
	if err != nil {
		return empty, ErrEnrollmentReconciliation
	}
	var finderPort usecase.UnissuedEnrollmentFinder = proofs
	var issuerPort usecase.EnrollmentProofIssuer = issuer
	reconciler, err := usecase.NewEnrollmentReconciler(finderPort, issuerPort)
	if err != nil {
		return empty, ErrEnrollmentReconciliation
	}
	got, err := reconciler.ReconcileOnce(ctx)
	if err != nil {
		return EnrollmentReconciliationResult{
			Candidates: got.Candidates, Issued: got.Issued, Skipped: got.Skipped,
		}, ErrEnrollmentReconciliation
	}
	return EnrollmentReconciliationResult{
		Candidates: got.Candidates, Issued: got.Issued, Skipped: got.Skipped,
	}, nil
}
