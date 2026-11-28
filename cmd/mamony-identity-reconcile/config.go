package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/herdifirdausss/mamony-go/cmd/internal/secretfile"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

var errUnsafeReconcileConfig = errors.New("identity reconciliation command blocked")

const (
	inspectFlag    = "--inspect"
	repairFlag     = "--repair-once"
	repairApproval = "I_APPROVE_BOUNDED_IDENTITY_FIRST_PROOF_REPAIR"
)

type commandConfig struct {
	mode string
	dsn  string
	keys identity.EnrollmentReconciliationConfig
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
	if maxBytes < 1 {
		return nil, errUnsafeReconcileConfig
	}
	f, err := secretfile.OpenPrivate(path)
	if err != nil {
		return nil, errUnsafeReconcileConfig
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil || len(b) == 0 || int64(len(b)) > maxBytes {
		return nil, errUnsafeReconcileConfig
	}
	return b, nil
}
