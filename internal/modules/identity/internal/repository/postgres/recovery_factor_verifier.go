package postgres

import (
	"context"
	"errors"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// Only trusted app verification code may use this EXECUTE-only verifier role.
// PostgreSQL deliberately never receives raw Ed25519 signatures or private
// keys. Do not expose this role as a generic SQL query service.
type RecoveryFactorVerifierStore struct { db Transactor }

func NewRecoveryFactorVerifierStore(db Transactor)(*RecoveryFactorVerifierStore,error){
	if db==nil{return nil,usecase.ErrRecoveryFactorUnavailable}
	return &RecoveryFactorVerifierStore{db:db},nil
}

func (s *RecoveryFactorVerifierStore) Read(ctx context.Context,id string)(usecase.RecoveryFactorEvidence,bool,error){
	var out usecase.RecoveryFactorEvidence
	if s==nil||s.db==nil||ctx==nil||ctx.Err()!=nil||len(id)!=26||id[:4]!="fac_"{
		return out,false,usecase.ErrRecoveryFactorUnavailable
	}
	var nonce,pub []byte
	err:=s.db.WithTx(ctx,func(tx pgx.Tx)error{
		return tx.QueryRow(ctx,"SELECT subject_id,subject_epoch,nonce,public_key FROM mamony_identity.read_recovery_code_factor_challenge($1)",
			id).Scan(&out.Subject,&out.Epoch,&nonce,&pub)
	})
	if errors.Is(err,pgx.ErrNoRows){return usecase.RecoveryFactorEvidence{},false,nil}
	if err!=nil||len(nonce)!=32||len(pub)!=32{
		return usecase.RecoveryFactorEvidence{},false,usecase.ErrRecoveryFactorUnavailable
	}
	copy(out.Nonce[:],nonce)
	copy(out.PublicKey[:],pub)
	return out,true,nil
}

func (s *RecoveryFactorVerifierStore) Fail(ctx context.Context,id string)error{
	if s==nil||s.db==nil||ctx==nil||ctx.Err()!=nil||len(id)!=26||id[:4]!="fac_"{
		return usecase.ErrRecoveryFactorUnavailable
	}
	var recorded bool
	err:=s.db.WithTx(ctx,func(tx pgx.Tx)error{
		return tx.QueryRow(ctx,"SELECT mamony_identity.fail_recovery_code_factor_challenge($1)",
			id).Scan(&recorded)
	})
	if err!=nil||!recorded{return usecase.ErrRecoveryFactorUnavailable}
	return nil
}

func (s *RecoveryFactorVerifierStore) Complete(ctx context.Context,id string,
	publicKey,digest [32]byte,event string)(bool,error){
	if s==nil||s.db==nil||ctx==nil||ctx.Err()!=nil||
		len(id)!=26||id[:4]!="fac_"||len(event)!=26||event[:4]!="evt_"||
		publicKey==([32]byte{})||digest==([32]byte{}){
		return false,usecase.ErrRecoveryFactorUnavailable
	}
	var allowed bool
	err:=s.db.WithTx(ctx,func(tx pgx.Tx)error{
		return tx.QueryRow(ctx,"SELECT mamony_identity.complete_recovery_code_factor_challenge($1,$2,$3,$4)",
			id,publicKey[:],digest[:],event).Scan(&allowed)
	})
	if err!=nil{return false,usecase.ErrRecoveryFactorUnavailable}
	return allowed,nil
}
