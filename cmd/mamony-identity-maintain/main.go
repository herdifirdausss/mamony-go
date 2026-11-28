// Command mamony-identity-maintain performs a single operator-approved,
// bounded Identity cleanup batch. It is neither an HTTP endpoint nor cron.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := readConfig(os.Getenv, os.Args[1:])
	if err != nil {
		logger.Error("identity maintenance startup blocked")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := identity.PurgeRetention(ctx, cfg)
	if err != nil {
		// Do not log underlying errors; they may contain secret-bearing DSNs.
		logger.Error("identity maintenance failed")
		os.Exit(1)
	}
	logger.Info("identity maintenance completed", "envelopes", result.EnvelopesPurged, "counters", result.CountersPurged)
}
