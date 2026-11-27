package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"time"
)

// RecoveryCodeSecurityNoticeJob is intentionally token-free and carries only
// the immutable v25 event, recipient, lease and approved event type.
type RecoveryCodeSecurityNoticeJob struct {
	EventID   string
	Recipient string
	Kind      string
	LeaseID   string
}

type RecoveryCodeSecurityNoticeStore interface {
	Claim(context.Context, string) (RecoveryCodeSecurityNoticeJob, bool, error)
	Finalize(context.Context, string, string, DeliveryOutcome, string) error
}

type RecoveryCodeSecurityNoticeSender interface {
	SendRecoveryCodeSecurityNotice(context.Context, string, string, string) (string, error)
}

// RecoveryCodeSecurityNoticeWorker is independent from password-change,
// PASSWORD_RECOVERED and encrypted reset-token delivery workers.
// No public HTTP handler, scheduler or production sender is started.
type RecoveryCodeSecurityNoticeWorker struct {
	store  RecoveryCodeSecurityNoticeStore
	sender RecoveryCodeSecurityNoticeSender
	random io.Reader
}

func NewRecoveryCodeSecurityNoticeWorker(store RecoveryCodeSecurityNoticeStore,
	sender RecoveryCodeSecurityNoticeSender) (*RecoveryCodeSecurityNoticeWorker, error) {
	if store == nil || sender == nil {
		return nil, ErrDeliveryUnavailable
	}
	return &RecoveryCodeSecurityNoticeWorker{store: store, sender: sender, random: rand.Reader}, nil
}

func (w *RecoveryCodeSecurityNoticeWorker) DispatchOnce(ctx context.Context) (bool, error) {
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
	if job.LeaseID != lease || len(job.EventID) != 26 ||
		job.EventID[:4] != "evt_" || job.Recipient == "" ||
		(job.Kind != "RECOVERY_CODES_ENROLLED" && job.Kind != "RECOVERY_CODES_ROTATED") {
		return true, ErrDeliveryUnavailable
	}
	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	providerID, sendErr := w.sender.SendRecoveryCodeSecurityNotice(
		sendCtx, job.EventID, job.Recipient, job.Kind)
	cancel()
	if sendErr != nil {
		providerID = ""
	}
	outcome := DeliveryTemporary
	if sendErr == nil {
		if normalized, valid := NormalizeMailgunMessageID(providerID); valid {
			providerID = normalized
			outcome = DeliveryAccepted
		} else {
			providerID = ""
		}
	}
	if errors.Is(sendErr, ErrPermanentDelivery) {
		outcome = DeliveryPermanent
	}
	if err := w.store.Finalize(ctx, job.EventID, job.LeaseID, outcome, providerID); err != nil {
		return true, ErrDeliveryUnavailable
	}
	return true, nil
}
