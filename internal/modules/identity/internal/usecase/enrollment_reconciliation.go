package usecase

import (
	"context"
	"errors"
	"strings"
)

// EnrollmentRepairBatchLimit bounds one explicitly authorized reconciliation
// invocation. It is not a provider-wide sending quota or an admission budget.
const EnrollmentRepairBatchLimit = 25

// UnissuedEnrollmentFinder reads only old, UNVERIFIED registrations that
// have never had an email proof. It must use the Identity verifier authority,
// not business projections or untrusted client-supplied identifiers.
type UnissuedEnrollmentFinder interface {
	FindUnissuedPending(context.Context, int) ([]string, error)
}

type EnrollmentReconcileResult struct {
	Candidates int
	Issued     int
	Skipped    int
}

// EnrollmentReconciler repairs the crash window between a committed pending
// registration and its first proof. It never rotates an existing proof,
// activates a subject, or exposes a bearer or email address to its caller.
type EnrollmentReconciler struct {
	finder UnissuedEnrollmentFinder
	issuer EnrollmentProofIssuer
}

func NewEnrollmentReconciler(finder UnissuedEnrollmentFinder, issuer EnrollmentProofIssuer) (*EnrollmentReconciler, error) {
	if finder == nil || issuer == nil {
		return nil, ErrEnrollmentUnavailable
	}
	return &EnrollmentReconciler{finder: finder, issuer: issuer}, nil
}

// ReconcileOnce performs at most one bounded scan. Discovery is advisory:
// concurrent registration/resend/repair may win the race, but the proof
// repository still serializes issuance under subject/contact locks and applies
// the regular cooldown and rolling hourly/daily budgets. A timeout is not
// evidence that the first issuer failed to commit.
func (r *EnrollmentReconciler) ReconcileOnce(ctx context.Context) (EnrollmentReconcileResult, error) {
	var result EnrollmentReconcileResult
	if ctx == nil || ctx.Err() != nil || r == nil || r.finder == nil || r.issuer == nil {
		return result, ErrEnrollmentUnavailable
	}
	subjects, err := r.finder.FindUnissuedPending(ctx, EnrollmentRepairBatchLimit)
	if err != nil || len(subjects) > EnrollmentRepairBatchLimit {
		return result, ErrEnrollmentUnavailable
	}
	result.Candidates = len(subjects)
	for _, subject := range subjects {
		if ctx.Err() != nil || len(subject) != 26 || !strings.HasPrefix(subject, "sub_") {
			return result, ErrEnrollmentUnavailable
		}
		switch err := r.issuer.Resend(ctx, subject); {
		case err == nil:
			result.Issued++
		case errors.Is(err, ErrEmailProofThrottled):
			// Another issuer won; it has already created a proof under
			// the same rate limits. Do not disclose or retry it here.
			result.Skipped++
		default:
			// Do not retry in a hot loop or expose a subject/database error.
			// A later independent invocation will rescan remaining gaps.
			return result, ErrEnrollmentUnavailable
		}
	}
	return result, nil
}
