package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
)

func probeFixture(t *testing.T) map[string]string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notice-monitor-dsn")
	dsn := "postgres://mamony_identity_notice_monitor_runtime:synthetic@db.example.test:5432/mamony_staging?sslmode=verify-full"
	if err := os.WriteFile(path, []byte(dsn+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"MAMONY_IDENTITY_NOTICE_MONITOR_DSN_FILE": path,
		"MAMONY_NOTICE_MAX_PENDING_DUE":           "2",
		"MAMONY_NOTICE_MAX_DEAD":                  "0",
		"MAMONY_NOTICE_MAX_SUBMITTED_UNKNOWN":     "10",
		"MAMONY_NOTICE_MAX_PROVIDER_FAILURES":     "0",
		"MAMONY_NOTICE_MAX_UNMATCHED_EVENTS":      "0",
	}
}

func probeEnv(settings map[string]string) func(string) string {
	return func(name string) string { return settings[name] }
}

func TestNoticeHealthProbeThresholdClassification(t *testing.T) {
	tests := []struct {
		label    string
		snapshot identity.SecurityNoticeHealthSnapshot
		exit     int
		status   string
	}{
		{
			label: "no alert at explicit limits",
			snapshot: identity.SecurityNoticeHealthSnapshot{
				PendingDue: 2, SubmittedUnknown: 10, Delivered: 100,
			},
			exit: 0, status: "OK",
		},
		{
			label: "due backlog", snapshot: identity.SecurityNoticeHealthSnapshot{
				PendingDue: 3,
			},
			exit: 2, status: "ALERT",
		},
		{
			label: "dead notice", snapshot: identity.SecurityNoticeHealthSnapshot{
				Dead: 1,
			},
			exit: 2, status: "ALERT",
		},
		{
			label: "unconfirmed provider", snapshot: identity.SecurityNoticeHealthSnapshot{
				SubmittedUnknown: 11,
			},
			exit: 2, status: "ALERT",
		},
		{
			label: "provider failure", snapshot: identity.SecurityNoticeHealthSnapshot{
				ProviderFailures: 1,
			},
			exit: 2, status: "ALERT",
		},
		{
			label: "unmatched callback", snapshot: identity.SecurityNoticeHealthSnapshot{
				UnmatchedEvents: 1,
			},
			exit: 2, status: "ALERT",
		},
	}
	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			settings := probeFixture(t)
			var output bytes.Buffer
			result := executeProbe(probeEnv(settings), []string{noticeProbeFlag},
				func(_ context.Context, dsn string) (identity.SecurityNoticeHealthSnapshot, error) {
					if !strings.Contains(dsn, "mamony_identity_notice_monitor_runtime") {
						t.Fatal("monitor role not pinned to dedicated credential")
					}
					return tc.snapshot, nil
				}, &output)
			if result != tc.exit || !strings.Contains(output.String(), `"status":"`+tc.status+`"`) {
				t.Fatalf("wrong probe severity or exit: %d %q", result, output.String())
			}
			if strings.Contains(output.String(), "postgres://") ||
				strings.Contains(output.String(), "synthetic@") {
				t.Fatal("monitor leaked database credential")
			}
		})
	}
}

func TestNoticeHealthProbeFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testing.T, map[string]string)
		args   []string
	}{
		{name: "no explicit flag", args: nil},
		{name: "wrong argument", args: []string{"--help"}},
		{name: "CI must reject live database input", change: func(_ *testing.T, env map[string]string) {
			env["CI"] = "true"
		}},
		{name: "missing due threshold", change: func(_ *testing.T, env map[string]string) {
			delete(env, "MAMONY_NOTICE_MAX_PENDING_DUE")
		}},
		{name: "negative threshold", change: func(_ *testing.T, env map[string]string) {
			env["MAMONY_NOTICE_MAX_DEAD"] = "-1"
		}},
		{name: "threshold overflow", change: func(_ *testing.T, env map[string]string) {
			env["MAMONY_NOTICE_MAX_UNMATCHED_EVENTS"] = "9999999999999999"
		}},
		{name: "wrong database role", change: func(t *testing.T, env map[string]string) {
			raw := "postgres://mamony_identity_maintenance_runtime:synthetic@db.example.test:5432/mamony_staging?sslmode=verify-full"
			if err := os.WriteFile(env["MAMONY_IDENTITY_NOTICE_MONITOR_DSN_FILE"], []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "test database refused", change: func(t *testing.T, env map[string]string) {
			raw := "postgres://mamony_identity_notice_monitor_runtime:synthetic@db.example.test:5432/mamony_test_ci?sslmode=verify-full"
			if err := os.WriteFile(env["MAMONY_IDENTITY_NOTICE_MONITOR_DSN_FILE"], []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "TLS verification mandatory", change: func(t *testing.T, env map[string]string) {
			raw := "postgres://mamony_identity_notice_monitor_runtime:synthetic@db.example.test:5432/mamony_staging?sslmode=disable"
			if err := os.WriteFile(env["MAMONY_IDENTITY_NOTICE_MONITOR_DSN_FILE"], []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "world-readable credential", change: func(t *testing.T, env map[string]string) {
			// #nosec G302 -- intentionally unsafe file permission fixture.
			if err := os.Chmod(env["MAMONY_IDENTITY_NOTICE_MONITOR_DSN_FILE"], 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlink credential", change: func(t *testing.T, env map[string]string) {
			symlink := filepath.Join(t.TempDir(), "notice-monitor-link")
			if err := os.Symlink(env["MAMONY_IDENTITY_NOTICE_MONITOR_DSN_FILE"], symlink); err != nil {
				t.Fatal(err)
			}
			env["MAMONY_IDENTITY_NOTICE_MONITOR_DSN_FILE"] = symlink
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := probeFixture(t)
			if tc.change != nil {
				tc.change(t, settings)
			}
			args := tc.args
			if args == nil && tc.name != "no explicit flag" {
				args = []string{noticeProbeFlag}
			}
			var output bytes.Buffer
			called := false
			exit := executeProbe(probeEnv(settings), args,
				func(context.Context, string) (identity.SecurityNoticeHealthSnapshot, error) {
					called = true
					return identity.SecurityNoticeHealthSnapshot{}, nil
				}, &output)
			if exit != 1 || called || output.String() != "{\"status\":\"ERROR\",\"pending_due\":0,\"dead\":0,\"submitted_unknown\":0,\"delivered\":0,\"provider_failures\":0,\"unmatched_events\":0}\n" {
				t.Fatalf("unsafe config was accepted or leaked info: exit=%d called=%v out=%q",
					exit, called, output.String())
			}
		})
	}
}

func TestNoticeHealthProbeRedactsStorageErrors(t *testing.T) {
	settings := probeFixture(t)
	var output bytes.Buffer
	exit := executeProbe(probeEnv(settings), []string{noticeProbeFlag},
		func(context.Context, string) (identity.SecurityNoticeHealthSnapshot, error) {
			return identity.SecurityNoticeHealthSnapshot{}, errors.New("secret dsn and recipient@example.test")
		}, &output)
	if exit != 1 || strings.Contains(output.String(), "secret") ||
		strings.Contains(output.String(), "@example.test") ||
		!strings.Contains(output.String(), `"status":"ERROR"`) {
		t.Fatalf("provider storage failure leaked an error or did not fail closed: %d %q",
			exit, output.String())
	}
}
