// Command mamony-identity-notice-health is a one-shot, read-only,
// thresholded operator probe. It never starts a mail worker, HTTP listener,
// monitoring scheduler, webhook endpoint or production integration.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

var errNoticeProbeConfig = errors.New("identity notice health probe configuration rejected")

const noticeProbeFlag = "--check-security-notices"

type probeLimits struct {
	PendingDue       int64
	Dead             int64
	SubmittedUnknown int64
	ProviderFailures int64
	UnmatchedEvents  int64
}

type probeConfig struct {
	dsn    string
	limits probeLimits
}

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

func main() {
	os.Exit(executeProbe(os.Getenv, os.Args[1:],
		identity.ReadSecurityNoticeHealthAsMonitor, os.Stdout))
}

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

func readProbeConfig(getenv func(string) string, args []string) (probeConfig, error) {
	var empty probeConfig
	if getenv == nil || len(args) != 1 || args[0] != noticeProbeFlag ||
		getenv("CI") != "" {
		return empty, errNoticeProbeConfig
	}
	dsn, err := readPrivateMonitorDSN(getenv("MAMONY_IDENTITY_NOTICE_MONITOR_DSN_FILE"))
	if err != nil {
		return empty, errNoticeProbeConfig
	}
	readLimit := func(name string) (int64, error) {
		raw := getenv(name)
		if raw == "" || len(raw) > 10 || strings.Trim(raw, "0123456789") != "" {
			return 0, errNoticeProbeConfig
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 || n > 1000000000 {
			return 0, errNoticeProbeConfig
		}
		return n, nil
	}
	var limits probeLimits
	values := []struct {
		name string
		dst  *int64
	}{
		{"MAMONY_NOTICE_MAX_PENDING_DUE", &limits.PendingDue},
		{"MAMONY_NOTICE_MAX_DEAD", &limits.Dead},
		{"MAMONY_NOTICE_MAX_SUBMITTED_UNKNOWN", &limits.SubmittedUnknown},
		{"MAMONY_NOTICE_MAX_PROVIDER_FAILURES", &limits.ProviderFailures},
		{"MAMONY_NOTICE_MAX_UNMATCHED_EVENTS", &limits.UnmatchedEvents},
	}
	for _, field := range values {
		*field.dst, err = readLimit(field.name)
		if err != nil {
			return empty, errNoticeProbeConfig
		}
	}
	return probeConfig{dsn: dsn, limits: limits}, nil
}

func readPrivateMonitorDSN(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", errNoticeProbeConfig
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() ||
		before.Mode().Perm()&0077 != 0 || before.Mode().Perm()&0400 == 0 {
		return "", errNoticeProbeConfig
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return "", errNoticeProbeConfig
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(filepath.Base(path))
	if err != nil {
		return "", errNoticeProbeConfig
	}
	defer func() { _ = file.Close() }()
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || after.Size() < 1 || after.Size() > 2048 {
		return "", errNoticeProbeConfig
	}
	contents, err := io.ReadAll(io.LimitReader(file, 2049))
	if err != nil || len(contents) == 0 || len(contents) > 2048 {
		return "", errNoticeProbeConfig
	}
	dsn := strings.TrimSuffix(string(contents), "\n")
	u, err := url.Parse(dsn)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") ||
		u.User == nil || u.User.Username() != "mamony_identity_notice_monitor_runtime" ||
		u.Hostname() == "" || u.Port() == "" || u.Opaque != "" || u.Fragment != "" ||
		u.Path == "" || u.Path == "/" ||
		strings.Contains(strings.TrimPrefix(u.Path, "/"), "/") ||
		strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "mamony_test_") ||
		u.Query().Get("sslmode") != "verify-full" || len(u.Query()) != 1 ||
		strings.ContainsAny(dsn, "\r\n") {
		return "", errNoticeProbeConfig
	}
	secret, ok := u.User.Password()
	if !ok || secret == "" {
		return "", errNoticeProbeConfig
	}
	return dsn, nil
}
