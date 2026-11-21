package usecase

import (
	"context"
	"errors"
	"testing"
	"time"
	"bytes"
)

type fakeDeliveryStore struct {
	job ProofDeliveryJob
	has bool
	claimed int
	final int
	outcome ProofDeliveryOutcome
	fail bool
}

func (f *fakeDeliveryStore) Claim(_ context.Context, lease string) (ProofDeliveryJob,bool,error) {
	f.claimed++
	if f.fail{return ProofDeliveryJob{},false,errors.New("sensitive db error")}
	if !f.has{return ProofDeliveryJob{},false,nil}
	j:=f.job
	j.LeaseID=lease
	return j,true,nil
}

func (f *fakeDeliveryStore) Finalize(_ context.Context,challenge,lease string, outcome ProofDeliveryOutcome)error{
	f.final++
	if f.fail{return errors.New("secret from DB")}
	if challenge!=f.job.ChallengeID || len(lease)!=26{return errors.New("invalid fence")}
	f.outcome=outcome
	return nil
}

type fakeDeliveryOpener struct { fail bool }
func (f fakeDeliveryOpener) Open(_,_,_ string, _ []byte)(string,error){
	if f.fail{return "",errors.New("secret key unavailable")}
	return "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",nil
}

type fakeDeliverySender struct {
	calls int
	key string
	recipient string
	secret string
	err error
}
func (f *fakeDeliverySender) SendVerification(_ context.Context,key,recipient,secret string)error {
	f.calls++
	f.key=key
	f.recipient=recipient
	f.secret=secret
	return f.err
}

func TestDeliveryWorkerAcceptsOnlyProviderAcknowledgement(t *testing.T){
	store:=&fakeDeliveryStore{has:true,job:ProofDeliveryJob{
		ChallengeID:"vch_AAAAAAAAAAAAAAAAAAAAAA",
		SubjectID:"sub_BBBBBBBBBBBBBBBBBBBBBB", Recipient:"alice@example.test",
		KeyID:"key-v1", SealedToken:bytes.Repeat([]byte{1},71),
		ExpiresAt:time.Now().Add(15*time.Minute),
	}}
	sender:=&fakeDeliverySender{}
	worker,err:=NewProofDeliveryWorker(store,fakeDeliveryOpener{},sender)
	if err!=nil{t.Fatal(err)}
	hit,err:=worker.DispatchOnce(context.Background())
	if err!=nil || !hit || sender.calls!=1 || store.final!=1 ||
		store.outcome!=DeliveryAccepted || sender.key!=store.job.ChallengeID ||
		sender.recipient!="alice@example.test" || len(sender.secret)!=43 {
		t.Fatal("provider acceptance not recorded under stable idempotency key")
	}
}

func TestDeliveryWorkerRetriesWithoutLoggingOrLeakingSecrets(t *testing.T){
	for _,tc:=range []struct{name string; sendErr error; decryptErr bool; ttl time.Duration; outcome ProofDeliveryOutcome; calls int}{
		{"transient",errors.New("API credential raw secret"),false,10*time.Minute,DeliveryTemporary,1},
		{"permanent",ErrPermanentDelivery,false,10*time.Minute,DeliveryPermanent,1},
		{"corrupt envelope",nil,true,10*time.Minute,DeliveryPermanent,0},
		{"expired",nil,false,-time.Minute,DeliveryPermanent,0},
	} {
		t.Run(tc.name,func(t *testing.T){
			store:=&fakeDeliveryStore{has:true,job:ProofDeliveryJob{
				ChallengeID:"vch_AAAAAAAAAAAAAAAAAAAAAA",
				SubjectID:"sub_BBBBBBBBBBBBBBBBBBBBBB",
				Recipient:"alice@example.test",
				KeyID:"key-v1",SealedToken:bytes.Repeat([]byte{2},71),
				ExpiresAt:time.Now().Add(tc.ttl),
			}}
			sender:=&fakeDeliverySender{err:tc.sendErr}
			w,_:=NewProofDeliveryWorker(store,fakeDeliveryOpener{fail:tc.decryptErr},sender)
			if done,err:=w.DispatchOnce(context.Background());!done || err!=nil || store.outcome!=tc.outcome || sender.calls!=tc.calls {
				t.Fatalf("bad delivery outcome done=%v err=%v",done,err)
			}
		})
	}
	if _,err:=NewProofDeliveryWorker(nil,fakeDeliveryOpener{},&fakeDeliverySender{});!errors.Is(err,ErrDeliveryUnavailable) {t.Fatal("missing store accepted")}
	store:=&fakeDeliveryStore{fail:true}
	w,_:=NewProofDeliveryWorker(store,fakeDeliveryOpener{},&fakeDeliverySender{})
	if _,err:=w.DispatchOnce(context.Background());!errors.Is(err,ErrDeliveryUnavailable) {t.Fatal("raw storage errors exposed")}
}
