package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type recoveryDeliveryStoreMock struct {
	job       RecoveryDeliveryJob
	found     bool
	claimed   int
	finalized int
	outcome   DeliveryOutcome
	claimErr  error
	finalErr  error
}

func (m *recoveryDeliveryStoreMock) Claim(_ context.Context, lease string) (RecoveryDeliveryJob, bool, error) {
	m.claimed++
	job := m.job
	job.LeaseID = lease
	return job, m.found, m.claimErr
}

func (m *recoveryDeliveryStoreMock) Finalize(_ context.Context, _, _ string, outcome DeliveryOutcome) error {
	m.finalized++
	m.outcome = outcome
	return m.finalErr
}

type recoveryEnvelopeOpenerMock struct {
	calls int
	token string
	err   error
}

func (m *recoveryEnvelopeOpenerMock) OpenRecovery(_, _, _ string, _ []byte) (string, error) {
	m.calls++
	return m.token, m.err
}

type recoverySenderMock struct {
	calls int
	token string
	err   error
}

func (m *recoverySenderMock) SendRecovery(_ context.Context, _, _, token string) error {
	m.calls++
	m.token = token
	return m.err
}

func TestRecoveryDeliveryWorkerOutcomeIsolation(t *testing.T) {
	now := time.Now()
	newWorker := func(sendErr, openErr error) (*RecoveryDeliveryWorker,
		*recoveryDeliveryStoreMock, *recoveryEnvelopeOpenerMock, *recoverySenderMock) {
		store := &recoveryDeliveryStoreMock{
			found: true,
			job: RecoveryDeliveryJob{
				ChallengeID: "rcv_AAAAAAAAAAAAAAAAAAAAAA",
				Recipient:   "owner@example.test", KeyID: "recovery-key",
				SealedToken: make([]byte, 71), ExpiresAt: now.Add(15 * time.Minute),
			},
		}
		opener := &recoveryEnvelopeOpenerMock{
			token: strings.Repeat("A", 43), err: openErr,
		}
		sender := &recoverySenderMock{err: sendErr}
		w, err := NewRecoveryDeliveryWorker(store, opener, sender)
		if err != nil {
			t.Fatal(err)
		}
		w.clock = func() time.Time { return now }
		return w, store, opener, sender
	}
	for _, tc := range []struct {
		name  string
		send  error
		open  error
		want  DeliveryOutcome
		calls int
	}{
		{"success", nil, nil, DeliveryAccepted, 1},
		{"retryable", errors.New("provider error with private URL"), nil, DeliveryTemporary, 1},
		{"permanent", ErrPermanentDelivery, nil, DeliveryPermanent, 1},
		{"tampered envelope", nil, errors.New("raw secret in AEAD error"), DeliveryPermanent, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worker, store, opener, sender := newWorker(tc.send, tc.open)
			worked, err := worker.DispatchOnce(context.Background())
			if err != nil || !worked || store.claimed != 1 ||
				store.finalized != 1 || store.outcome != tc.want ||
				opener.calls != 1 || sender.calls != tc.calls {
				t.Fatalf("wrong recovery delivery failure classification: worked=%v err=%v", worked, err)
			}
		})
	}
	t.Run("expired job does not reveal bearer", func(t *testing.T) {
		worker, store, opener, sender := newWorker(nil, nil)
		store.job.ExpiresAt = now.Add(10 * time.Second)
		worked, err := worker.DispatchOnce(context.Background())
		if err != nil || !worked || store.outcome != DeliveryPermanent ||
			opener.calls != 0 || sender.calls != 0 {
			t.Fatal("expired recovery ciphertext reached provider")
		}
	})
	t.Run("wrong current contact does not reveal bearer", func(t *testing.T) {
		worker, store, opener, sender := newWorker(nil, nil)
		store.job.Recipient = "BAD"
		worked, err := worker.DispatchOnce(context.Background())
		if err != nil || !worked || store.outcome != DeliveryPermanent ||
			opener.calls != 0 || sender.calls != 0 {
			t.Fatal("invalid recovery destination reached provider")
		}
	})
	t.Run("no job", func(t *testing.T) {
		worker, store, _, sender := newWorker(nil, nil)
		store.found = false
		worked, err := worker.DispatchOnce(context.Background())
		if err != nil || worked || store.finalized != 0 || sender.calls != 0 {
			t.Fatal("empty recovery queue caused a send")
		}
	})
	t.Run("finalization outage is sanitized", func(t *testing.T) {
		worker, store, _, _ := newWorker(nil, nil)
		store.finalErr = errors.New("private password recovery bearer in SQL diagnostics")
		worked, err := worker.DispatchOnce(context.Background())
		if !worked || !errors.Is(err, ErrRecoveryDeliveryUnavailable) ||
			strings.Contains(err.Error(), "private") {
			t.Fatal("recovery worker leaked storage details")
		}
	})
	t.Run("claim outage is sanitized", func(t *testing.T) {
		worker, store, _, sender := newWorker(nil, nil)
		store.claimErr = errors.New("private email in SQL diagnostics")
		worked, err := worker.DispatchOnce(context.Background())
		if worked || !errors.Is(err, ErrRecoveryDeliveryUnavailable) || sender.calls != 0 {
			t.Fatal("recovery worker ignored storage outage")
		}
	})
}
