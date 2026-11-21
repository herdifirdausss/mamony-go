// Command mamony-identity-reconcile is an opt-in, bounded operator tool.
// It is not an HTTP endpoint, cron job, or email sending worker.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

var errUnsafeReconcileConfig = errors.New("identity reconciliation command blocked")

const (
	inspectFlag = "--inspect"
	repairFlag  = "--repair-once"
	repairApproval = "I_APPROVE_BOUNDED_IDENTITY_FIRST_PROOF_REPAIR"
)

type commandConfig struct {
	mode string
	dsn  string
	keys identity.EnrollmentReconciliationConfig
}

type executionReport struct {
	before identity.EnrollmentBacklog
	after  identity.EnrollmentBacklog
	repair identity.EnrollmentReconciliationResult
}

type commandDeps struct {
	inspect func(context.Context, string) (identity.EnrollmentBacklog, error)
	repair func(context.Context, identity.EnrollmentReconciliationConfig) (identity.EnrollmentReconciliationResult, error)
}

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
		repair: identity.ReconcileEnrollmentOnce,
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

// Inspect needs only a restricted verifier connection. Repair additionally
// requires a separate explicit acknowledgement plus the externally supplied
// AES-GCM keyring. Neither mode accepts a DSN or key in argv or inline env.
func readCommandConfig(getenv func(string) string, args []string) (commandConfig, error) {
	var empty commandConfig
	if getenv == nil || len(args) != 1 ||
		(args[0] != inspectFlag && args[0] != repairFlag) {
		return empty, errUnsafeReconcileConfig
	}
	dsnBytes, err := readPrivateFile(getenv("MAMONY_IDENTITY_RECONCILE_DSN_FILE"), 2048)
	if err != nil {
		return empty, errUnsafeReconcileConfig
	}
	dsn := strings.TrimSuffix(string(dsnBytes), "\n")
	if !validProductionVerifierDSN(dsn) {
		return empty, errUnsafeReconcileConfig
	}
	cfg := commandConfig{mode: args[0], dsn: dsn}
	if args[0] == inspectFlag {
		return cfg, nil
	}
	if getenv("CI") != "" ||
		getenv("MAMONY_IDENTITY_RECONCILE_APPROVAL") != repairApproval {
		return empty, errUnsafeReconcileConfig
	}
	b, err := readPrivateFile(getenv("MAMONY_IDENTITY_ENVELOPE_KEYS_FILE"), 4096)
	if err != nil {
		return empty, errUnsafeReconcileConfig
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var doc struct {
		Active string            `json:"active"`
		Keys   map[string]string `json:"keys"`
	}
	if err := dec.Decode(&doc); err != nil || doc.Active == "" ||
		len(doc.Keys) < 1 || len(doc.Keys) > 8 {
		return empty, errUnsafeReconcileConfig
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return empty, errUnsafeReconcileConfig
	}
	keys := make(map[string][]byte, len(doc.Keys))
	for id, encoded := range doc.Keys {
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(key) != 32 {
			return empty, errUnsafeReconcileConfig
		}
		keys[id] = key
	}
	if _, ok := keys[doc.Active]; !ok {
		return empty, errUnsafeReconcileConfig
	}
	cfg.keys = identity.EnrollmentReconciliationConfig{
		DSN: dsn, ActiveKeyID: doc.Active, EnvelopeKeys: keys,
	}
	return cfg, nil
}

func validProductionVerifierDSN(raw string) bool {
	if raw == "" || strings.ContainsAny(raw, "\r\n") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") ||
		u.User == nil || u.User.Username() != "mamony_identity_verifier_runtime" ||
		u.Hostname() == "" || u.Fragment != "" || u.Path == "" ||
		strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "mamony_test_") ||
		strings.Contains(strings.TrimPrefix(u.Path, "/"), "/") ||
		len(u.Query()) != 1 || len(u.Query()["sslmode"]) != 1 ||
		u.Query().Get("sslmode") != "verify-full" {
		return false
	}
	p, err := strconv.Atoi(u.Port())
	return err == nil && p > 0 && p <= 65535
}

// Refuses symlinks, group/world-readable files, oversized data and file
// replacement while opening. Never log secret path contents or error chains.
func readPrivateFile(path string, maxBytes int64) ([]byte, error) {
	if path == "" || !filepath.IsAbs(path) || maxBytes < 1 {
		return nil, errUnsafeReconcileConfig
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() ||
		before.Mode().Perm()&0077 != 0 || before.Mode().Perm()&0400 == 0 {
		return nil, errUnsafeReconcileConfig
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, errUnsafeReconcileConfig
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(filepath.Base(path))
	if err != nil {
		return nil, errUnsafeReconcileConfig
	}
	defer func() { _ = f.Close() }()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		return nil, errUnsafeReconcileConfig
	}
	b, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil || len(b) == 0 || int64(len(b)) > maxBytes {
		return nil, errUnsafeReconcileConfig
	}
	return b, nil
}
