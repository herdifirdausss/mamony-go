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

// ErrSecurityNoticeRuntime avoids revealing a DSN, sending key, recipient,
// provider response or database query across this capability boundary.
var ErrSecurityNoticeRuntime = errors.New("identity security notice runtime unavailable")

// SecurityNoticeDeliveryConfig is trusted deployment configuration, NEVER
// request input. HTTPClient is injected only by tests using local mock servers;
// the explicit production worker always leaves it nil for standard TLS.
type SecurityNoticeDeliveryConfig struct {
	DSN          string
	Region       string
	Domain       string
	From         string
	MailgunKey   string
	PollInterval time.Duration
	HTTPClient   *http.Client
}

// RunSecurityNoticeDelivery is a separate, opt-in worker. It cannot process
// verification bearer tokens or mount HTTP routes. Only the new SQL notice
// runtime may claim/finalize password-change notifications.
func RunSecurityNoticeDelivery(ctx context.Context, cfg SecurityNoticeDeliveryConfig) error {
	if ctx == nil || ctx.Err() != nil ||
		cfg.PollInterval < usecase.MinMailPollInterval ||
		cfg.PollInterval > usecase.MaxMailPollInterval {
		return ErrSecurityNoticeRuntime
	}
	sender, err := mailgun.NewSecurityNoticeSender(mailgun.SecurityNoticeConfig{
		Region: mailgun.Region(cfg.Region),
		Domain: cfg.Domain,
		From:   cfg.From,
		APIKey: cfg.MailgunKey,
	}, cfg.HTTPClient)
	if err != nil {
		return ErrSecurityNoticeRuntime
	}
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: cfg.DSN, RuntimeRole: "mamony_identity_security_notice_runtime",
		MaxConns: 2,
	})
	if err != nil {
		return ErrSecurityNoticeRuntime
	}
	defer pool.Close()
	store, err := identitypg.NewSecurityNoticeStore(pool)
	if err != nil {
		return ErrSecurityNoticeRuntime
	}
	var storePort usecase.SecurityNoticeStore = store
	var senderPort usecase.SecurityNoticeSender = sender
	worker, err := usecase.NewSecurityNoticeWorker(storePort, senderPort)
	if err != nil {
		return ErrSecurityNoticeRuntime
	}
	runner, err := usecase.NewDeliveryRunner(worker, cfg.PollInterval)
	if err != nil {
		return ErrSecurityNoticeRuntime
	}
	if err := runner.Run(ctx); err != nil {
		return ErrSecurityNoticeRuntime
	}
	return nil
}
