// Command mamony-identity-security-notice-worker is an explicit, isolated
// Mailgun security-alert process. It is never started by the HTTP service.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := readRuntimeConfig(os.Getenv, os.Args[1:])
	if err != nil {
		logger.Error("identity security notice worker startup blocked")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Info("identity security notice worker started")
	if err := identity.RunSecurityNoticeDelivery(ctx, cfg); err != nil {
		// No source errors may reach logs: they may include provider secrets,
		// verification details, recipient addresses or DSNs.
		logger.Error("identity security notice worker stopped unexpectedly")
		os.Exit(1)
	}
	logger.Info("identity security notice worker stopped")
}
