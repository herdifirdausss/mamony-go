package usecase

import (
	"context"
	"errors"
	"time"
	"crypto/rand"
	"io"
)

// ErrDeliveryUnavailable is deliberately generic; a provider may include
// bearer URLs, SMTP credentials or contact data inside its original error.
var ErrDeliveryUnavailable = errors.New("email proof delivery unavailable")

// ErrPermanentDelivery is the only explicit signal that a sender must not
// retry this specific message. All other send failures are retryable.
var ErrPermanentDelivery = errors.New("permanent email delivery rejection")

type ProofDeliveryJob struct {
	ChallengeID string
	SubjectID string
	Recipient string
	KeyID string
	SealedToken []byte
	LeaseID string
	ExpiresAt time.Time
}

type ProofDeliveryOutcome string

const (
	DeliveryAccepted ProofDeliveryOutcome = "ACCEPTED"
	DeliveryTemporary ProofDeliveryOutcome = "TEMPORARY"
	DeliveryPermanent ProofDeliveryOutcome = "PERMANENT"
)

type ProofDeliveryStore interface {
	Claim(context.Context, string) (ProofDeliveryJob, bool, error)
	Finalize(context.Context, string, string, ProofDeliveryOutcome) error
}

type ProofEnvelopeOpener interface {
	Open(subjectID, challengeID, keyID string, sealed []byte) (string, error)
}

// EmailSender submits to an external provider; successful return means only
// that the provider accepted a message, NOT that a user received or opened it.
// The idempotency key is stable for the same challenge across retries.
type EmailSender interface {
	SendVerification(context.Context, string, string, string) error
}

type ProofDeliveryWorker struct {
	store ProofDeliveryStore
	opener ProofEnvelopeOpener
	sender EmailSender
	random io.Reader
	clock func()time.Time
}

func NewProofDeliveryWorker(store ProofDeliveryStore, opener ProofEnvelopeOpener, sender EmailSender) (*ProofDeliveryWorker,error) {
	if store==nil || opener==nil || sender==nil {return nil,ErrDeliveryUnavailable}
	return &ProofDeliveryWorker{store:store,opener:opener,sender:sender,random:rand.Reader,clock:time.Now},nil
}

// DispatchOnce is invoked only by a separately authorized bounded worker.
// A caller supplies a context that can be canceled on process shutdown.
// PostgreSQL leases and conditional completion handle crash/restart. An
// external timeout after provider acceptance can cause a duplicate send;
// providers with native idempotency should use the challenge ID as their key.
func (w *ProofDeliveryWorker) DispatchOnce(ctx context.Context) (bool,error) {
	if w==nil || w.store==nil || w.opener==nil || w.sender==nil {return false,ErrDeliveryUnavailable}
	lease,err:=newOpaqueID(w.random,"lse_")
	if err!=nil {return false,ErrDeliveryUnavailable}
	job,ok,err:=w.store.Claim(ctx,lease)
	if err!=nil{return false,ErrDeliveryUnavailable}
	if !ok{return false,nil}
	outcome:=DeliveryTemporary
	if !job.ExpiresAt.After(w.clock().Add(10*time.Second)) {
		outcome=DeliveryPermanent
	} else {
		secret,openErr:=w.opener.Open(job.SubjectID,job.ChallengeID,job.KeyID,job.SealedToken)
		if openErr!=nil || len(secret)!=43 {
			outcome=DeliveryPermanent
		} else {
			sendCtx,cancel:=context.WithTimeout(ctx,10*time.Second)
			sendErr:=w.sender.SendVerification(sendCtx,job.ChallengeID,job.Recipient,secret)
			cancel()
			switch {
			case sendErr==nil:
				outcome=DeliveryAccepted
			case errors.Is(sendErr,ErrPermanentDelivery):
				outcome=DeliveryPermanent
			}
		}
	}
	// Do not leak a raw provider/decryption error into logs, HTTP or CI.
	if err:=w.store.Finalize(ctx,job.ChallengeID,job.LeaseID,outcome);err!=nil {
		return true,ErrDeliveryUnavailable
	}
	return true,nil
}
