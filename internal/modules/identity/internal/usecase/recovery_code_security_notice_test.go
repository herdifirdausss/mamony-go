package usecase

import (
	"context"
	"errors"
	"io"
	"testing"
)

type fakeRecoveryCodeNoticeStore struct {
	job        RecoveryCodeSecurityNoticeJob
	found      bool
	claimErr   error
	finalErr   error
	calls      int
	outcome    ProofDeliveryOutcome
	providerID string
}

func (s *fakeRecoveryCodeNoticeStore) Claim(_ context.Context, lease string) (RecoveryCodeSecurityNoticeJob, bool, error) {
	if !s.found || s.claimErr != nil {
		return RecoveryCodeSecurityNoticeJob{}, s.found, s.claimErr
	}
	s.job.LeaseID = lease
	return s.job, true, nil
}
func (s *fakeRecoveryCodeNoticeStore) Finalize(_ context.Context, _, _ string, o ProofDeliveryOutcome, id string) error {
	s.calls++
	s.outcome = o
	s.providerID = id
	return s.finalErr
}

type fakeRecoveryCodeNoticeSender struct {
	calls int
	kind  string
	to    string
	err   error
	id    string
}

func (s *fakeRecoveryCodeNoticeSender) SendRecoveryCodeSecurityNotice(_ context.Context, _, recipient, kind string) (string, error) {
	s.calls++
	s.kind = kind
	s.to = recipient
	return s.id, s.err
}

func TestRecoveryCodeSecurityNoticeWorkerClaimAndFinalize(t *testing.T) {
	for _, kind := range []string{"RECOVERY_CODES_ENROLLED", "RECOVERY_CODES_ROTATED"} {
		store := &fakeRecoveryCodeNoticeStore{
			found: true, job: RecoveryCodeSecurityNoticeJob{
				EventID:   "evt_AAAAAAAAAAAAAAAAAAAAAA",
				Recipient: "fixture@example.test", Kind: kind,
			},
		}
		sender := &fakeRecoveryCodeNoticeSender{id: "<fixture-code-changed@mailgun.test>"}
		worker, err := NewRecoveryCodeSecurityNoticeWorker(store, sender)
		if err != nil {
			t.Fatal(err)
		}
		if processed, err := worker.DispatchOnce(context.Background()); err != nil || !processed {
			t.Fatal("accepted notice was not finalized")
		}
		if sender.calls != 1 || sender.kind != kind || sender.to != "fixture@example.test" ||
			store.calls != 1 || store.outcome != DeliveryAccepted ||
			store.providerID != "fixture-code-changed@mailgun.test" {
			t.Fatal("wrong event type or normalized provider digest passed to SQL")
		}
	}
}

func TestRecoveryCodeSecurityNoticeWorkerFailureAndFencing(t *testing.T) {
	for _, tc := range []struct {
		name       string
		sendErr    error
		providerID string
		outcome    ProofDeliveryOutcome
	}{
		{name: "transport temporary", sendErr: ErrDeliveryUnavailable, outcome: DeliveryTemporary},
		{name: "provider permanent", sendErr: ErrPermanentDelivery, outcome: DeliveryPermanent},
		{name: "malformed acknowledgement", providerID: "not a provider ID", outcome: DeliveryTemporary},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeRecoveryCodeNoticeStore{
				found: true, job: RecoveryCodeSecurityNoticeJob{
					EventID:   "evt_AAAAAAAAAAAAAAAAAAAAAA",
					Recipient: "fixture@example.test", Kind: "RECOVERY_CODES_ROTATED",
				},
			}
			sender := &fakeRecoveryCodeNoticeSender{err: tc.sendErr, id: tc.providerID}
			worker, err := NewRecoveryCodeSecurityNoticeWorker(store, sender)
			if err != nil {
				t.Fatal(err)
			}
			if processed, err := worker.DispatchOnce(context.Background()); err != nil || !processed {
				t.Fatal("temporary/permanent attempt did not finalize")
			}
			if store.outcome != tc.outcome || store.providerID != "" {
				t.Fatal("failed Mailgun response incorrectly carried accepted provider ID")
			}
		})
	}
	for _, invalid := range []string{"PASSWORD_RECOVERED", "PASSWORD_CHANGED", "RECOVERY_CODES_ISSUED"} {
		store := &fakeRecoveryCodeNoticeStore{found: true, job: RecoveryCodeSecurityNoticeJob{
			EventID: "evt_AAAAAAAAAAAAAAAAAAAAAA", Recipient: "fixture@example.test", Kind: invalid,
		}}
		sender := &fakeRecoveryCodeNoticeSender{id: "fixture@mailgun.test"}
		worker, _ := NewRecoveryCodeSecurityNoticeWorker(store, sender)
		if _, err := worker.DispatchOnce(context.Background()); !errors.Is(err, ErrDeliveryUnavailable) ||
			store.calls != 0 || sender.calls != 0 {
			t.Fatal("worker sent an unrecognized event or finalized a forged claim")
		}
	}
	store := &fakeRecoveryCodeNoticeStore{}
	sender := &fakeRecoveryCodeNoticeSender{}
	worker, _ := NewRecoveryCodeSecurityNoticeWorker(store, sender)
	if found, err := worker.DispatchOnce(context.Background()); err != nil || found {
		t.Fatal("idle worker reported a send")
	}
	worker.random = io.LimitReader(nil, 0)
	if _, err := worker.DispatchOnce(context.Background()); !errors.Is(err, ErrDeliveryUnavailable) {
		t.Fatal("failed CSPRNG still leased a row")
	}
	if _, err := NewRecoveryCodeSecurityNoticeWorker(nil, sender); err == nil {
		t.Fatal("worker accepted missing SQL boundary")
	}
	if _, err := NewRecoveryCodeSecurityNoticeWorker(store, nil); err == nil {
		t.Fatal("worker accepted missing provider")
	}
}
