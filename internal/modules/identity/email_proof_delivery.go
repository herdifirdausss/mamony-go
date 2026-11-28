// Package identity exposes the Identity capability facade. No caller outside
// this capability can import its private repositories or issue proof tokens.
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

// ErrDeliveryRuntime is the sole error returned across this facade. In
// particular, it never includes a DSN, API key, email address or bearer URL.
var ErrDeliveryRuntime = errors.New("identity delivery runtime unavailable")

// DeliveryConfig is trusted deployment configuration, not HTTP input.
// The production CLI never sets HTTPClient; test wiring uses an httptest
// transport to keep all Mailgun calls local and disconnected from real email.
type DeliveryConfig struct {
	DSN             string
	Region          string
	Domain          string
	From            string
	VerificationURL string
	MailgunKey      string
	ActiveKeyID     string
	EnvelopeKeys    map[string][]byte
	PollInterval    time.Duration
	HTTPClient      *http.Client
}

// RunDelivery is an explicit, separately launched runtime. It does not
// authorize public enrollment or generate any verification challenge.
func RunDelivery(ctx context.Context, cfg DeliveryConfig) error {
	if ctx == nil || ctx.Err() != nil || cfg.PollInterval < usecase.MinMailPollInterval ||
		cfg.PollInterval > usecase.MaxMailPollInterval {
		return ErrDeliveryRuntime
	}
	ring, err := mailcrypto.NewKeyring(cfg.ActiveKeyID, cfg.EnvelopeKeys)
	if err != nil {
		return ErrDeliveryRuntime
	}
	sender, err := mailgun.NewSender(mailgun.Config{
		Region:          mailgun.Region(cfg.Region),
		Domain:          cfg.Domain,
		From:            cfg.From,
		APIKey:          cfg.MailgunKey,
		VerificationURL: cfg.VerificationURL,
	}, cfg.HTTPClient)
	if err != nil {
		return ErrDeliveryRuntime
	}
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN:         cfg.DSN,
		RuntimeRole: "mamony_identity_delivery_runtime",
		MaxConns:    2,
	})
	if err != nil {
		return ErrDeliveryRuntime
	}
	defer pool.Close()
	store, err := identitypg.NewDeliveryStore(pool)
	if err != nil {
		return ErrDeliveryRuntime
	}
	// Adapt concrete storage/crypto/provider implementations to the ports
	// explicitly before crossing the business usecase boundary.
	var storePort usecase.ProofDeliveryStore = store
	var openerPort usecase.ProofEnvelopeOpener = ring
	var senderPort usecase.EmailSender = sender
	worker, err := usecase.NewProofDeliveryWorker(storePort, openerPort, senderPort)
	if err != nil {
		return ErrDeliveryRuntime
	}
	runner, err := usecase.NewDeliveryRunner(worker, cfg.PollInterval)
	if err != nil {
		return ErrDeliveryRuntime
	}
	if err := runner.Run(ctx); err != nil {
		return ErrDeliveryRuntime
	}
	return nil
}
