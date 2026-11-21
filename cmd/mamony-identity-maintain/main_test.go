package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetentionCommandDeniesByDefault(t *testing.T) {
	testEnv := func(key string) string {
		if key == "MAMONY_IDENTITY_MAINTENANCE_APPROVAL" {
			return maintenanceApproval
		}
		return ""
	}
	for _, args := range [][]string{nil, {"--other"}, {"--purge-expired", "extra"}} {
		if _, err := readConfig(testEnv, args); !errors.Is(err, errMaintenanceApproval) {
			t.Fatal("unsafe maintenance launch accepted")
		}
	}
	if _, err := readConfig(nil, []string{"--purge-expired"}); !errors.Is(err, errMaintenanceApproval) {
		t.Fatal("missing approval source accepted")
	}
}

func TestRetentionCommandRejectsUnsafeSecretFile(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "maintenance-dsn")
	raw := "postgres" + "://" + "mamony_identity_maintenance_runtime:synthetic@db.example.test:5432/mamony?sslmode=verify-full"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	env := func(k string) string {
		switch k {
		case "MAMONY_IDENTITY_MAINTENANCE_APPROVAL":
			return maintenanceApproval
		case "MAMONY_IDENTITY_MAINTENANCE_DSN_FILE":
			return path
		default:
			return ""
		}
	}
	if cfg, err := readConfig(env, []string{"--purge-expired"}); err != nil || cfg.DSN != raw {
		t.Fatalf("safe synthetic configuration unexpectedly rejected: %v", err)
	}
	if err := os.Chmod(path, 0644); err != nil { // #nosec G302 -- deliberately unsafe negative test.
		t.Fatal(err)
	}
	if _, err := readConfig(env, []string{"--purge-expired"}); !errors.Is(err, errMaintenanceApproval) {
		t.Fatal("world-readable maintenance secret accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(d, "symlink")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readConfig(func(k string) string {
		if k == "MAMONY_IDENTITY_MAINTENANCE_DSN_FILE" { return link }
		return env(k)
	}, []string{"--purge-expired"}); !errors.Is(err, errMaintenanceApproval) {
		t.Fatal("symlink secret accepted")
	}
	for _, bad := range []string{
		"postgres://mamony_identity_runtime:synthetic@db.example.test:5432/mamony?sslmode=verify-full",
		"postgres://mamony_identity_maintenance_runtime:synthetic@db.example.test:5432/mamony_test_ci?sslmode=verify-full",
		"postgres://mamony_identity_maintenance_runtime:synthetic@db.example.test:5432/mamony?sslmode=disable",
		strings.Repeat("a", 2049),
	} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil { t.Fatal(err) }
		if _, err := readConfig(env, []string{"--purge-expired"}); !errors.Is(err, errMaintenanceApproval) {
			t.Fatal("unsafe DSN accepted")
		}
	}
}
