// Command mamony-identity-reconcile is an opt-in, bounded operator tool.
// It is not an HTTP endpoint, cron job, or email sending worker.
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
	cfg, err := readCommandConfig(os.Getenv, os.Args[1:])
	if err != nil {
		logger.Error("identity reconciliation configuration rejected")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	report, err := execute(ctx, cfg, commandDeps{
		inspect: identity.InspectEnrollmentBacklog,
		repair:  identity.ReconcileEnrollmentOnce,
	})
	if err != nil {
		// Do not log underlying DB/provider/crypto or filesystem errors.
		logger.Error("identity reconciliation operation failed")
		os.Exit(1)
	}
	// An ALERT is an exit code consumed by an external trusted supervisor.
	// It does NOT send notifications or install any automatic scheduling.
	status, code := classify(report.after)
	logger.Info("identity reconciliation result",
		"mode", cfg.mode, "status", status,
		"before_eligible_sampled", report.before.EligibleSampled,
		"after_eligible_sampled", report.after.EligibleSampled,
		"stale_without_first_proof", report.after.StalePresent,
		"candidates", report.repair.Candidates,
		"issued", report.repair.Issued, "skipped", report.repair.Skipped)
	os.Exit(code)
}
