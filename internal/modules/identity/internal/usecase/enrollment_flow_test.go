package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

type enrollmentFakeReg struct {
	subject string
	err     error
	calls   int
}

func (r *enrollmentFakeReg) RegisterPendingWithSubject(context.Context, string, string) (string, error) {
	r.calls++
	return r.subject, r.err
}

type enrollmentFakeLookup struct {
	subject string
	err     error
	calls   int
	email   string
}

func (l *enrollmentFakeLookup) FindUnverifiedSubject(_ context.Context, email string) (string, error) {
	l.calls++
	l.email = email
	return l.subject, l.err
}

type enrollmentFakeProof struct {
	calls   int
	subject string
	err     error
}

func (p *enrollmentFakeProof) Resend(_ context.Context, subject string) error {
	p.calls++
	p.subject = subject
	return p.err
}

func TestEnrollmentFlowMasksExistenceAndBindsNewProof(t *testing.T) {
	reg := &enrollmentFakeReg{subject: "sub_AAAAAAAAAAAAAAAAAAAAAA"}
	lookup := &enrollmentFakeLookup{subject: "sub_BBBBBBBBBBBBBBBBBBBBBB"}
	proof := &enrollmentFakeProof{}
	flow, err := NewEnrollmentFlow(reg, lookup, proof)
	if err != nil {
		t.Fatal(err)
	}
	if err := flow.Register(context.Background(), "new@example.test", "password placeholder"); err != nil {
		t.Fatal(err)
	}
	if proof.calls != 1 || proof.subject != reg.subject {
		t.Fatal("new pending registration did not issue a proof for its own subject")
	}
	reg.subject = ""
	if err := flow.Register(context.Background(), "existing@example.test", "password placeholder"); err != nil || proof.calls != 1 {
		t.Fatal("existing alias rotated a proof from registration")
	}
	if err := flow.Resend(context.Background(), "KNOWN@EXAMPLE.TEST"); err != nil ||
		lookup.email != "known@example.test" || proof.subject != lookup.subject || proof.calls != 2 {
		t.Fatal("resend did not resolve subject from server")
	}
	lookup.subject = ""
	if err := flow.Resend(context.Background(), "missing@example.test"); err != nil || proof.calls != 2 {
		t.Fatal("unknown account caused proof issuance or distinguishable internal error")
	}
	lookup.subject = "sub_BBBBBBBBBBBBBBBBBBBBBB"
	proof.err = ErrEmailProofThrottled
	if err := flow.Resend(context.Background(), "known@example.test"); err != nil {
		t.Fatal("trusted subject-level throttle must be normalized")
	}
	proof.err = errors.New("sensitive SQL text")
	if err := flow.Resend(context.Background(), "known@example.test"); !errors.Is(err, ErrEnrollmentUnavailable) ||
		err.Error() != ErrEnrollmentUnavailable.Error() {
		t.Fatal("raw downstream failure escaped")
	}
	lookup.err = errors.New("database details")
	if err := flow.Resend(context.Background(), "known@example.test"); !errors.Is(err, ErrEnrollmentUnavailable) ||
		err.Error() != ErrEnrollmentUnavailable.Error() {
		t.Fatal("raw SQL error escaped subject lookup")
	}
	reg.err = domain.ErrInvalidPassword
	if err := flow.Register(context.Background(), "existing@example.test", "short"); !errors.Is(err, domain.ErrInvalidPassword) {
		t.Fatal("invalid password must be distinguishable only by validity, never account existence")
	}
	if _, err := NewEnrollmentFlow(nil, lookup, proof); !errors.Is(err, ErrEnrollmentUnavailable) {
		t.Fatal("missing registration port accepted")
	}
}
