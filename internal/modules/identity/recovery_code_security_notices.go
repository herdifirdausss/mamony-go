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

var ErrRecoveryCodeSecurityNoticeRuntime = errors.New("recovery code notice runtime unavailable")

// RecoveryCodeSecurityNoticeConfig is trusted-only and fully opt-in. No
// server, scheduler or public Identity HTTP handler invokes this worker.
type RecoveryCodeSecurityNoticeConfig struct {
	DSN          string
	Region       string
	Domain       string
	From         string
	MailgunKey   string
	PollInterval time.Duration
	HTTPClient   *http.Client
}

// RunRecoveryCodeSecurityNotices never starts on app bootstrap or CI unless
// explicitly invoked with a dedicated sender DB role and trusted Mailgun
// configuration. A provider 200 does NOT mean the email was delivered.
func RunRecoveryCodeSecurityNotices(ctx context.Context, cfg RecoveryCodeSecurityNoticeConfig) error {
	if ctx == nil || ctx.Err() != nil ||
		cfg.PollInterval < usecase.MinMailPollInterval ||
		cfg.PollInterval > usecase.MaxMailPollInterval {
		return ErrRecoveryCodeSecurityNoticeRuntime
	}
	sender, err := mailgun.NewRecoveryCodeSecurityNoticeSender(mailgun.SecurityNoticeConfig{
		Region: mailgun.Region(cfg.Region),
		Domain: cfg.Domain, From: cfg.From, APIKey: cfg.MailgunKey,
	}, cfg.HTTPClient)
	if err != nil {
		return ErrRecoveryCodeSecurityNoticeRuntime
	}
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: cfg.DSN, RuntimeRole: "mamony_identity_recovery_code_notice_runtime", MaxConns: 2,
	})
	if err != nil {
		return ErrRecoveryCodeSecurityNoticeRuntime
	}
	defer pool.Close()
	store, err := identitypg.NewRecoveryCodeSecurityNoticeStore(pool)
	if err != nil {
		return ErrRecoveryCodeSecurityNoticeRuntime
	}
	var storePort usecase.RecoveryCodeSecurityNoticeStore = store
	var senderPort usecase.RecoveryCodeSecurityNoticeSender = sender
	worker, err := usecase.NewRecoveryCodeSecurityNoticeWorker(storePort, senderPort)
	if err != nil {
		return ErrRecoveryCodeSecurityNoticeRuntime
	}
	runner, err := usecase.NewDeliveryRunner(worker, cfg.PollInterval)
	if err != nil {
		return ErrRecoveryCodeSecurityNoticeRuntime
	}
	if err := runner.Run(ctx); err != nil {
		return ErrRecoveryCodeSecurityNoticeRuntime
	}
	return nil
}

// ReadRecoveryCodeSecurityNoticeHealth is count-only and uses an entirely
// distinct monitor credential with no SELECT privilege on user or outbox rows.
func ReadRecoveryCodeSecurityNoticeHealth(ctx context.Context, dsn string) (identitypg.RecoveryCodeSecurityNoticeHealth, error) {
	var zero identitypg.RecoveryCodeSecurityNoticeHealth
	if ctx == nil || ctx.Err() != nil {
		return zero, ErrRecoveryCodeSecurityNoticeRuntime
	}
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: dsn, RuntimeRole: "mamony_identity_recovery_code_notice_monitor_runtime", MaxConns: 1,
	})
	if err != nil {
		return zero, ErrRecoveryCodeSecurityNoticeRuntime
	}
	defer pool.Close()
	store, err := identitypg.NewRecoveryCodeSecurityNoticeHealthStore(pool)
	if err != nil {
		return zero, ErrRecoveryCodeSecurityNoticeRuntime
	}
	health, err := store.Read(ctx)
	if err != nil {
		return zero, ErrRecoveryCodeSecurityNoticeRuntime
	}
	return health, nil
}
