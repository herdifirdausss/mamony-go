package usecase

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeDispatcher struct {
	calls int
	done  chan struct{}
	fail  error
}

func (f *fakeDispatcher) DispatchOnce(context.Context) (bool, error) {
	f.calls++
	if f.done != nil {
		f.done <- struct{}{}
	}
	return true, f.fail
}

func TestDeliveryRunnerPollsSeriallyAndStopsOnShutdown(t *testing.T) {
	f := &fakeDispatcher{done: make(chan struct{}, 4)}
	r, err := NewDeliveryRunner(f, DefaultMailPollInterval)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time, 2)
	result := make(chan error, 1)
	go func() { result <- r.runWithTicks(ctx, ticks) }()
	<-f.done
	ticks <- time.Now()
	<-f.done
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not stop delivery polling")
	}
	if f.calls != 2 {
		t.Fatalf("expected exactly two serial dispatches, got %d", f.calls)
	}
}

func TestDeliveryRunnerRejectsMissingAuthorityAndUnsafeIntervals(t *testing.T) {
	for _, d := range []time.Duration{0, MinMailPollInterval - time.Nanosecond, MaxMailPollInterval + time.Nanosecond} {
		if _, err := NewDeliveryRunner(&fakeDispatcher{}, d); !errors.Is(err, ErrDeliveryUnavailable) {
			t.Fatal("unsafe interval accepted")
		}
	}
	if _, err := NewDeliveryRunner(nil, DefaultMailPollInterval); !errors.Is(err, ErrDeliveryUnavailable) {
		t.Fatal("missing worker accepted")
	}
	f := &fakeDispatcher{fail: errors.New("sensitive provider URL and credential")}
	r, _ := NewDeliveryRunner(f, DefaultMailPollInterval)
	if err := r.runWithTicks(context.Background(), make(chan time.Time)); !errors.Is(err, ErrDeliveryUnavailable) ||
		err.Error() != "email proof delivery unavailable" {
		t.Fatalf("raw error details reached supervisor: %v", err)
	}
}
