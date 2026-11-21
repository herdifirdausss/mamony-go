// Command mamony-identity-maintain performs a single operator-approved,
// bounded Identity cleanup batch. It is neither an HTTP endpoint nor cron.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

var errMaintenanceApproval = errors.New("identity maintenance startup blocked")

// Approval only makes an opt-in action possible; it does NOT authenticate the
// operator or replace deployment IAM, audit/change approval or backups.
const maintenanceApproval = "I_ACKNOWLEDGE_IDENTITY_RETENTION_PURGE"

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

func readConfig(getenv func(string) string, args []string) (identity.RetentionConfig, error) {
	var empty identity.RetentionConfig
	if getenv == nil || len(args) != 1 || args[0] != "--purge-expired" ||
		getenv("MAMONY_IDENTITY_MAINTENANCE_APPROVAL") != maintenanceApproval ||
		getenv("CI") != "" {
		return empty, errMaintenanceApproval
	}
	path := getenv("MAMONY_IDENTITY_MAINTENANCE_DSN_FILE")
	if path == "" || !filepath.IsAbs(path) {
		return empty, errMaintenanceApproval
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 ||
		before.Mode().Perm()&0400 == 0 {
		return empty, errMaintenanceApproval
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return empty, errMaintenanceApproval
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(filepath.Base(path))
	if err != nil {
		return empty, errMaintenanceApproval
	}
	defer func() { _ = f.Close() }()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || after.Size() < 1 || after.Size() > 2048 {
		return empty, errMaintenanceApproval
	}
	b := make([]byte, after.Size())
	if n, err := f.Read(b); err != nil || int64(n) != after.Size() {
		return empty, errMaintenanceApproval
	}
	dsn := strings.TrimSuffix(string(b), "\n")
	u, err := url.Parse(dsn)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") ||
		u.User == nil || u.User.Username() != "mamony_identity_maintenance_runtime" ||
		strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "mamony_test_") ||
		u.Query().Get("sslmode") != "verify-full" || len(u.Query()) != 1 ||
		strings.ContainsAny(dsn, "\r\n") {
		return empty, errMaintenanceApproval
	}
	return identity.RetentionConfig{DSN: dsn}, nil
}
