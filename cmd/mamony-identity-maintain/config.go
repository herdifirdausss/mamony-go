package main

import (
	"errors"
	"net/url"
	"strings"

	"github.com/herdifirdausss/mamony-go/cmd/internal/secretfile"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

var errMaintenanceApproval = errors.New("identity maintenance startup blocked")

// Approval only makes an opt-in action possible; it does NOT authenticate the
// operator or replace deployment IAM, audit/change approval or backups.
const maintenanceApproval = "I_ACKNOWLEDGE_IDENTITY_RETENTION_PURGE"

func readConfig(getenv func(string) string, args []string) (identity.RetentionConfig, error) {
	var empty identity.RetentionConfig
	if getenv == nil || len(args) != 1 || args[0] != "--purge-expired" ||
		getenv("MAMONY_IDENTITY_MAINTENANCE_APPROVAL") != maintenanceApproval ||
		getenv("CI") != "" {
		return empty, errMaintenanceApproval
	}
	b, err := secretfile.ReadPrivateLimited(getenv("MAMONY_IDENTITY_MAINTENANCE_DSN_FILE"), 2048)
	if err != nil {
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
