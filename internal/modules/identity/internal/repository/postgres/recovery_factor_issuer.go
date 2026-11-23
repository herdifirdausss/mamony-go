package postgres

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// RecoveryFactorIssuerStore may only EXECUTE the purpose-specific challenge
// issuer transaction. The SQL function consumes a FRESH recovery-code grant.
type RecoveryFactorIssuerStore struct { db Transactor }

func NewRecoveryFactorIssuerStore(db Transactor)(*RecoveryFactorIssuerStore,error){
	if db==nil{return nil,usecase.ErrRecoveryFactorUnavailable}
	return &RecoveryFactorIssuerStore{db:db},nil
}

func (s *RecoveryFactorIssuerStore) Issue(ctx context.Context,session,grant [32]byte,id string,nonce [32]byte)(bool,error){
	if s==nil || s.db==nil || ctx==nil || ctx.Err()!=nil ||
		session==([32]byte{}) || grant==([32]byte{}) || len(id)!=26 || id[:4]!="fac_" ||
		nonce==([32]byte{}) {
		return false,usecase.ErrRecoveryFactorUnavailable
	}
	var ok bool
	err:=s.db.WithTx(ctx,func(tx pgx.Tx)error{
		return tx.QueryRow(ctx,"SELECT mamony_identity.issue_recovery_code_factor_challenge($1,$2,$3,$4)",
			session[:],grant[:],id,nonce[:]).Scan(&ok)
	})
	if err!=nil{return false,usecase.ErrRecoveryFactorUnavailable}
	return ok,nil
}
