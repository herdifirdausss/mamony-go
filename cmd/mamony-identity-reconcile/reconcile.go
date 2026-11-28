package main

import (
	"context"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

type executionReport struct {
	before identity.EnrollmentBacklog
	after  identity.EnrollmentBacklog
	repair identity.EnrollmentReconciliationResult
}

type commandDeps struct {
	inspect func(context.Context, string) (identity.EnrollmentBacklog, error)
	repair  func(context.Context, identity.EnrollmentReconciliationConfig) (identity.EnrollmentReconciliationResult, error)
}

func classify(snapshot identity.EnrollmentBacklog) (string, int) {
	if snapshot.EligibleSampled > 0 || snapshot.StalePresent {
		return "ALERT", 2
	}
	return "OK", 0
}

func execute(ctx context.Context, cfg commandConfig, deps commandDeps) (executionReport, error) {
	var output executionReport
	if ctx == nil || ctx.Err() != nil || deps.inspect == nil ||
		(cfg.mode != inspectFlag && cfg.mode != repairFlag) {
		return output, errUnsafeReconcileConfig
	}
	before, err := deps.inspect(ctx, cfg.dsn)
	if err != nil {
		return output, errUnsafeReconcileConfig
	}
	output.before = before
	output.after = before
	if cfg.mode == inspectFlag {
		return output, nil
	}
	if deps.repair == nil {
		return executionReport{}, errUnsafeReconcileConfig
	}
	result, err := deps.repair(ctx, cfg.keys)
	if err != nil {
		return executionReport{}, errUnsafeReconcileConfig
	}
	output.repair = result
	after, err := deps.inspect(ctx, cfg.dsn)
	if err != nil {
		return executionReport{}, errUnsafeReconcileConfig
	}
	output.after = after
	return output, nil
}
