package usecase

import (
	"context"
	"errors"
	"testing"
)

type fakeRecoveryNoticeStore struct {
	job      RecoverySecurityNoticeJob
	found    bool
	claimErr error
	finalErr error
	claims   int
	finals   int
	outcome  DeliveryOutcome
	provider string
}

func (s *fakeRecoveryNoticeStore) Claim(_ context.Context, lease string) (RecoverySecurityNoticeJob, bool, error) {
	s.claims++
	job := s.job
	job.LeaseID = lease
	return job, s.found, s.claimErr
}
func (s *fakeRecoveryNoticeStore) Finalize(_ context.Context, _, _ string, outcome DeliveryOutcome, provider string) error {
	s.finals++
	s.outcome, s.provider = outcome, provider
	return s.finalErr
}

type fakeRecoveryNoticeSender struct {
	err   error
	id    string
	calls int
}

func (s *fakeRecoveryNoticeSender) SendPasswordRecovered(_ context.Context, _, _ string) (string, error) {
	s.calls++
	return s.id, s.err
}

func TestRecoverySecurityNoticeWorkerOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sendErr  error
		provider string
		want     DeliveryOutcome
		wantID   string
	}{
		{"accepted", nil, "<synthetic@mailgun.test>", DeliveryAccepted, "synthetic@mailgun.test"},
		{"throttled", errors.New("secret email and URL"), "", DeliveryTemporary, ""},
		{"hard rejected", ErrPermanentDelivery, "", DeliveryPermanent, ""},
		{"invalid acknowledgement", nil, "bad-response", DeliveryTemporary, ""},
		{"malicious adapter", ErrPermanentDelivery, "<synthetic@mailgun.test>", DeliveryPermanent, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeRecoveryNoticeStore{found: true, job: RecoverySecurityNoticeJob{
				EventID: "evt_AAAAAAAAAAAAAAAAAAAAAA", Recipient: "owner@example.test",
			}}
			sender := &fakeRecoveryNoticeSender{err: tc.sendErr, id: tc.provider}
			worker, err := NewRecoverySecurityNoticeWorker(store, sender)
			if err != nil {
				t.Fatal(err)
			}
			worked, err := worker.DispatchOnce(context.Background())
			if !worked || err != nil || store.claims != 1 || store.finals != 1 ||
				store.outcome != tc.want || store.provider != tc.wantID || sender.calls != 1 {
				t.Fatal("recovery security notice was not finalized safely")
			}
		})
	}
	t.Run("empty queue", func(t *testing.T) {
		store := &fakeRecoveryNoticeStore{}
		sender := &fakeRecoveryNoticeSender{}
		worker, err := NewRecoverySecurityNoticeWorker(store, sender)
		if err != nil {
			t.Fatal(err)
		}
		worked, err := worker.DispatchOnce(context.Background())
		if worked || err != nil || sender.calls != 0 || store.finals != 0 {
			t.Fatal("empty recovery notice queue caused email")
		}
	})
	t.Run("storage outage redacted", func(t *testing.T) {
		store := &fakeRecoveryNoticeStore{found: true, job: RecoverySecurityNoticeJob{
			EventID: "evt_AAAAAAAAAAAAAAAAAAAAAA", Recipient: "owner@example.test",
		}, finalErr: errors.New("DSN and account leak")}
		worker, err := NewRecoverySecurityNoticeWorker(store, &fakeRecoveryNoticeSender{id: "id@mailgun.test"})
		if err != nil {
			t.Fatal(err)
		}
		worked, err := worker.DispatchOnce(context.Background())
		if !worked || !errors.Is(err, ErrDeliveryUnavailable) ||
			err.Error() != ErrDeliveryUnavailable.Error() {
			t.Fatal("unsafe finalization error escaped")
		}
		store.claimErr = errors.New("sensitive DB credentials")
		worked, err = worker.DispatchOnce(context.Background())
		if worked || !errors.Is(err, ErrDeliveryUnavailable) {
			t.Fatal("unsafe claim failure escaped")
		}
	})
}
