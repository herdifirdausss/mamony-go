package identity

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailgun"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
)

var ErrRecoverySecurityNoticeRuntime = errors.New("recovery security notice runtime unavailable")

// RecoverySecurityNoticeConfig is trusted-only. No process, scheduler or
// HTTP route starts this worker, nor does it issue recovery approvals.
type RecoverySecurityNoticeConfig struct {
	DSN          string
	Region       string
	Domain       string
	From         string
	MailgunKey   string
	PollInterval time.Duration
	HTTPClient   *http.Client
}

func RunRecoverySecurityNotices(ctx context.Context, cfg RecoverySecurityNoticeConfig) error {
	if ctx == nil || ctx.Err() != nil ||
		cfg.PollInterval < usecase.MinMailPollInterval ||
		cfg.PollInterval > usecase.MaxMailPollInterval {
		return ErrRecoverySecurityNoticeRuntime
	}
	sender, err := mailgun.NewRecoverySecurityNoticeSender(mailgun.SecurityNoticeConfig{
		Region: mailgun.Region(cfg.Region),
		Domain: cfg.Domain, From: cfg.From, APIKey: cfg.MailgunKey,
	}, cfg.HTTPClient)
	if err != nil {
		return ErrRecoverySecurityNoticeRuntime
	}
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: cfg.DSN, RuntimeRole: "mamony_identity_recovery_notice_runtime", MaxConns: 2,
	})
	if err != nil {
		return ErrRecoverySecurityNoticeRuntime
	}
	defer pool.Close()
	return nil
}

// ReadRecoverySecurityNoticeHealth exposes counts only, not account,
// recipient, provider ID or event identity. Uses a DIFFERENT DB credential
// from the sender and from the existing password-change monitor.
func ReadRecoverySecurityNoticeHealth(ctx context.Context,dsn string) (identitypg.RecoverySecurityNoticeHealth,error) {
	var zero identitypg.RecoverySecurityNoticeHealth
	if ctx==nil || ctx.Err()!=nil {return zero,ErrRecoverySecurityNoticeRuntime}
	pool,err:=platformpg.Open(ctx,platformpg.Config{
		DSN:dsn,RuntimeRole:"mamony_identity_recovery_notice_monitor_runtime",MaxConns:1,
	})
	if err!=nil {return zero,ErrRecoverySecurityNoticeRuntime}
	defer pool.Close()
	store,err:=identitypg.NewRecoverySecurityNoticeHealthStore(pool)
	if err!=nil {return zero,ErrRecoverySecurityNoticeRuntime}
	health,err:=store.Read(ctx)
	if err!=nil {return zero,ErrRecoverySecurityNoticeRuntime}
	return health,nil
}
