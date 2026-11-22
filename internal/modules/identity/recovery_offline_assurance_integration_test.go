//go:build integration

package identity

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/credential"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityZRecoveryOfflineAssurance tests a real restricted SQL approval
// against disposable Postgres with synthetically PRE-ENROLLED offline codes.
// No browser/HTTP enrollment is implemented, and no live codes are issued.
func TestIdentityZRecoveryOfflineAssurance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 170*time.Second)
	defer cancel()
	adminDSN := guardedDB(t,"TEST_DATABASE_ADMIN_URL","mamony_test_admin")
	issueDSN := guardedDB(t,"TEST_IDENTITY_RECOVERY_ISSUER_URL","mamony_identity_recovery_issuer_runtime")
	assuranceDSN := guardedDB(t,"TEST_IDENTITY_RECOVERY_ASSURANCE_URL","mamony_identity_recovery_assurance_runtime")
	redeemDSN := guardedDB(t,"TEST_IDENTITY_RECOVERY_REDEEMER_URL","mamony_identity_recovery_redeemer_runtime")
	root,_:=url.Parse(adminDSN)
	for _,raw:=range []string{issueDSN,assuranceDSN,redeemDSN}{
		parsed,_:=url.Parse(raw)
		if parsed.Host!=root.Host || parsed.Path!=root.Path{
			t.Fatal("assurance proof cannot use a non-disposable DB")
		}
	}
	admin,err:=pgx.Connect(ctx,adminDSN)
	if err!=nil{t.Fatal(err)}
	defer func(){_ = admin.Close(context.Background())}()
	pool:=func(dsn,role string)*platformpg.Pool{
		t.Helper()
		p,err:=platformpg.Open(ctx,platformpg.Config{DSN:dsn,RuntimeRole:role,MaxConns:4})
		if err!=nil{t.Fatal(err)}
		t.Cleanup(p.Close)
		return p
	}
	issuer,err:=identitypg.NewRecoveryIssuerStore(pool(issueDSN,"mamony_identity_recovery_issuer_runtime"))
	if err!=nil{t.Fatal(err)}
	store,err:=identitypg.NewRecoveryAssuranceStore(pool(assuranceDSN,"mamony_identity_recovery_assurance_runtime"))
	if err!=nil{t.Fatal(err)}
	redeemer,err:=identitypg.NewRecoveryRedemptionStore(pool(redeemDSN,"mamony_identity_recovery_redeemer_runtime"))
	if err!=nil{t.Fatal(err)}
	assurer,err:=usecase.NewRecoveryOfflineCodeAssurance(store)
	if err!=nil{t.Fatal(err)}
	hasher,err:=credential.NewPasswordHasher(2)
	if err!=nil{t.Fatal(err)}
	policy:=domain.PasswordPolicy{IsBlocked:func(value,canonicalEmail string)bool{
		return value=="synthetic compromised password" || strings.Contains(value,canonicalEmail)
	}}
	reset,err:=usecase.NewPasswordRecoveryRedemption(redeemer,hasher,policy)
	if err!=nil{t.Fatal(err)}
	type fixture struct{
		subject string
		email string
		epoch int64
		token usecase.PasswordRecoveryProof
		challenge string
	}
	makeSubject:=func()fixture{
		t.Helper()
		f:=fixture{subject:randomSyntheticID(t,"sub_"),
			email:strings.ToLower(randomSyntheticID(t,"offline_"))+"@example.test"}
		if _,err:=admin.Exec(ctx,`INSERT INTO mamony_identity.subjects
			(subject_id,created_at) VALUES($1,clock_timestamp())`,f.subject);err!=nil{
			t.Fatal(err)
		}
		if _,err:=admin.Exec(ctx,`INSERT INTO mamony_identity.email_aliases
			(canonical_email,subject_id,verified_at,created_at)
			VALUES($1,$2,clock_timestamp(),clock_timestamp())`,f.email,f.subject);err!=nil{
			t.Fatal(err)
		}
		initial,err:=hasher.Hash(ctx,"synthetic initial offline recovery baseline")
		if err!=nil{t.Fatal(err)}
		if _,err:=admin.Exec(ctx,`INSERT INTO mamony_identity.password_verifiers
			(subject_id,kdf_version,memory_kib,iterations,parallelism,salt,digest,created_at)
			VALUES($1,1,65536,3,1,$2,$3,clock_timestamp())`,
			f.subject,initial.Salt,initial.Digest);err!=nil{t.Fatal(err)}
		if _,err:=admin.Exec(ctx,
			"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1",
			f.subject);err!=nil{t.Fatal(err)}
		if err:=admin.QueryRow(ctx,"SELECT session_epoch FROM mamony_identity.subjects WHERE subject_id=$1",
			f.subject).Scan(&f.epoch);err!=nil{t.Fatal(err)}
		t.Cleanup(func(){
			cleanupCtx,done:=context.WithTimeout(context.Background(),5*time.Second)
			defer done()
			conn,err:=pgx.Connect(cleanupCtx,adminDSN)
			if err!=nil{t.Errorf("recovery fixture cleanup connection: %v",err);return}
			defer func(){_ = conn.Close(context.Background())}()
			if _,err:=conn.Exec(cleanupCtx,`UPDATE mamony_identity.password_recovery_deliveries d
				SET status='DEAD',lease_id=NULL,leased_until=NULL
				WHERE status='PENDING' AND EXISTS (
					SELECT 1 FROM mamony_identity.password_recovery_challenges c
					 WHERE c.challenge_id=d.challenge_id AND c.subject_id=$1)`,
				f.subject);err!=nil{t.Errorf("recovery fixture cleanup: %v",err)}
			if _,err:=conn.Exec(cleanupCtx,`UPDATE mamony_identity.password_recovery_notices
				SET status='DEAD',lease_id=NULL,leased_until=NULL
				WHERE status='PENDING' AND subject_id=$1`,f.subject);err!=nil{
				t.Errorf("recovery notice cleanup: %v",err)
			}
		})
		return f
	}
	enroll:=func(f fixture)usecase.OfflineRecoveryCode{
		t.Helper()
		code,err:=usecase.NewOfflineRecoveryCode()
		if err!=nil{t.Fatal(err)}
		// Privileged disposable fixture ONLY. No normal runtime may insert
		// this digest or derive a code for an existing account.
		if _,err:=admin.Exec(ctx,`INSERT INTO mamony_identity.password_recovery_offline_codes
			(code_digest,subject_id,enrolled_epoch,contact_version,enrolled_at)
			VALUES($1,$2,$3,1,clock_timestamp()-interval '1 hour')`,
			func() []byte { d := code.Digest(); return d[:] }(),f.subject,f.epoch);err!=nil{t.Fatal(err)}
		return code
	}
	issue:=func(f fixture)fixture{
		t.Helper()
		token,err:=usecase.NewPasswordRecoveryProof()
		if err!=nil{t.Fatal(err)}
		f.token=token
		f.challenge=randomSyntheticID(t,"rcv_")
		allowed,err:=issuer.Issue(ctx,usecase.RecoveryIssue{
			CanonicalEmail:f.email,ChallengeID:f.challenge,TokenDigest:token.Digest(),
			KeyID:"syntheticassurance",SealedToken:bytes.Repeat([]byte{0x63},71),
		})
		if err!=nil || !allowed{t.Fatalf("fixture challenge issue: %v %t",err,allowed)}
		return f
	}
	existingGrant:=func(f fixture)(string,bool){
		t.Helper()
		var bound string
		var consumed bool
		err:=admin.QueryRow(ctx,`SELECT challenge_id,consumed_at IS NOT NULL
			FROM mamony_identity.password_recovery_assurance_grants
			WHERE subject_id=$1`,f.subject).Scan(&bound,&consumed)
		if errors.Is(err,pgx.ErrNoRows){return "",false}
		if err!=nil{t.Fatal(err)}
		return bound,consumed
	}
	usedCode:=func(c usecase.OfflineRecoveryCode)bool{
		t.Helper()
		var used bool
		d:=c.Digest()
		err:=admin.QueryRow(ctx,`SELECT consumed_at IS NOT NULL
			FROM mamony_identity.password_recovery_offline_codes WHERE code_digest=$1`,
			d[:]).Scan(&used)
		if err!=nil{t.Fatal(err)}
		return used
	}
	markEarlier:=func(f fixture){
		t.Helper()
		if _,err:=admin.Exec(ctx,`UPDATE mamony_identity.password_recovery_challenges
			SET requested_at=clock_timestamp()-interval '65 seconds'
			WHERE challenge_id=$1`,f.challenge);err!=nil{t.Fatal(err)}
	}

	f:=makeSubject()
	offline1:=enroll(f)
	offline2:=enroll(f)
	f=issue(f)
	if err:=assurer.Approve(ctx,f.token.ForTrustedDelivery(),"bad");
		!errors.Is(err,usecase.ErrRecoveryAssuranceDenied){t.Fatal("invalid code approved")}
	wrongEmail,err:=usecase.NewPasswordRecoveryProof()
	if err!=nil{t.Fatal(err)}
	if err:=assurer.Approve(ctx,wrongEmail.ForTrustedDelivery(),offline1.TrustedOfflineDisplay());
		!errors.Is(err,usecase.ErrRecoveryAssuranceDenied){t.Fatal("unknown email proof authorized offline code")}
	if usedCode(offline1){t.Fatal("invalid email bearer burned unrelated offline code")}
	if err:=reset.Redeem(ctx,f.token.ForTrustedDelivery(),f.email,"synthetic new password after offline verification");
		!errors.Is(err,usecase.ErrPasswordRecoveryDenied){t.Fatal("email-only recovery bypassed assurance")}
	if err:=assurer.Approve(ctx,f.token.ForTrustedDelivery(),offline1.TrustedOfflineDisplay());err!=nil{
		t.Fatalf("two valid independent factors were denied: %v",err)
	}
	bound,consumed:=existingGrant(f)
	if bound!=f.challenge || consumed || !usedCode(offline1) {
		t.Fatal("offline code not atomically consumed into challenge-bound assurance")
	}
	if err:=assurer.Approve(ctx,f.token.ForTrustedDelivery(),offline1.TrustedOfflineDisplay());
		!errors.Is(err,usecase.ErrRecoveryAssuranceDenied){t.Fatal("offline code replay was accepted")}
	if err:=assurer.Approve(ctx,f.token.ForTrustedDelivery(),offline2.TrustedOfflineDisplay());
		!errors.Is(err,usecase.ErrRecoveryAssuranceDenied) || usedCode(offline2){
		t.Fatal("active same-challenge grant burned another offline code")
	}
	// Issue a newer email recovery challenge; the old approval must NOT
	// authorize it. This closes the v21 subject+epoch-only grant race.
	markEarlier(f)
	previous:=f
	f=issue(f)
	if err:=reset.Redeem(ctx,f.token.ForTrustedDelivery(),f.email,"synthetic new password for later challenge");
		!errors.Is(err,usecase.ErrPasswordRecoveryDenied){
		t.Fatal("old grant was reused to redeem a new recovery challenge")
	}
	if err:=assurer.Approve(ctx,previous.token.ForTrustedDelivery(),offline2.TrustedOfflineDisplay());
		!errors.Is(err,usecase.ErrRecoveryAssuranceDenied) || usedCode(offline2){
		t.Fatal("revoked old email proof burned a saved offline code")
	}
	// Early audit collision must roll back a new code consumption and
	// replacement grant, even if both proofs were valid.
	auditCollision:=randomSyntheticID(t,"evt_")
	if _,err:=admin.Exec(ctx,`INSERT INTO mamony_identity.security_outbox
		(event_id,subject_id,event_type,occurred_at)
		VALUES($1,$2,'ENROLLMENT_PENDING',clock_timestamp())`,
		auditCollision,f.subject);err!=nil{t.Fatal(err)}
	cd:=offline2.Digest()
	if accepted,err:=store.Approve(ctx,f.token.Digest(),cd,auditCollision);
		err==nil || accepted || usedCode(offline2){t.Fatal("failed audit event did not roll back saved code")}
	bound,_=existingGrant(f)
	if bound!=previous.challenge{t.Fatal("audit collision replaced an earlier grant")}
	if err:=assurer.Approve(ctx,f.token.ForTrustedDelivery(),offline2.TrustedOfflineDisplay());err!=nil{
		t.Fatalf("retry with distinct audit ID could not approve new challenge: %v",err)
	}
	bound,_=existingGrant(f)
	if bound!=f.challenge || !usedCode(offline2){t.Fatal("grant did not bind second email challenge")}
	var auditCount int
	if err:=admin.QueryRow(ctx,`SELECT count(*) FROM mamony_identity.security_outbox
		WHERE subject_id=$1 AND event_type='RECOVERY_ASSURANCE_APPROVED'`,
		f.subject).Scan(&auditCount);err!=nil || auditCount!=2{
		t.Fatal("one immutable event was not emitted per consumed recovery code")
	}
	if err:=reset.Redeem(ctx,f.token.ForTrustedDelivery(),f.email,"synthetic new password for later challenge");err!=nil{
		t.Fatalf("properly bound offline-assisted recovery rejected: %v",err)
	}
	bound,consumed=existingGrant(f)
	if bound!=f.challenge || !consumed {
		t.Fatal("v21 redemption did not consume the v24 challenge-bound grant")
	}
	if err:=reset.Redeem(ctx,f.token.ForTrustedDelivery(),f.email,"synthetic repeated recovery password");
		!errors.Is(err,usecase.ErrPasswordRecoveryDenied){t.Fatal("replayed full recovery was not denied")}

	// An offline code for a different account cannot approve this subject.
	other:=makeSubject()
	otherCode:=enroll(other)
	other=issue(other)
	if err:=assurer.Approve(ctx,other.token.ForTrustedDelivery(),offline1.TrustedOfflineDisplay());
		!errors.Is(err,usecase.ErrRecoveryAssuranceDenied){t.Fatal("cross-account code authorized recovery")}
	if err:=assurer.Approve(ctx,other.token.ForTrustedDelivery(),otherCode.TrustedOfflineDisplay());err!=nil{
		t.Fatalf("other verified account could not approve own code: %v",err)
	}

	// Concurrent approvers on the SAME code and bearer: exactly one wins.
	raceAccount:=makeSubject()
	raceCode:=enroll(raceAccount)
	raceAccount=issue(raceAccount)
	digest:=raceAccount.token.Digest()
	backupDigest:=raceCode.Digest()
	events:=[]string{randomSyntheticID(t,"evt_"),randomSyntheticID(t,"evt_")}
	results:=make(chan bool,2)
	var wg sync.WaitGroup
	for _,event:=range events{
		wg.Add(1)
		go func(eventID string){
			defer wg.Done()
			allowed,err:=store.Approve(ctx,digest,backupDigest,eventID)
			results<-allowed && err==nil
		}(event)
	}
	wg.Wait()
	close(results)
	winners:=0
	for ok:=range results{if ok{winners++}}
	if winners!=1 || !usedCode(raceCode) {
		t.Fatal("concurrent approval granted one offline code multiple times")
	}
	// Subject/contact epoch change invalidates already-issued challenge and
	// pre-enrolled offline code; no stale grant should be minted.
	stale:=makeSubject()
	staleCode:=enroll(stale)
	stale=issue(stale)
	if _,err:=admin.Exec(ctx,`UPDATE mamony_identity.email_aliases
		SET verified_at=clock_timestamp()+interval '1 second'
		WHERE subject_id=$1`,stale.subject);err!=nil{t.Fatal(err)}
	if err:=assurer.Approve(ctx,stale.token.ForTrustedDelivery(),staleCode.TrustedOfflineDisplay());
		!errors.Is(err,usecase.ErrRecoveryAssuranceDenied) || usedCode(staleCode) {
		t.Fatal("stale subject epoch was permitted to approve recovery")
	}
}
