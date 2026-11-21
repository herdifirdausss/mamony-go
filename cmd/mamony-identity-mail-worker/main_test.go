package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fixtureFiles(t *testing.T) map[string]string {
	t.Helper()
	root := t.TempDir()
	write := func(name string, body []byte) string {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	keys := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x31}, 32))
	return map[string]string{
		"MAMONY_IDENTITY_DELIVERY_DSN_FILE":  write("dsn", []byte(fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s\n", "mamony_identity_delivery_runtime", "synthetic", "db.example.test:5432", "mamony_staging", "verify-full"))),
		"MAMONY_IDENTITY_MAILGUN_KEY_FILE":   write("key", []byte("synthetic-domain-sending-key\n")),
		"MAMONY_IDENTITY_ENVELOPE_KEYS_FILE": write("envelopes", []byte(`{"active":"key1","keys":{"key1":"`+keys+`"}}`)),
		"MAMONY_IDENTITY_MAIL_LIVE_APPROVAL": liveApproval,
		"MAMONY_IDENTITY_MAILGUN_REGION":     "EU",
		"MAMONY_IDENTITY_MAILGUN_DOMAIN":     "mg.example.test",
		"MAMONY_IDENTITY_MAIL_FROM":          "security@mg.example.test",
		"MAMONY_IDENTITY_MAIL_VERIFY_URL":    "https://app.example.test/verify-email",
	}
}

func lookupEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestMailWorkerConfigRequiresDualExplicitEnablement(t *testing.T) {
	config := fixtureFiles(t)
	if _, err := readRuntimeConfig(lookupEnv(config), nil); !errors.Is(err, errStartup) {
		t.Fatal("worker started without CLI opt-in")
	}
	if _, err := readRuntimeConfig(lookupEnv(config), []string{"--help"}); !errors.Is(err, errStartup) {
		t.Fatal("unexpected CLI flag accepted")
	}
	config["MAMONY_IDENTITY_MAIL_LIVE_APPROVAL"] = "true"
	if _, err := readRuntimeConfig(lookupEnv(config), []string{realMailFlag}); !errors.Is(err, errStartup) {
		t.Fatal("missing explicit operator acknowledgement accepted")
	}
	config["MAMONY_IDENTITY_MAIL_LIVE_APPROVAL"] = liveApproval
	config["CI"] = "true"
	if _, err := readRuntimeConfig(lookupEnv(config), []string{realMailFlag}); !errors.Is(err, errStartup) {
		t.Fatal("CI attempted live Mailgun sending")
	}
	delete(config, "CI")
	got, err := readRuntimeConfig(lookupEnv(config), []string{realMailFlag})
	if err != nil {
		t.Fatal("valid synthetic secret configuration rejected")
	}
	if got.DSN == "" || got.MailgunKey != "synthetic-domain-sending-key" ||
		got.ActiveKeyID != "key1" || len(got.EnvelopeKeys["key1"]) != 32 ||
		got.Region != "EU" || got.HTTPClient != nil {
		t.Fatal("loaded worker config does not preserve strict authority")
	}
}

func TestMailWorkerRejectsUnsafeSecretFileAndDisposableDBForLiveMode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, env map[string]string)
	}{
		{"missing key file", func(_ *testing.T, env map[string]string) {
			env["MAMONY_IDENTITY_MAILGUN_KEY_FILE"] = ""
		}},
		{"relative file", func(_ *testing.T, env map[string]string) {
			env["MAMONY_IDENTITY_MAILGUN_KEY_FILE"] = "secret.txt"
		}},
		{"world-readable file", func(t *testing.T, env map[string]string) {
			if err := os.Chmod(env["MAMONY_IDENTITY_MAILGUN_KEY_FILE"], 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink", func(t *testing.T, env map[string]string) {
			link := filepath.Join(t.TempDir(), "key")
			if err := os.Symlink(env["MAMONY_IDENTITY_MAILGUN_KEY_FILE"], link); err != nil {
				t.Fatal(err)
			}
			env["MAMONY_IDENTITY_MAILGUN_KEY_FILE"] = link
		}},
		{"too-large file", func(t *testing.T, env map[string]string) {
			if err := os.WriteFile(env["MAMONY_IDENTITY_MAILGUN_KEY_FILE"], bytes.Repeat([]byte("x"), 600), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"test database", func(t *testing.T, env map[string]string) {
			if err := os.WriteFile(env["MAMONY_IDENTITY_DELIVERY_DSN_FILE"], []byte(fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=%s", "mamony_identity_delivery_runtime", "synthetic", "localhost:5432", "mamony_test_fixture", "disable")), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"bad JSON key file", func(t *testing.T, env map[string]string) {
			if err := os.WriteFile(env["MAMONY_IDENTITY_ENVELOPE_KEYS_FILE"], []byte(`{"active":"key1","keys":{"key1":"short"}}`), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"unknown JSON field", func(t *testing.T, env map[string]string) {
			if err := os.WriteFile(env["MAMONY_IDENTITY_ENVELOPE_KEYS_FILE"], []byte(`{"active":"key1","keys":{"key1":"a"},"unsafe":"true"}`), 0600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := fixtureFiles(t)
			tc.change(t, env)
			if _, err := readRuntimeConfig(lookupEnv(env), []string{realMailFlag}); !errors.Is(err, errStartup) {
				t.Fatalf("unsafe worker config accepted: %v", err)
			}
		})
	}
}

func TestWorkerSecretErrorsDoNotIncludePathsOrContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-secret")
	if _, err := readPrivateFile(path, 100); !errors.Is(err, errStartup) ||
		err.Error() != "identity mail worker startup blocked" {
		t.Fatal("secret file path or details leaked")
	}
}
