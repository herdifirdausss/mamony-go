package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
)

type fakeRecoveryAssurance struct {
	calls int
	allowed bool
	err error
	recoveryDigest [32]byte
	offlineDigest [32]byte
	event string
}
func (f *fakeRecoveryAssurance) Approve(_ context.Context,recovery,code [32]byte,event string)(bool,error){
	f.calls++
	f.recoveryDigest,f.offlineDigest,f.event=recovery,code,event
	return f.allowed,f.err
}

type offlineFailReader struct{}
func (offlineFailReader) Read(_ []byte)(int,error){return 0,io.ErrUnexpectedEOF}

func TestOfflineRecoveryCodeProofIsolationAndRedaction(t *testing.T){
	c,err:=NewOfflineRecoveryCode()
	if err!=nil{t.Fatal(err)}
	b:=c.TrustedOfflineDisplay()
	if len(b)!=43{t.Fatal("offline recovery code was not 256 bits")}
	parsed,err:=OfflineRecoveryCodeDigest(b)
	if err!=nil || parsed!=c.Digest() {
		t.Fatal("offline recovery digest contract did not round-trip")
	}
	if parsed==sha256.Sum256([]byte(b)){
		t.Fatal("recovery backup code did not use purpose-separated digest")
	}
	if _,err:=PasswordRecoveryProofDigest(b);err!=nil{
		// The formats are intentionally same length, but the DIGEST
		// purposes must differ even for identical underlying entropy.
		t.Fatal("same-length offline code failed base64url parser")
	}
	emailDigest,_:=PasswordRecoveryProofDigest(b)
	if emailDigest==parsed {
		t.Fatal("offline code digest collided with email proof digest")
	}
	for _,raw:=range []string{"",b+"=",b+"!",strings.Repeat("0",42),"!!!"+b[3:]}{
		if _,err:=OfflineRecoveryCodeDigest(raw);err==nil{
			t.Fatal("malformed offline code accepted")
		}
	}
	if got:=fmt.Sprintf("%v|%+v|%#v",c,c,c);strings.Contains(got,b){
		t.Fatal("backup material leaked through fmt formatting")
	}
	if got:=c.LogValue();got.Kind()!=slog.KindString || strings.Contains(got.String(),b){
		t.Fatal("backup material leaked through structured logging")
	}
	if data,err:=json.Marshal(c);err==nil || len(data)>0{
		t.Fatal("offline code silently serialized as JSON")
	}
	if value,err:=c.MarshalText();err==nil || len(value)>0{
		t.Fatal("offline code silently serialized as text")
	}
	if _,err:=newOfflineRecoveryCode(offlineFailReader{});!errors.Is(err,ErrOfflineRecoveryCodeUnavailable){
		t.Fatal("failed CSPRNG produced a recovery code")
	}
}

func TestRecoveryOfflineCodeAssuranceFailClosed(t *testing.T){
	email,err:=NewPasswordRecoveryProof()
	if err!=nil{t.Fatal(err)}
	code,err:=NewOfflineRecoveryCode()
	if err!=nil{t.Fatal(err)}
	store:=&fakeRecoveryAssurance{allowed:true}
	worker,err:=NewRecoveryOfflineCodeAssurance(store)
	if err!=nil{t.Fatal(err)}
	if err:=worker.Approve(context.Background(),email.ForTrustedDelivery(),code.TrustedOfflineDisplay());err!=nil{
		t.Fatal("valid two separate recovery evidences did not reach store")
	}
	if store.calls!=1 || store.recoveryDigest!=email.Digest() ||
		store.offlineDigest!=code.Digest() ||
		len(store.event)!=26 || !strings.HasPrefix(store.event,"evt_"){
		t.Fatal("recovery assurance evidence was passed to SQL incorrectly")
	}
	if err:=worker.Approve(context.Background(),"bad",code.TrustedOfflineDisplay());!errors.Is(err,ErrRecoveryAssuranceDenied){
		t.Fatal("invalid recovery email proof reached restricted SQL")
	}
	if err:=worker.Approve(context.Background(),email.ForTrustedDelivery(),"bad");!errors.Is(err,ErrRecoveryAssuranceDenied){
		t.Fatal("invalid offline code reached restricted SQL")
	}
	if store.calls!=1 {t.Fatal("malformed evidence still executed SQL")}
	store.allowed=false
	if err:=worker.Approve(context.Background(),email.ForTrustedDelivery(),code.TrustedOfflineDisplay());!errors.Is(err,ErrRecoveryAssuranceDenied){
		t.Fatal("SQL denial was not sanitized")
	}
	store.err=errors.New("sensitive account and SQL DSN")
	if err:=worker.Approve(context.Background(),email.ForTrustedDelivery(),code.TrustedOfflineDisplay());!errors.Is(err,ErrRecoveryAssuranceUnavailable) ||
		err.Error()!=ErrRecoveryAssuranceUnavailable.Error(){
		t.Fatal("backend error leaked sensitive details")
	}
	cancelled,cancel:=context.WithCancel(context.Background());cancel()
	if err:=worker.Approve(cancelled,email.ForTrustedDelivery(),code.TrustedOfflineDisplay());!errors.Is(err,ErrRecoveryAssuranceUnavailable){
		t.Fatal("cancelled context performed approval")
	}
}
