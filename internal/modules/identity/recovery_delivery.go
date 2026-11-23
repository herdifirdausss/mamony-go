package identity

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailcrypto"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailgun"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
)

// ErrRecoveryDeliveryRuntime never wraps DSN, API key, email or token errors.
var ErrRecoveryDeliveryRuntime = errors.New("identity recovery delivery runtime unavailable")

// RecoveryDeliveryConfig is injection-only trusted code configuration.
// No CLI, HTTP endpoint, cron, startup hook or provider credentials are
// introduced by this candidate. Production must independently provision
// restricted DB and distinct recovery-purpose AEAD/secrets before opting in.
type RecoveryDeliveryConfig struct {
	DSN          string
	Region       string
	Domain       string
	From         string
	MailgunKey   string
	RecoveryURL  string
	ActiveKeyID  string
	EnvelopeKeys map[string][]byte
	PollInterval time.Duration
	HTTPClient   *http.Client
}

// RunRecoveryDelivery can only be invoked explicitly by trusted code.
// No command or app listener invokes it in this Draft PR.
func RunRecoveryDelivery(ctx context.Context, cfg RecoveryDeliveryConfig) error {
	if ctx == nil || ctx.Err() != nil ||
		cfg.PollInterval < usecase.MinMailPollInterval ||
		cfg.PollInterval > usecase.MaxMailPollInterval {
		return ErrRecoveryDeliveryRuntime
	}
	ring, err := mailcrypto.NewRecoveryKeyring(cfg.ActiveKeyID, cfg.EnvelopeKeys)
	if err != nil {
		return ErrRecoveryDeliveryRuntime
	}
	sender, err := mailgun.NewRecoveryMailSender(mailgun.RecoveryConfig{
		Region: mailgun.Region(cfg.Region), Domain: cfg.Domain,
		From: cfg.From, APIKey: cfg.MailgunKey, RecoveryURL: cfg.RecoveryURL,
	}, cfg.HTTPClient)
	if err != nil {
		return ErrRecoveryDeliveryRuntime
	}
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: cfg.DSN, RuntimeRole: "mamony_identity_recovery_delivery_runtime",
		MaxConns: 2,
	})
	if err != nil {
		return ErrRecoveryDeliveryRuntime
	}
	defer pool.Close()
	store, err := identitypg.NewRecoveryDeliveryStore(pool)
	if err != nil {
		return ErrRecoveryDeliveryRuntime
	}
	// Make dependency inversion explicit: adapters satisfy consumer-owned ports
	// at this facade boundary, never import/call the usecase directly.
	var storePort usecase.RecoveryDeliveryStore = store
	var openerPort usecase.RecoveryEnvelopeOpener = ring
	var senderPort usecase.RecoverySender = sender
	worker, err := usecase.NewRecoveryDeliveryWorker(storePort, openerPort, senderPort)
	if err != nil {
		return ErrRecoveryDeliveryRuntime
	}
	runner, err := usecase.NewDeliveryRunner(worker, cfg.PollInterval)
	if err != nil {
		return ErrRecoveryDeliveryRuntime
	}
	if err := runner.Run(ctx); err != nil {
		return ErrRecoveryDeliveryRuntime
	}
	return nil
}
