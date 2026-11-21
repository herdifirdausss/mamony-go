package main

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

func TestIdentityReconcileCommandConfigDeniesByDefault(t *testing.T) {
	if _, err := readCommandConfig(nil, []string{inspectFlag}); !errors.Is(err, errUnsafeReconcileConfig) {
		t.Fatal("missing configuration source accepted")
	}
	if _, err := readCommandConfig(func(string) string { return "" }, nil); !errors.Is(err, errUnsafeReconcileConfig) {
		t.Fatal("missing command accepted")
	}
	dir := t.TempDir()
	dsnPath := filepath.Join(dir, "verifier-dsn")
	keysPath := filepath.Join(dir, "envelope-keys")
	dsn := "postgres" + "://" + "mamony_identity_verifier_runtime:synthetic@db.example.test:5432/mamony?sslmode=verify-full"
	if err := os.WriteFile(dsnPath, []byte(dsn), 0600); err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	if err := os.WriteFile(keysPath, []byte(`{"active":"v1","keys":{"v1":"`+key+`"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	env := func(k string) string {
		switch k {
		case "MAMONY_IDENTITY_RECONCILE_DSN_FILE":
			return dsnPath
		case "MAMONY_IDENTITY_ENVELOPE_KEYS_FILE":
			return keysPath
		default:
			return ""
		}
	}
	inspect, err := readCommandConfig(env, []string{inspectFlag})
	if err != nil || inspect.dsn != dsn || inspect.mode != inspectFlag ||
		len(inspect.keys.EnvelopeKeys) != 0 {
		t.Fatalf("safe read-only inspect rejected or loaded write key: %v", err)
	}
	if _, err := readCommandConfig(env, []string{repairFlag}); !errors.Is(err, errUnsafeReconcileConfig) {
		t.Fatal("repair without explicit approval accepted")
	}
	approved := func(k string) string {
		if k == "MAMONY_IDENTITY_RECONCILE_APPROVAL" {
			return repairApproval
		}
		return env(k)
	}
	repair, err := readCommandConfig(approved, []string{repairFlag})
	if err != nil || len(repair.keys.EnvelopeKeys["v1"]) != 32 ||
		repair.keys.ActiveKeyID != "v1" || repair.keys.DSN != dsn {
		t.Fatalf("approved synthetic configuration rejected: %v", err)
	}
	for _, args := range [][]string{{"--inspect", "--repair-once"}, {"--other"}, {"--repair-once", "--inspect"}} {
		if _, err := readCommandConfig(approved, args); !errors.Is(err, errUnsafeReconcileConfig) {
			t.Fatal("unsafe CLI command accepted")
		}
	}
	inCI := func(k string) string {
		if k == "CI" {
			return "true"
		}
		return approved(k)
	}
	if _, err := readCommandConfig(inCI, []string{repairFlag}); !errors.Is(err, errUnsafeReconcileConfig) {
		t.Fatal("production repair approved in CI")
	}
	if err := os.Chmod(dsnPath, 0644); err != nil { // #nosec G302 -- unsafe fixture deliberately.
		t.Fatal(err)
	}
	if _, err := readCommandConfig(approved, []string{inspectFlag}); !errors.Is(err, errUnsafeReconcileConfig) {
		t.Fatal("group/world-readable secret file accepted")
	}
	if err := os.Chmod(dsnPath, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "symlink")
	if err := os.Symlink(dsnPath, link); err != nil {
		t.Fatal(err)
	}
	symlinkEnv := func(k string) string {
		if k == "MAMONY_IDENTITY_RECONCILE_DSN_FILE" {
			return link
		}
		return approved(k)
	}
	if _, err := readCommandConfig(symlinkEnv, []string{inspectFlag}); !errors.Is(err, errUnsafeReconcileConfig) {
		t.Fatal("symlink secret file accepted")
	}
	for _, bad := range []string{
		"postgres://mamony_identity_runtime:synthetic@db.example.test:5432/mamony?sslmode=verify-full",
		"postgres://mamony_identity_verifier_runtime:synthetic@db.example.test:5432/mamony_test_ci?sslmode=verify-full",
		"postgres://mamony_identity_verifier_runtime:synthetic@db.example.test:5432/mamony?sslmode=disable",
		"postgres://mamony_identity_verifier_runtime:synthetic@db.example.test:5432/mamony?sslmode=verify-full&options=x",
		strings.Repeat("x", 2050),
	} {
		if err := os.WriteFile(dsnPath, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readCommandConfig(approved, []string{inspectFlag}); !errors.Is(err, errUnsafeReconcileConfig) {
			t.Fatal("unsafe production DSN accepted")
		}
	}
	if err := os.WriteFile(dsnPath, []byte(dsn), 0600); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"active":"v1","keys":{"v1":"short"}}`,
		`{"active":"unknown","keys":{"v1":"` + key + `"}}`,
		`{"active":"v1","keys":{"v1":"` + key + `"},"unknown":true}`,
		`{"active":"v1","keys":{"v1":"` + key + `"}} {}`} {
		if err := os.WriteFile(keysPath, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readCommandConfig(approved, []string{repairFlag}); !errors.Is(err, errUnsafeReconcileConfig) {
			t.Fatal("unsafe envelope key material accepted")
		}
	}
}

func TestIdentityReconcileExecuteIsReadOnlyUnlessApproved(t *testing.T) {
	calls := 0
	repairCalls := 0
	inspect := func(context.Context, string) (identity.EnrollmentBacklog, error) {
		calls++
		if calls == 1 {
			return identity.EnrollmentBacklog{EligibleSampled: 3}, nil
		}
		return identity.EnrollmentBacklog{}, nil
	}
	repair := func(context.Context, identity.EnrollmentReconciliationConfig) (identity.EnrollmentReconciliationResult, error) {
		repairCalls++
		return identity.EnrollmentReconciliationResult{Candidates: 3, Issued: 3}, nil
	}
	cfg := commandConfig{mode: inspectFlag, dsn: "synthetic"}
	got, err := execute(context.Background(), cfg, commandDeps{inspect: inspect, repair: repair})
	if err != nil || calls != 1 || repairCalls != 0 || got.after.EligibleSampled != 3 {
		t.Fatalf("read-only inspect unexpectedly mutated state: %+v %v", got, err)
	}
	if status, exitCode := classify(got.after); status != "ALERT" || exitCode != 2 {
		t.Fatal("actionable backlog did not produce external supervisor alert")
	}
	cfg.mode = repairFlag
	got, err = execute(context.Background(), cfg, commandDeps{inspect: inspect, repair: repair})
	if err != nil || calls != 3 || repairCalls != 1 || got.repair.Issued != 3 {
		t.Fatalf("approved one-shot repair did not perform bounded sequence: %+v %v", got, err)
	}
	if status, exitCode := classify(got.after); status != "OK" || exitCode != 0 {
		t.Fatal("cleared backlog still reported alert")
	}
	if status, exitCode := classify(identity.EnrollmentBacklog{StalePresent: true}); status != "ALERT" || exitCode != 2 {
		t.Fatal("stale manual-remediation case did not alert")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := execute(ctx, cfg, commandDeps{inspect: inspect, repair: repair}); !errors.Is(err, errUnsafeReconcileConfig) {
		t.Fatal("canceled operation accepted")
	}
	if _, err := execute(context.Background(), cfg, commandDeps{inspect: inspect}); !errors.Is(err, errUnsafeReconcileConfig) {
		t.Fatal("missing repair port accepted")
	}
	if _, err := execute(context.Background(), cfg, commandDeps{
		inspect: func(context.Context, string) (identity.EnrollmentBacklog, error) {
			return identity.EnrollmentBacklog{}, errors.New("sensitive database error")
		}, repair: repair,
	}); !errors.Is(err, errUnsafeReconcileConfig) || err.Error() != errUnsafeReconcileConfig.Error() {
		t.Fatal("inspection failure leaked sensitive details")
	}
}
