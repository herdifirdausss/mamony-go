package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"time"
)

// RecoverySecurityNoticeJob must NEVER contain a password, recovery link,
// bearer token, password verifier, login session or assurance grant.
type RecoverySecurityNoticeJob struct {
	EventID string
	Recipient string
	LeaseID string
}

type RecoverySecurityNoticeStore interface {
	Claim(context.Context, string) (RecoverySecurityNoticeJob, bool, error)
	Finalize(context.Context, string, string, ProofDeliveryOutcome, string) error
}

type RecoverySecurityNoticeSender interface {
	SendPasswordRecovered(context.Context, string, string) (string, error)
}

// Explicitly distinct from SecurityNoticeWorker (PASSWORD_CHANGED) and
// RecoveryDeliveryWorker (token-bearing reset link). Unmounted by default.
type RecoverySecurityNoticeWorker struct {
	store RecoverySecurityNoticeStore
	sender RecoverySecurityNoticeSender
	random io.Reader
}

func NewRecoverySecurityNoticeWorker(store RecoverySecurityNoticeStore,
	sender RecoverySecurityNoticeSender) (*RecoverySecurityNoticeWorker, error) {
	if store==nil || sender==nil {
		return nil, ErrDeliveryUnavailable
	}
	return &RecoverySecurityNoticeWorker{store:store,sender:sender,random:rand.Reader},nil
}

func (w *RecoverySecurityNoticeWorker) DispatchOnce(ctx context.Context) (bool,error) {
	if w==nil || w.store==nil || w.sender==nil || w.random==nil ||
		ctx==nil || ctx.Err()!=nil {
		return false,ErrDeliveryUnavailable
	}
	lease,err:=newOpaqueID(w.random,"lse_")
	if err != nil {
		return false, ErrDeliveryUnavailable
	}
	job,found,err:=w.store.Claim(ctx,lease)
	if err != nil {
		return false, ErrDeliveryUnavailable
	}
	if !found {
		return false, nil
	}
	if job.LeaseID!=lease || len(job.EventID)!=26 || job.EventID[:4]!="evt_" ||
		job.Recipient=="" {
		return true,ErrDeliveryUnavailable
	}
	sendCtx,cancel:=context.WithTimeout(ctx,10*time.Second)
	providerID,sendErr:=w.sender.SendPasswordRecovered(sendCtx,job.EventID,job.Recipient)
	cancel()
	if sendErr != nil {
		providerID = ""
	}
	outcome:=DeliveryTemporary
	if sendErr==nil {
		if normalized,valid:=NormalizeMailgunMessageID(providerID); valid {
			providerID=normalized
			outcome=DeliveryAccepted
		} else {
			providerID=""
		}
	}
	if errors.Is(sendErr, ErrPermanentDelivery) {
		outcome = DeliveryPermanent
	}
	if err:=w.store.Finalize(ctx,job.EventID,job.LeaseID,outcome,providerID); err!=nil {
		return true,ErrDeliveryUnavailable
	}
	return true,nil
}
