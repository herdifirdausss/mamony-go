package main

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/herdifirdausss/mamony-go/cmd/internal/secretfile"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

var errStartup = errors.New("identity security notice worker startup blocked")

const (
	realNoticeFlag = "--send-live-security-mailgun"
	noticeApproval = "I_ACKNOWLEDGE_REAL_SECURITY_NOTICE_EMAIL_SENDS"
)

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
		DSN:          dsn,
		Region:       getenv("MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_REGION"),
		Domain:       getenv("MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_DOMAIN"),
		From:         getenv("MAMONY_IDENTITY_SECURITY_NOTICE_MAIL_FROM"),
		MailgunKey:   key,
		PollInterval: 5 * time.Second,
	}, nil
}

func readPrivateFile(path string, limit int64) ([]byte, error) {
	raw, err := secretfile.ReadPrivateLimited(path, limit)
	if err != nil {
		return nil, errStartup
	}
	return raw, nil
}
