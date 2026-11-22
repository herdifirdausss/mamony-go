package main

import (
	"bytes"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func syntheticNoticeDSN(role, host, database, sslmode string) string {
	return "postgres" + "://" + url.UserPassword(role, "synthetic").String() +
		"@" + host + "/" + database + "?sslmode=" + sslmode
}

func fixtureNoticeFiles(t *testing.T) map[string]string {
	t.Helper()
	root := t.TempDir()
	write := func(name, value string) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	return map[string]string{
		"MAMONY_IDENTITY_SECURITY_NOTICE_DSN_FILE": write("notice-dsn",
			syntheticNoticeDSN("mamony_identity_security_notice_runtime",
				"db.example.test:5432", "mamony_staging", "verify-full")+"\n"),
		"MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_KEY_FILE": write("notice-key", "synthetic-mailgun-domain-key\n"),
		"MAMONY_IDENTITY_SECURITY_NOTICE_LIVE_APPROVAL": noticeApproval,
		"MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_REGION": "EU",
		"MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_DOMAIN": "mg.example.test",
		"MAMONY_IDENTITY_SECURITY_NOTICE_MAIL_FROM": "Mamony Security <security@mg.example.test>",
	}
}

func noticeEnv(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

func TestSecurityNoticeWorkerRequiresExplicitLiveOptIn(t *testing.T) {
	env := fixtureNoticeFiles(t)
	if _, err := readRuntimeConfig(noticeEnv(env), nil); !errors.Is(err, errStartup) {
		t.Fatal("worker accepted missing CLI approval")
	}
	if _, err := readRuntimeConfig(noticeEnv(env), []string{"--help"}); !errors.Is(err, errStartup) {
		t.Fatal("worker accepted wrong CLI argument")
	}
	env["MAMONY_IDENTITY_SECURITY_NOTICE_LIVE_APPROVAL"] = "yes"
	if _, err := readRuntimeConfig(noticeEnv(env), []string{realNoticeFlag}); !errors.Is(err, errStartup) {
		t.Fatal("worker accepted incorrect operator acknowledgement")
	}
	env["MAMONY_IDENTITY_SECURITY_NOTICE_LIVE_APPROVAL"] = noticeApproval
	env["CI"] = "true"
	if _, err := readRuntimeConfig(noticeEnv(env), []string{realNoticeFlag}); !errors.Is(err, errStartup) {
		t.Fatal("worker accepted real send authorization in CI")
	}
	delete(env, "CI")
	cfg, err := readRuntimeConfig(noticeEnv(env), []string{realNoticeFlag})
	if err != nil || cfg.Region != "EU" || cfg.DSN == "" ||
		cfg.MailgunKey != "synthetic-mailgun-domain-key" ||
		cfg.Domain != "mg.example.test" || cfg.PollInterval.Seconds() != 5 ||
		cfg.HTTPClient != nil {
		t.Fatalf("valid synthetic worker settings rejected: %v", err)
	}
}

func TestSecurityNoticeWorkerRefusesUnsafeCredentialSourcesAndDSNs(t *testing.T) {
	for _, tc := range []struct {
		name string
		change func(*testing.T, map[string]string)
	}{
		{"missing key file", func(_ *testing.T, env map[string]string) {
			env["MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_KEY_FILE"] = ""
		}},
		{"relative path", func(_ *testing.T, env map[string]string) {
			env["MAMONY_IDENTITY_SECURITY_NOTICE_DSN_FILE"] = "notice-dsn"
		}},
		{"world-readable file", func(t *testing.T, env map[string]string) {
			// #nosec G302 -- deliberately insecure negative test fixture.
			if err := os.Chmod(env["MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_KEY_FILE"], 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink", func(t *testing.T, env map[string]string) {
			link := filepath.Join(t.TempDir(), "key")
			if err := os.Symlink(env["MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_KEY_FILE"], link); err != nil {
				t.Fatal(err)
			}
			env["MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_KEY_FILE"] = link
		}},
		{"oversized secret", func(t *testing.T, env map[string]string) {
			if err := os.WriteFile(env["MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_KEY_FILE"],
				bytes.Repeat([]byte{'x'}, 513), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"disposable production database", func(t *testing.T, env map[string]string) {
			if err := os.WriteFile(env["MAMONY_IDENTITY_SECURITY_NOTICE_DSN_FILE"],
				[]byte(syntheticNoticeDSN("mamony_identity_security_notice_runtime",
					"localhost:5432", "mamony_test_fixture", "verify-full")), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"credential role confusion", func(t *testing.T, env map[string]string) {
			if err := os.WriteFile(env["MAMONY_IDENTITY_SECURITY_NOTICE_DSN_FILE"],
				[]byte(syntheticNoticeDSN("mamony_identity_delivery_runtime",
					"db.example.test:5432", "mamony_staging", "verify-full")), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"TLS disabled", func(t *testing.T, env map[string]string) {
			if err := os.WriteFile(env["MAMONY_IDENTITY_SECURITY_NOTICE_DSN_FILE"],
				[]byte(syntheticNoticeDSN("mamony_identity_security_notice_runtime",
					"db.example.test:5432", "mamony_staging", "disable")), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"extra libpq parameters", func(t *testing.T, env map[string]string) {
			raw := syntheticNoticeDSN("mamony_identity_security_notice_runtime",
				"db.example.test:5432", "mamony_staging", "verify-full") + "&hostaddr=127.0.0.1"
			if err := os.WriteFile(env["MAMONY_IDENTITY_SECURITY_NOTICE_DSN_FILE"],
				[]byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"no explicit port", func(t *testing.T, env map[string]string) {
			if err := os.WriteFile(env["MAMONY_IDENTITY_SECURITY_NOTICE_DSN_FILE"],
				[]byte(syntheticNoticeDSN("mamony_identity_security_notice_runtime",
					"db.example.test", "mamony_staging", "verify-full")), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"embedded newline in key", func(t *testing.T, env map[string]string) {
			if err := os.WriteFile(env["MAMONY_IDENTITY_SECURITY_NOTICE_MAILGUN_KEY_FILE"],
				[]byte("first\nsecond"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := fixtureNoticeFiles(t)
			tc.change(t, env)
			if _, err := readRuntimeConfig(noticeEnv(env), []string{realNoticeFlag});
				!errors.Is(err, errStartup) {
				t.Fatal("unsafe security notice worker settings were accepted")
			}
		})
	}
}

func TestSecurityNoticeStartupErrorsNeverExposePrivateSecretPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	if _, err := readPrivateFile(path, 100); !errors.Is(err, errStartup) ||
		err.Error() != "identity security notice worker startup blocked" {
		t.Fatal("startup failure exposed private file path or secret contents")
	}
}
