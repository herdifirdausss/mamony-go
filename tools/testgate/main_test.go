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
