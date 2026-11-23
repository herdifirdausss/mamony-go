package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"strings"
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
	Finalize(context.Context, string, string, ProofDeliveryOutcome, string) error
}

// SecurityNoticeSender must submit a fixed, token-free password-changed
// message. An accepted submission is NOT proof of inbox delivery.
type SecurityNoticeSender interface {
	SendPasswordChanged(context.Context, string, string) (string, error)
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
	providerMessageID, sendErr := w.sender.SendPasswordChanged(sendCtx, job.EventID, job.Recipient)
	cancel()
	// Never persist or trust a provider message ID alongside a rejected or
	// ambiguous send, even if a misbehaving adapter returns one with an error.
	if sendErr != nil {
		providerMessageID = ""
	}
	outcome := DeliveryTemporary
	if sendErr == nil {
		if normalized, ok := NormalizeMailgunMessageID(providerMessageID); ok {
			providerMessageID = normalized
			outcome = DeliveryAccepted
		} else {
			// Invalid/missing acknowledgement cannot be correlated later.
			providerMessageID = ""
		}
	}
	switch {
	case outcome == DeliveryAccepted:
	case errors.Is(sendErr, ErrPermanentDelivery):
		outcome = DeliveryPermanent
	}
	// A provider acknowledgement must include an unambiguous message ID;
	// otherwise the job retries and may submit a duplicate. The DB stores
	// only its SHA-256 digest, not the raw provider message ID.
	if err := w.store.Finalize(ctx, job.EventID, job.LeaseID, outcome, providerMessageID); err != nil {
		return true, ErrDeliveryUnavailable
	}
	return true, nil
}

// NormalizeMailgunMessageID canonicalizes the Mailgun HTTP response ID
// (<local@domain>) and event-data.message.headers.message-id (local@domain).
// Never lowercase a message ID: its local part may be case-sensitive.
func NormalizeMailgunMessageID(raw string) (string, bool) {
	if len(raw) < 3 || len(raw) > 512 || strings.TrimSpace(raw) != raw {
		return "", false
	}
	if strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">") {
		raw = raw[1 : len(raw)-1]
	}
	if len(raw) < 3 || len(raw) > 510 || strings.Count(raw, "@") != 1 {
		return "", false
	}
	left, right, ok := strings.Cut(raw, "@")
	if !ok || left == "" || right == "" || strings.HasSuffix(right, ".") {
		return "", false
	}
	for _, c := range raw {
		if c <= 32 || c >= 127 || c == '<' || c == '>' ||
			c == ',' || c == '/' || c == '\\' {
			return "", false
		}
	}
	return raw, true
}
