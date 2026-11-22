// Command mamony-identity-security-notice-worker is an explicit, isolated
// Mailgun security-alert process. It is never started by the HTTP service.
package main

import (
	"context"
	"errors"
	"io"
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

var errStartup = errors.New("identity security notice worker startup blocked")

const (
	realNoticeFlag = "--send-live-security-mailgun"
	noticeApproval = "I_ACKNOWLEDGE_REAL_SECURITY_NOTICE_EMAIL_SENDS"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := readRuntimeConfig(os.Getenv, os.Args[1:])
	if err != nil {
		logger.Error("identity security notice worker startup blocked")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Info("identity security notice worker started")
	if err := identity.RunSecurityNoticeDelivery(ctx, cfg); err != nil {
		// No source errors may reach logs: they may include provider secrets,
		// verification details, recipient addresses or DSNs.
		logger.Error("identity security notice worker stopped unexpectedly")
		os.Exit(1)
	}
	logger.Info("identity security notice worker stopped")
}

func readRuntimeConfig(getenv func(string) string, args []string) (identity.SecurityNoticeDeliveryConfig, error) {
	var empty identity.SecurityNoticeDeliveryConfig
	// Two independent, exact opt-ins plus a non-CI process: no magic defaults.
	if getenv == nil || len(args) != 1 || args[0] != realNoticeFlag ||
		getenv("MAMONY_IDENTITY_SECURITY_NOTICE_LIVE_APPROVAL") != noticeApproval ||
		getenv("CI") != "" {
		return empty, errStartup
	}
	raw, err := readPrivateFile(getenv("MAMONY_IDENTITY_SECURITY_NOTICE_DSN_FILE"), 2048)
	if err != nil {
		return empty, errStartup
	}
	dsn := strings.TrimSuffix(string(raw), "\n")
	u, err := url.Parse(dsn)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") ||
		u.User == nil || u.User.Username() != "mamony_identity_security_notice_runtime" ||
		u.Hostname() == "" || u.Port() == "" || u.Fragment != "" ||
		u.Opaque != "" || u.Path == "" || u.Path == "/" ||
		strings.Contains(strings.TrimPrefix(u.Path, "/"), "/") ||
		strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "mamony_test_") ||
		u.Query().Get("sslmode") != "verify-full" || len(u.Query()) != 1 ||
		strings.ContainsAny(dsn, "\r\n") {
		return empty, errStartup
	}
	if _, ok := u.User.Password(); !ok {
		return empty, errStartup
	}
	material, err := readPrivateFile(getenv("MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_KEY_FILE"), 512)
	if err != nil {
		return empty, errStartup
	}
	key := strings.TrimSuffix(string(material), "\n")
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return empty, errStartup
	}
	return identity.SecurityNoticeDeliveryConfig{
		DSN: dsn,
		Region: getenv("MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_REGION"),
		Domain: getenv("MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_DOMAIN"),
		From: getenv("MAMONY_IDENTITY_SECURITY_NOTICE_MAIL_FROM"),
		MailgunKey: key,
		PollInterval: 5 * time.Second,
	}, nil
}

func readPrivateFile(path string, limit int64) ([]byte, error) {
	if path == "" || !filepath.IsAbs(path) || limit < 1 {
		return nil, errStartup
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() ||
		before.Mode().Perm()&0077 != 0 || before.Mode().Perm()&0400 == 0 {
		return nil, errStartup
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, errStartup
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(filepath.Base(path))
	if err != nil {
		return nil, errStartup
	}
	defer func() { _ = f.Close() }()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		return nil, errStartup
	}
	contents, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || len(contents) == 0 || int64(len(contents)) > limit {
		return nil, errStartup
	}
	return contents, nil
}
