package usecase

import (
	"context"
	"errors"
	"testing"
)

type repairFinderStub struct {
	subjects []string
	err      error
	limit    int
}

func (s *repairFinderStub) FindUnissuedPending(_ context.Context, limit int) ([]string, error) {
	s.limit = limit
	return s.subjects, s.err
}

type repairIssuerStub struct {
	calls []string
	errs  map[string]error
}

func (s *repairIssuerStub) FirstIfMissing(_ context.Context, subject string) error {
	s.calls = append(s.calls, subject)
	return s.errs[subject]
}

func TestEnrollmentReconciliationBoundsAndFailures(t *testing.T) {
	a := "sub_AAAAAAAAAAAAAAAAAAAAAA"
	b := "sub_BBBBBBBBBBBBBBBBBBBBBB"
	finder := &repairFinderStub{subjects: []string{a, b}}
	issuer := &repairIssuerStub{errs: map[string]error{b: ErrEmailProofThrottled}}
	reconciler, err := NewEnrollmentReconciler(finder, issuer)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reconciler.ReconcileOnce(context.Background())
	if err != nil || got.Candidates != 2 || got.Issued != 1 || got.Skipped != 1 ||
		finder.limit != EnrollmentRepairBatchLimit || len(issuer.calls) != 2 {
		t.Fatalf("bounded repair contract failed: result=%+v err=%v", got, err)
	}
	finder.subjects = nil
	issuer.calls = nil
	if got, err := reconciler.ReconcileOnce(context.Background()); err != nil ||
		got.Candidates != 0 || len(issuer.calls) != 0 {
		t.Fatal("empty scan must not issue anything")
	}
	finder.subjects = []string{"client-controlled"}
	if _, err := reconciler.ReconcileOnce(context.Background()); !errors.Is(err, ErrEnrollmentUnavailable) ||
		len(issuer.calls) != 0 {
		t.Fatal("invalid subject escaped to issuer")
	}
	finder.subjects = []string{a}
	finder.err = errors.New("secret database error")
	if _, err := reconciler.ReconcileOnce(context.Background()); !errors.Is(err, ErrEnrollmentUnavailable) ||
		err.Error() != ErrEnrollmentUnavailable.Error() {
		t.Fatal("raw finder failure leaked")
	}
	finder.err = nil
	issuer.errs[a] = errors.New("sensitive provider/DB text")
	if _, err := reconciler.ReconcileOnce(context.Background()); !errors.Is(err, ErrEnrollmentUnavailable) ||
		err.Error() != ErrEnrollmentUnavailable.Error() {
		t.Fatal("raw issuance failure leaked")
	}
	finder.subjects = make([]string, EnrollmentRepairBatchLimit+1)
	if _, err := reconciler.ReconcileOnce(context.Background()); !errors.Is(err, ErrEnrollmentUnavailable) {
		t.Fatal("finder exceeded fixed batch limit")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reconciler.ReconcileOnce(ctx); !errors.Is(err, ErrEnrollmentUnavailable) {
		t.Fatal("canceled reconciliation unexpectedly continued")
	}
	if _, err := NewEnrollmentReconciler(nil, issuer); !errors.Is(err, ErrEnrollmentUnavailable) {
		t.Fatal("nil finder accepted")
	}
	if _, err := NewEnrollmentReconciler(finder, nil); !errors.Is(err, ErrEnrollmentUnavailable) {
		t.Fatal("nil issuer accepted")
	}
}
