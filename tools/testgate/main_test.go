package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestValidateTestDSN(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, url, user string
		allowed         bool
	}{
		{"runtime allowed", "postgres://mamony_test_runtime:x@127.0.0.1:5432/mamony_test_ci?sslmode=disable", "mamony_test_runtime", true},
		{"admin allowed", "postgresql://mamony_test_admin:x@localhost:5433/mamony_test_local?sslmode=disable", "mamony_test_admin", true},
		{"production database", "postgres://mamony_test_runtime:x@localhost:5432/payments?sslmode=disable", "mamony_test_runtime", false},
		{"external host", "postgres://mamony_test_runtime:x@prod.example:5432/mamony_test_ci?sslmode=disable", "mamony_test_runtime", false},
		{"missing port", "postgres://mamony_test_runtime:x@localhost/mamony_test_ci?sslmode=disable", "mamony_test_runtime", false},
		{"user mismatch", "postgres://postgres:x@localhost:5432/mamony_test_ci?sslmode=disable", "mamony_test_runtime", false},
		{"no password", "postgres://mamony_test_runtime@localhost:5432/mamony_test_ci?sslmode=disable", "mamony_test_runtime", false},
		{"host override", "postgres://mamony_test_runtime:x@localhost:5432/mamony_test_ci?sslmode=disable&host=prod.example", "mamony_test_runtime", false},
		{"dbname override", "postgres://mamony_test_runtime:x@localhost:5432/mamony_test_ci?sslmode=disable&dbname=payments", "mamony_test_runtime", false},
		{"encoded path extra", "postgres://mamony_test_runtime:x@localhost:5432/mamony_test_ci%2Fother?sslmode=disable", "mamony_test_runtime", false},
		{"unsupported query", "postgres://mamony_test_runtime:x@localhost:5432/mamony_test_ci?sslmode=verify-full", "mamony_test_runtime", false},
		{"missing env", "", "mamony_test_runtime", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDSN(tc.url, tc.user)
			if (err == nil) != tc.allowed {
				t.Fatalf("validateDSN() error=%v allowed=%v", err, tc.allowed)
			}
		})
	}
}

func TestSummarizeEventsRejectsEmptyOrSubtests(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, events string
		passed       int
	}{
		{"no tests", "{\"Action\":\"pass\",\"Package\":\"foo\"}\n", 0},
		{"subtest only", "{\"Action\":\"pass\",\"Package\":\"github.com/herdifirdausss/mamony-go/tools/postgrestest\",\"Test\":\"TestPostgresTenantIsolation/one\"}\n", 0},
		{"wrong package", "{\"Action\":\"pass\",\"Package\":\"github.com/herdifirdausss/mamony-go/tools/testgate\",\"Test\":\"TestPostgresTenantIsolation\"}\n", 0},
		{"real test", "{\"Action\":\"pass\",\"Package\":\"github.com/herdifirdausss/mamony-go/tools/postgrestest\",\"Test\":\"TestPostgresTenantIsolation\"}\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			passed, err := summarizeEvents(strings.NewReader(tc.events), &out)
			if err != nil || len(passed) != tc.passed {
				t.Fatalf("summarizeEvents() passed=%v err=%v", passed, err)
			}
		})
	}
}

func TestSummarizeEventsRejectsInvalidEvidence(t *testing.T) {
	t.Parallel()
	_, err := summarizeEvents(strings.NewReader("not-json"), &bytes.Buffer{})
	if err == nil {
		t.Fatal("invalid evidence was accepted")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write denied")
}

func TestSummarizeEventsPropagatesWriterFailure(t *testing.T) {
	t.Parallel()
	input := "{\"Action\":\"output\",\"Output\":\"test output\"}\n"
	_, err := summarizeEvents(strings.NewReader(input), failingWriter{})
	if err == nil || !strings.Contains(err.Error(), "write integration output") {
		t.Fatalf("expected a propagated write error, got %v", err)
	}
}

func TestK2aRequiredEvidenceRegistration(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"TestPostgresLedgerK2aMigration",
		"TestPostgresLedgerK2aCurrency",
		"TestPostgresLedgerK2aStorage",
		"TestPostgresLedgerK2aPrivileges",
		"TestPostgresLedgerK2aRestore",
		"TestPostgresLedgerK2aContainment",
	} {
		count := 0
		for _, required := range requiredTests {
			if required == name {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("mandatory test %s registration count: %d", name, count)
		}
		for _, action := range []string{"pass", "skip", "fail"} {
			event := "{\"Action\":\"" + action + "\",\"Package\":\"github.com/herdifirdausss/mamony-go/tools/postgrestest\",\"Test\":\"" + name + "\"}\n"
			passed, err := summarizeEvents(strings.NewReader(event), &bytes.Buffer{})
			if err != nil || passed[name] != (action == "pass") {
				t.Fatalf("incorrect evidence disposition: %s %s", name, action)
			}
		}
	}
}

func TestIdentityMigrationRequiredSubtestEvidence(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"TestPostgresIdentityMigration/isolated_v34_to_v35_upgrade",
		"TestPostgresIdentityMigration/isolated_v34_to_v35_upgrade/populated_data_preservation",
		"TestPostgresIdentityMigration/isolated_v34_to_v35_upgrade/applied_manifest_verification",
	} {
		count := 0
		for _, required := range requiredTests {
			if required == name {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("mandatory subtest %s registration count: %d", name, count)
		}
		for _, tc := range []struct {
			action, pkg, test string
			want              bool
		}{
			{"pass", requiredPackage, name, true},
			{"skip", requiredPackage, name, false},
			{"fail", requiredPackage, name, false},
			{"pass", identityPackage, name, false},
			{"pass", requiredPackage, name + "/other", false},
			{"pass", requiredPackage, "TestPostgresIdentityMigration", false},
		} {
			event := "{\"Action\":\"" + tc.action + "\",\"Package\":\"" + tc.pkg + "\",\"Test\":\"" + tc.test + "\"}\n"
			passed, err := summarizeEvents(strings.NewReader(event), &bytes.Buffer{})
			if err != nil || passed[name] != tc.want {
				t.Fatalf("incorrect evidence disposition: %s %s", name, tc.action)
			}
			if tc.test == name && passed["TestPostgresIdentityMigration"] {
				t.Fatal("subtest evidence must not satisfy its parent")
			}
		}
	}
}
