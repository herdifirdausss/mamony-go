package main

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

type probeReport struct {
	Status           string `json:"status"`
	PendingDue       int64  `json:"pending_due"`
	Dead             int64  `json:"dead"`
	SubmittedUnknown int64  `json:"submitted_unknown"`
	Delivered        int64  `json:"delivered"`
	ProviderFailures int64  `json:"provider_failures"`
	UnmatchedEvents  int64  `json:"unmatched_events"`
}

type healthObserver func(context.Context, string) (identity.SecurityNoticeHealthSnapshot, error)

// executeProbe returns 0 healthy, 2 explicit threshold breach, 1 config/DB
// failure. It emits exactly one JSON line without addresses, URLs, account
// identifiers, signing material, provider message IDs or internal DB errors.
func executeProbe(getenv func(string) string, args []string,
	observe healthObserver, output io.Writer) int {
	if output == nil || observe == nil {
		return 1
	}
	cfg, err := readProbeConfig(getenv, args)
	if err != nil {
		_ = json.NewEncoder(output).Encode(probeReport{Status: "ERROR"})
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	snapshot, err := observe(ctx, cfg.dsn)
	if err != nil || ctx.Err() != nil ||
		snapshot.PendingDue < 0 || snapshot.Dead < 0 ||
		snapshot.SubmittedUnknown < 0 || snapshot.Delivered < 0 ||
		snapshot.ProviderFailures < 0 || snapshot.UnmatchedEvents < 0 {
		_ = json.NewEncoder(output).Encode(probeReport{Status: "ERROR"})
		return 1
	}
	report := probeReport{
		Status:           "OK",
		PendingDue:       snapshot.PendingDue,
		Dead:             snapshot.Dead,
		SubmittedUnknown: snapshot.SubmittedUnknown,
		Delivered:        snapshot.Delivered,
		ProviderFailures: snapshot.ProviderFailures,
		UnmatchedEvents:  snapshot.UnmatchedEvents,
	}
	exit := 0
	if snapshot.PendingDue > cfg.limits.PendingDue ||
		snapshot.Dead > cfg.limits.Dead ||
		snapshot.SubmittedUnknown > cfg.limits.SubmittedUnknown ||
		snapshot.ProviderFailures > cfg.limits.ProviderFailures ||
		snapshot.UnmatchedEvents > cfg.limits.UnmatchedEvents {
		report.Status = "ALERT"
		exit = 2
	}
	if err := json.NewEncoder(output).Encode(report); err != nil {
		return 1
	}
	return exit
}
