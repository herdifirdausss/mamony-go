package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"time"
)

// SecurityNoticeJob intentionally contains only the immutable event ID and
// verified recipient snapshot. Never include a password, session, grant or
// bearer secret in an email or durable notification queue.
type SecurityNoticeJob struct {
	EventID   string
	Recipient string
	LeaseID   string
}

type SecurityNoticeStore interface {
	Claim(context.Context, string) (SecurityNoticeJob, bool, error)
	Finalize(context.Context, string, string, ProofDeliveryOutcome) error
}

// SecurityNoticeSender must submit a fixed, token-free password-changed
// message. An accepted submission is NOT proof of inbox delivery.
type SecurityNoticeSender interface {
	SendPasswordChanged(context.Context, string, string) error
}

// SecurityNoticeWorker reuses the established lease/retry runner contract,
// but NOT encrypted verification-token deliveries or their SQL permissions.
type SecurityNoticeWorker struct {
	store  SecurityNoticeStore
	sender SecurityNoticeSender
	random io.Reader
}

func NewSecurityNoticeWorker(store SecurityNoticeStore, sender SecurityNoticeSender) (*SecurityNoticeWorker, error) {
	if store == nil || sender == nil {
		return nil, ErrDeliveryUnavailable
	}
	return &SecurityNoticeWorker{store: store, sender: sender, random: rand.Reader}, nil
}

func (w *SecurityNoticeWorker) DispatchOnce(ctx context.Context) (bool, error) {
	if w == nil || w.store == nil || w.sender == nil || w.random == nil ||
		ctx == nil || ctx.Err() != nil {
		return false, ErrDeliveryUnavailable
	}
	lease, err := newOpaqueID(w.random, "lse_")
	if err != nil {
		return false, ErrDeliveryUnavailable
	}
	job, found, err := w.store.Claim(ctx, lease)
	if err != nil {
		return false, ErrDeliveryUnavailable
	}
	if !found {
		return false, nil
	}
	if job.EventID == "" || job.LeaseID != lease || job.Recipient == "" {
		return true, ErrDeliveryUnavailable
	}
	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	sendErr := w.sender.SendPasswordChanged(sendCtx, job.EventID, job.Recipient)
	cancel()
	outcome := DeliveryTemporary
	switch {
	case sendErr == nil:
		outcome = DeliveryAccepted
	case errors.Is(sendErr, ErrPermanentDelivery):
		outcome = DeliveryPermanent
	}
	// Provider error messages are NEVER returned or persisted; a lost response
	// after provider acceptance may cause at-least-once duplicate submission.
	if err := w.store.Finalize(ctx, job.EventID, job.LeaseID, outcome); err != nil {
		return true, ErrDeliveryUnavailable
	}
	return true, nil
}
