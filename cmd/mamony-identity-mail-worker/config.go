package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

var errStartup = errors.New("identity mail worker startup blocked")

const (
	realMailFlag = "--send-live-mailgun"
	liveApproval = "I_ACKNOWLEDGE_REAL_MAILGUN_EMAIL_SENDS"
)

func readRuntimeConfig(getenv func(string) string, args []string) (identity.DeliveryConfig, error) {
	var empty identity.DeliveryConfig
	// No defaults, magic CI fixtures, or accidental worker launch.
	if getenv == nil || len(args) != 1 || args[0] != realMailFlag ||
		getenv("MAMONY_IDENTITY_MAIL_LIVE_APPROVAL") != liveApproval ||
		getenv("CI") != "" {
		return empty, errStartup
	}
	dsnRaw, err := readPrivateFile(getenv("MAMONY_IDENTITY_DELIVERY_DSN_FILE"), 2048)
	if err != nil {
		return empty, errStartup
	}
	dsn := strings.TrimSuffix(string(dsnRaw), "\n")
	u, err := url.Parse(dsn)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") ||
		u.User == nil || u.User.Username() != "mamony_identity_delivery_runtime" ||
		strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "mamony_test_") ||
		u.Query().Get("sslmode") != "verify-full" || len(u.Query()) != 1 ||
		strings.ContainsAny(dsn, "\r\n") {
		return empty, errStartup
	}
	mailKey, err := readPrivateFile(getenv("MAMONY_IDENTITY_MAILGUN_KEY_FILE"), 512)
	if err != nil {
		return empty, errStartup
	}
	key := strings.TrimSuffix(string(mailKey), "\n")
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return empty, errStartup
	}
	material, err := readPrivateFile(getenv("MAMONY_IDENTITY_ENVELOPE_KEYS_FILE"), 4096)
	if err != nil {
		return empty, errStartup
	}
	var doc struct {
		Active string            `json:"active"`
		Keys   map[string]string `json:"keys"`
	}
	decoder := json.NewDecoder(bytes.NewReader(material))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		return empty, errStartup
	}
	var more any
	if err := decoder.Decode(&more); !errors.Is(err, io.EOF) {
		return empty, errStartup
	}
	if doc.Active == "" || len(doc.Keys) == 0 || len(doc.Keys) > 8 {
		return empty, errStartup
	}
	keys := make(map[string][]byte, len(doc.Keys))
	for id, encoded := range doc.Keys {
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(data) != 32 {
			return empty, errStartup
		}
		keys[id] = data
	}
	if _, ok := keys[doc.Active]; !ok {
		return empty, errStartup
	}
	return identity.DeliveryConfig{
		DSN:             dsn,
		Region:          getenv("MAMONY_IDENTITY_MAILGUN_REGION"),
		Domain:          getenv("MAMONY_IDENTITY_MAILGUN_DOMAIN"),
		From:            getenv("MAMONY_IDENTITY_MAIL_FROM"),
		VerificationURL: getenv("MAMONY_IDENTITY_MAIL_VERIFY_URL"),
		MailgunKey:      key,
		ActiveKeyID:     doc.Active,
		EnvelopeKeys:    keys,
		PollInterval:    5 * time.Second,
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
	// os.Root confines file opens to the selected directory. Combined with
	// Lstat + SameFile this refuses symlinks and unexpected path replacement.
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
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || len(raw) == 0 || int64(len(raw)) > limit {
		return nil, errStartup
	}
	return raw, nil
}
