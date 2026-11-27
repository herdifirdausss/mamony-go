package main

import (
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/herdifirdausss/mamony-go/cmd/internal/secretfile"
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
	file, err := secretfile.OpenPrivate(path)
	if err != nil {
		return "", errNoticeProbeConfig
	}
	defer func() { _ = file.Close() }()
	after, err := file.Stat()
	if err != nil || after.Size() < 1 || after.Size() > 2048 {
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
