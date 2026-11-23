package usecase

import (
	"context"
	"errors"
	"testing"
)

type fakeNoticeStore struct {
	job      SecurityNoticeJob
	found    bool
	err      error
	finalErr error
	claims   int
	finals   int
	outcome  ProofDeliveryOutcome
}

func (f *fakeNoticeStore) Claim(_ context.Context, lease string) (SecurityNoticeJob, bool, error) {
	f.claims++
	if f.err != nil || !f.found {
		return SecurityNoticeJob{}, false, f.err
	}
	job := f.job
	job.LeaseID = lease
	return job, true, nil
}

func (f *fakeNoticeStore) Finalize(_ context.Context, event, lease string, result ProofDeliveryOutcome, providerID string) error {
	f.finals++
	if f.finalErr != nil {
		return f.finalErr
	}
	if event != f.job.EventID || len(lease) != 26 {
		return errors.New("lease fence mismatch")
	}
	if result == DeliveryAccepted && providerID != "<synthetic@mailgun.test>" && providerID != "synthetic@mailgun.test" {
		return errors.New("provider message ID mismatch")
	}
	f.outcome = result
	return nil
}

type fakeNoticeSender struct {
	err       error
	calls     int
	event     string
	recipient string
}

func (f *fakeNoticeSender) SendPasswordChanged(_ context.Context, event, recipient string) (string, error) {
	f.calls++
	f.event = event
	f.recipient = recipient
	if f.err != nil {
		return "", f.err
	}
	return "<synthetic@mailgun.test>", nil
}

func TestSecurityNoticeWorkerBoundedDeliveryOutcomes(t *testing.T) {
	const event = "evt_AAAAAAAAAAAAAAAAAAAAAA"
	for _, tc := range []struct {
		label   string
		sendErr error
		want    ProofDeliveryOutcome
	}{
		{label: "accepted only at provider boundary", sendErr: nil, want: DeliveryAccepted},
		{label: "transient error redacted", sendErr: errors.New("secret provider URL"), want: DeliveryTemporary},
		{label: "permanent rejection", sendErr: ErrPermanentDelivery, want: DeliveryPermanent},
	} {
		t.Run(tc.label, func(t *testing.T) {
			store := &fakeNoticeStore{found: true, job: SecurityNoticeJob{
				EventID: event, Recipient: "alice@example.test",
			}}
			sender := &fakeNoticeSender{err: tc.sendErr}
			worker, err := NewSecurityNoticeWorker(store, sender)
			if err != nil {
				t.Fatal(err)
			}
			did, err := worker.DispatchOnce(context.Background())
			if err != nil || !did || store.claims != 1 || store.finals != 1 ||
				store.outcome != tc.want || sender.calls != 1 ||
				sender.event != event || sender.recipient != "alice@example.test" {
				t.Fatalf("notification outcome not fenced: %v %v", did, err)
			}
		})
	}
	t.Run("no pending work", func(t *testing.T) {
		store := &fakeNoticeStore{}
		sender := &fakeNoticeSender{}
		w, _ := NewSecurityNoticeWorker(store, sender)
		did, err := w.DispatchOnce(context.Background())
		if did || err != nil || sender.calls != 0 || store.finals != 0 {
			t.Fatal("empty queue caused a send")
		}
	})
	t.Run("store and finalize errors are never exposed", func(t *testing.T) {
		store := &fakeNoticeStore{found: true,
			job:      SecurityNoticeJob{EventID: event, Recipient: "alice@example.test"},
			finalErr: errors.New("db password leaked"),
		}
		sender := &fakeNoticeSender{}
		w, _ := NewSecurityNoticeWorker(store, sender)
		did, err := w.DispatchOnce(context.Background())
		if !did || !errors.Is(err, ErrDeliveryUnavailable) ||
			err.Error() != ErrDeliveryUnavailable.Error() {
			t.Fatal("raw finalization error was exposed or treated as accepted")
		}
		store.err = errors.New("raw DSN credentials")
		did, err = w.DispatchOnce(context.Background())
		if did || !errors.Is(err, ErrDeliveryUnavailable) ||
			err.Error() != ErrDeliveryUnavailable.Error() {
			t.Fatal("raw claim failure was exposed")
		}
	})
	t.Run("missing authority and canceled context fail closed", func(t *testing.T) {
		if _, err := NewSecurityNoticeWorker(nil, &fakeNoticeSender{}); !errors.Is(err, ErrDeliveryUnavailable) {
			t.Fatal("missing notice store accepted")
		}
		store := &fakeNoticeStore{}
		w, _ := NewSecurityNoticeWorker(store, &fakeNoticeSender{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if done, err := w.DispatchOnce(ctx); done || !errors.Is(err, ErrDeliveryUnavailable) ||
			store.claims != 0 {
			t.Fatal("canceled notice dispatch claimed work")
		}
	})
}
