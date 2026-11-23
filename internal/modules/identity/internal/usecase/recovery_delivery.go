package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

var ErrRecoveryDeliveryUnavailable = errors.New("password recovery delivery unavailable")

type RecoveryDeliveryJob struct {
	ChallengeID string
	Recipient   string
	KeyID       string
	SealedToken []byte
	LeaseID     string
	ExpiresAt   time.Time
}

type RecoveryDeliveryStore interface {
	Claim(context.Context, string) (RecoveryDeliveryJob, bool, error)
	Finalize(context.Context, string, string, ProofDeliveryOutcome) error
}

type RecoveryEnvelopeOpener interface {
	OpenRecovery(email, challengeID, keyID string, sealed []byte) (string, error)
}

// RecoverySender submits only a reset proof to the verified email destination.
// It is never a replacement for subject verification, login, session issuance
// or a stronger-factor/MFA account recovery approval.
type RecoverySender interface {
	SendRecovery(context.Context, string, string, string) error
}

// RecoveryDeliveryWorker is deliberately unmounted and owns neither public
// HTTP nor a running process. Provider acceptance is not inbox delivery.
type RecoveryDeliveryWorker struct {
	store  RecoveryDeliveryStore
	opener RecoveryEnvelopeOpener
	sender RecoverySender
	random io.Reader
	clock  func() time.Time
}

func NewRecoveryDeliveryWorker(store RecoveryDeliveryStore, opener RecoveryEnvelopeOpener,
	sender RecoverySender) (*RecoveryDeliveryWorker, error) {
	if store == nil || opener == nil || sender == nil {
		return nil, ErrRecoveryDeliveryUnavailable
	}
	return &RecoveryDeliveryWorker{
		store: store, opener: opener, sender: sender,
		random: rand.Reader, clock: time.Now,
	}, nil
}

func (w *RecoveryDeliveryWorker) DispatchOnce(ctx context.Context) (bool, error) {
	if w == nil || w.store == nil || w.opener == nil || w.sender == nil ||
		w.random == nil || w.clock == nil || ctx == nil || ctx.Err() != nil {
		return false, ErrRecoveryDeliveryUnavailable
	}
	lease, err := newOpaqueID(w.random, "lse_")
	if err != nil {
		return false, ErrRecoveryDeliveryUnavailable
	}
	job, found, err := w.store.Claim(ctx, lease)
	if err != nil {
		return false, ErrRecoveryDeliveryUnavailable
	}
	if !found {
		return false, nil
	}
	if job.LeaseID != lease || len(job.ChallengeID) != 26 ||
		job.ChallengeID[:4] != "rcv_" {
		return true, ErrRecoveryDeliveryUnavailable
	}
	outcome := DeliveryTemporary
	canonical, aliasErr := domain.CanonicalEmail(job.Recipient)
	if aliasErr != nil || canonical != job.Recipient || job.KeyID == "" ||
		!job.ExpiresAt.After(w.clock().Add(15*time.Second)) ||
		len(job.SealedToken) < 60 || len(job.SealedToken) > 256 {
		outcome = DeliveryPermanent
	} else {
		secret, openErr := w.opener.OpenRecovery(job.Recipient, job.ChallengeID,
			job.KeyID, job.SealedToken)
		if openErr != nil {
			outcome = DeliveryPermanent
		} else if _, err := PasswordRecoveryProofDigest(secret); err != nil {
			outcome = DeliveryPermanent
		} else {
			sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			sendErr := w.sender.SendRecovery(sendCtx, job.ChallengeID,
				job.Recipient, secret)
			cancel()
			switch {
			case sendErr == nil:
				outcome = DeliveryAccepted
			case errors.Is(sendErr, ErrPermanentDelivery):
				outcome = DeliveryPermanent
			}
		}
	}
	// Provider errors may contain full bearer URLs, emails or API secrets:
	// never propagate their text outside the trusted adapter.
	if err := w.store.Finalize(ctx, job.ChallengeID, job.LeaseID, outcome); err != nil {
		return true, ErrRecoveryDeliveryUnavailable
	}
	return true, nil
}
