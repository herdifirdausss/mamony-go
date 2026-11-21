package postgres

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestRuntimeConfigRefusesUnsafeConnection(t *testing.T) {
	t.Parallel()
	// Assemble a synthetic credential rather than embedding a literal
	// password-bearing DSN; gosec G101 remains enabled for application code.
	safe := (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword("mamony_test_runtime", "sentinel-do-not-log"),
		Host:     "127.0.0.1:5432",
		Path:     "/mamony_test_ci",
		RawQuery: "sslmode=disable",
	}).String()
	cases := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"local disposable", Config{safe, "mamony_test_runtime", 1}, true},
		{"verified tls", Config{"postgres://mamony_platform_runtime:pw@db.example:5432/mamony_db?sslmode=verify-full", "mamony_platform_runtime", 4}, true},
		{"missing role", Config{safe, "", 1}, false},
		{"wrong user", Config{safe, "mamony_platform_runtime", 1}, false},
		{"privileged role", Config{"postgres://mamony_platform_migrator:pw@localhost:5432/mamony_test_ci?sslmode=disable", "mamony_platform_migrator", 1}, false},
		{"non test tls disabled", Config{"postgres://mamony_platform_runtime:pw@localhost:5432/mamony_db?sslmode=disable", "mamony_platform_runtime", 1}, false},
		{"remote tls disabled", Config{"postgres://mamony_test_runtime:pw@evil.example:5432/mamony_test_ci?sslmode=disable", "mamony_test_runtime", 1}, false},
		{"sql option override", Config{safe + "&options=-c%20role%3Dpostgres", "mamony_test_runtime", 1}, false},
		{"max zero", Config{safe, "mamony_test_runtime", 0}, false},
		{"max excessive", Config{safe, "mamony_test_runtime", 9}, false},
		{"missing password", Config{"postgres://mamony_test_runtime@127.0.0.1:5432/mamony_test_ci?sslmode=disable", "mamony_test_runtime", 1}, false},
		{"unverified tls", Config{"postgres://mamony_platform_runtime:pw@db.example:5432/mamony_db?sslmode=require", "mamony_platform_runtime", 1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(tc.cfg)
			if (err == nil) != tc.ok {
				t.Fatalf("unexpected config validation: err=%v ok=%v", err, tc.ok)
			}
			if err != nil && strings.Contains(err.Error(), "sentinel-do-not-log") {
				t.Fatal("runtime credential leaked in validation error")
			}
		})
	}
}

func TestTenantNameValidation(t *testing.T) {
	t.Parallel()
	for _, tenant := range []string{"", " ", "a b", "tenant/a", "tenant;SET ROLE postgres", strings.Repeat("x", 65)} {
		if err := validateTenantID(tenant); err == nil {
			t.Errorf("invalid tenant accepted: %q", tenant)
		}
	}
	for _, tenant := range []string{"tenant_a", "tenant-B", "t1"} {
		if err := validateTenantID(tenant); err != nil {
			t.Errorf("valid tenant rejected: %q: %v", tenant, err)
		}
	}
}

func TestMissingPoolDoesNotInvokeCallback(t *testing.T) {
	t.Parallel()
	var p *Pool
	if err := p.WithTenantTx(context.Background(), "tenant_a", nil); err == nil {
		t.Fatal("nil runtime pool accepted")
	}
	p = &Pool{}
	if err := p.WithTenantTx(context.Background(), "tenant_a", nil); err == nil {
		t.Fatal("invalid runtime pool or callback accepted")
	}
}
