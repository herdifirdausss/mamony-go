// Command mamony-identity-mail-worker is an opt-in, isolated Mailgun worker.
// It is never started by the HTTP application and has no HTTP handlers.
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
		logger.Error("identity mail worker startup blocked")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Info("identity mail worker started")
	if err := identity.RunDelivery(ctx, cfg); err != nil {
		// Never log source errors. They could contain credential-bearing URLs.
		logger.Error("identity mail worker stopped unexpectedly")
		os.Exit(1)
	}
	logger.Info("identity mail worker stopped")
}
