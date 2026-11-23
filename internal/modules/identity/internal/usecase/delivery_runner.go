package usecase

import (
	"context"
	"time"
)

const (
	// Polling is intentionally serialized: one process does not start a
	// second send until the previous send/finalization has returned.
	DefaultMailPollInterval = 5 * time.Second
	MinMailPollInterval     = 100 * time.Millisecond
	MaxMailPollInterval     = 30 * time.Second
)

type DeliveryDispatcher interface {
	DispatchOnce(context.Context) (bool, error)
}

// DeliveryRunner polls a single, bounded dispatcher. Multiple processes may
// coexist; PostgreSQL claim leases serialize individual challenge deliveries.
// Fail-closed: an unexpected store/worker error exits this loop so a trusted
// process supervisor can restart it after an operator-controlled backoff.
type DeliveryRunner struct {
	dispatch DeliveryDispatcher
	interval time.Duration
}

func NewDeliveryRunner(dispatch DeliveryDispatcher, interval time.Duration) (*DeliveryRunner, error) {
	if dispatch == nil || interval < MinMailPollInterval || interval > MaxMailPollInterval {
		return nil, ErrDeliveryUnavailable
	}
	return &DeliveryRunner{dispatch: dispatch, interval: interval}, nil
}

func (r *DeliveryRunner) Run(ctx context.Context) error {
	if r == nil || r.dispatch == nil || r.interval < MinMailPollInterval ||
		r.interval > MaxMailPollInterval {
		return ErrDeliveryUnavailable
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	return r.runWithTicks(ctx, ticker.C)
}

func (r *DeliveryRunner) runWithTicks(ctx context.Context, ticks <-chan time.Time) error {
	for {
		// Give cancellation priority over a queued tick or a new claim.
		if err := ctx.Err(); err != nil {
			return nil
		}
		_, err := r.dispatch.DispatchOnce(ctx)
		if err != nil && ctx.Err() == nil {
			return ErrDeliveryUnavailable
		}
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-ticks:
			if !ok {
				return ErrDeliveryUnavailable
			}
		}
	}
}
