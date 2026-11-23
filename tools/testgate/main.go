// Command testgate requires real, explicitly named database integration tests
// to execute successfully. It intentionally does not provision PostgreSQL.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const requiredPackage = "github.com/herdifirdausss/mamony-go/tools/postgrestest"
const identityPackage = "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
const identityIntegrationPackage = "github.com/herdifirdausss/mamony-go/internal/modules/identity"

var requiredTests = []string{
	"TestPostgresRoleSeparation",
	"TestPostgresTenantIsolation",
	"TestPostgresTenantContext",
	"TestPostgresPlatformMigration",
	"TestPostgresRuntimePoolIsolation",
	"TestPostgresRuntimePoolRejectsElevatedRole",
	"TestPostgresIdentityMigration",
	"TestPostgresIdentityVerifierRole",
	"TestPostgresIdentityZAuthenticatorRole",
	"TestPostgresIdentityZSessionRole",
	"TestPostgresIdentityDeliveryRole",
	"TestPostgresIdentityAdmissionRole",
	"TestPostgresIdentityMaintenanceRole",
	"TestIdentityRegistrationPersistence",
	"TestIdentityPasswordAuthentication",
	"TestIdentityOpaqueSessions",
	"TestIdentityEmailVerification",
	"TestIdentityProofDeliveryOutbox",
	"TestIdentityMailgunMockPipeline",
	"TestIdentityHTTPEnrollmentDB",
	"TestIdentityZEnrollmentReconciliationDB",
	"TestIdentityZEnrollmentBacklogDB",
	"TestIdentityZEmailConfirmationHTTPDB",
	"TestIdentityZConfirmationAdmissionHTTPDB",
	"TestIdentityResendAdmission",
	"TestIdentityAnonymousAdmission",
	"TestIdentityConfirmationAdmission",
	"TestIdentityLoginAdmission",
	"TestIdentityRetentionPurge",
}

type testEvent struct {
	Action  string
	Package string
	Test    string
	Output  string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "INTEGRATION FAIL:", err)
		os.Exit(1)
	}
}

func run() error {
	runtimeDSN := os.Getenv("TEST_DATABASE_URL")
	adminDSN := os.Getenv("TEST_DATABASE_ADMIN_URL")
	platformMigratorDSN := os.Getenv("TEST_PLATFORM_MIGRATION_URL")
	platformRuntimeDSN := os.Getenv("TEST_PLATFORM_RUNTIME_URL")
	identityMigratorDSN := os.Getenv("TEST_IDENTITY_MIGRATION_URL")
	identityRuntimeDSN := os.Getenv("TEST_IDENTITY_RUNTIME_URL")
	identityVerifierDSN := os.Getenv("TEST_IDENTITY_VERIFIER_URL")
	identityAuthenticatorDSN := os.Getenv("TEST_IDENTITY_AUTHENTICATOR_URL")
	identitySessionDSN := os.Getenv("TEST_IDENTITY_SESSION_URL")
	identityDeliveryDSN := os.Getenv("TEST_IDENTITY_DELIVERY_URL")
	identityMaintenanceDSN := os.Getenv("TEST_IDENTITY_MAINTENANCE_URL")
	if err := validateDSN(runtimeDSN, "mamony_test_runtime"); err != nil {
		return fmt.Errorf("runtime fixture: %w", err)
	}
	if err := validateDSN(adminDSN, "mamony_test_admin"); err != nil {
		return fmt.Errorf("admin fixture: %w", err)
	}
	if err := validateDSN(platformMigratorDSN, "mamony_platform_migrator"); err != nil {
		return fmt.Errorf("platform migrator fixture: %w", err)
	}
	if err := validateDSN(platformRuntimeDSN, "mamony_platform_runtime"); err != nil {
		return fmt.Errorf("platform runtime fixture: %w", err)
	}
	if err := validateDSN(identityMigratorDSN, "mamony_identity_migrator"); err != nil {
		return fmt.Errorf("identity migrator fixture: %w", err)
	}
	if err := validateDSN(identityRuntimeDSN, "mamony_identity_runtime"); err != nil {
		return fmt.Errorf("identity runtime fixture: %w", err)
	}
	if err := validateDSN(identityVerifierDSN, "mamony_identity_verifier_runtime"); err != nil {
		return fmt.Errorf("identity verifier fixture: %w", err)
	}
	if err := validateDSN(identityAuthenticatorDSN, "mamony_identity_authenticator_runtime"); err != nil {
		return fmt.Errorf("identity authenticator fixture: %w", err)
	}
	if err := validateDSN(identitySessionDSN, "mamony_identity_session_runtime"); err != nil {
		return fmt.Errorf("identity session fixture: %w", err)
	}
	if err := validateDSN(identityDeliveryDSN, "mamony_identity_delivery_runtime"); err != nil {
		return fmt.Errorf("identity delivery fixture: %w", err)
	}
	if err := validateDSN(identityMaintenanceDSN, "mamony_identity_maintenance_runtime"); err != nil {
		return fmt.Errorf("identity maintenance fixture: %w", err)
	}
	runtimeURL, _ := url.Parse(runtimeDSN)
	adminURL, _ := url.Parse(adminDSN)
	migratorURL, _ := url.Parse(platformMigratorDSN)
	platformRuntimeURL, _ := url.Parse(platformRuntimeDSN)
	identityMigratorURL, _ := url.Parse(identityMigratorDSN)
	identityRuntimeURL, _ := url.Parse(identityRuntimeDSN)
	identityVerifierURL, _ := url.Parse(identityVerifierDSN)
	identityAuthenticatorURL, _ := url.Parse(identityAuthenticatorDSN)
	identitySessionURL, _ := url.Parse(identitySessionDSN)
	identityDeliveryURL, _ := url.Parse(identityDeliveryDSN)
	identityMaintenanceURL, _ := url.Parse(identityMaintenanceDSN)
	for _, other := range []*url.URL{adminURL, migratorURL, platformRuntimeURL, identityMigratorURL, identityRuntimeURL, identityVerifierURL, identityAuthenticatorURL, identitySessionURL, identityDeliveryURL, identityMaintenanceURL} {
		if runtimeURL.Path != other.Path || runtimeURL.Port() != other.Port() || runtimeURL.Hostname() != other.Hostname() {
			return errors.New("all test roles must target the same isolated test database")
		}
	}
	if _, err := exec.LookPath("psql"); err != nil {
		return errors.New("NOT RUN: PostgreSQL client psql is missing")
	}

	passed := make(map[string]bool)
	// Commands and package paths are compile-time constants. Never build a
	// command from a test-supplied repository path or environment variable.
	for _, cmd := range []*exec.Cmd{
		exec.Command("go", "test", "-json", "-tags=integration",
			"-run", "^TestPostgres", "-count=1", "-timeout=3m", "./tools/postgrestest"),
		exec.Command("go", "test", "-json", "-tags=integration",
			"-run", "^TestIdentity", "-count=1", "-timeout=3m", "./internal/modules/identity/internal/repository/postgres"),
		exec.Command("go", "test", "-json", "-tags=integration",
			"-run", "^TestIdentity(MailgunMock|HTTPEnrollmentDB|ZEnrollmentReconciliationDB|ZEnrollmentBacklogDB|ZEmailConfirmationHTTPDB|ZConfirmationAdmissionHTTPDB|PasswordAuthentication|OpaqueSessions)", "-count=1", "-timeout=3m", "./internal/modules/identity"),
	} {
		// Never inherit libpq overrides in the child tests.
		cmd.Env = stripPGEnv(os.Environ())
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		observed, parseErr := summarizeEvents(&stdout, os.Stdout)
		if parseErr != nil {
			return parseErr
		}
		if err != nil {
			return fmt.Errorf("required integration test command failed: %w (output: %s)", err, strings.TrimSpace(stderr.String()))
		}
		for k := range observed {
			passed[k] = true
		}
	}
	for _, name := range requiredTests {
		if !passed[name] {
			return fmt.Errorf("required PostgreSQL or Identity integration test did not pass: %s", name)
		}
	}
	fmt.Println("PostgreSQL integration gate: PASS")
	return nil
}

// validateDSN only accepts a disposable loopback test database. The query
// allowlist prevents libpq's host, hostaddr, service, or dbname overrides.
func validateDSN(raw, expectedUser string) error {
	if raw == "" {
		return errors.New("NOT RUN: test database URL is missing")
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return errors.New("invalid test database URL")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return errors.New("unexpected database URL scheme")
	}
	if u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" {
		return errors.New("test database must be on loopback")
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil || p < 1 || p > 65535 {
		return errors.New("test database must have a valid explicit port")
	}
	if u.Path == "" || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "mamony_test_") ||
		strings.Contains(strings.TrimPrefix(u.Path, "/"), "/") {
		return errors.New("test database name must start with mamony_test_")
	}
	if u.User == nil || u.User.Username() != expectedUser {
		return errors.New("test database user is not the expected role")
	}
	if _, ok := u.User.Password(); !ok {
		return errors.New("test database URL must have a synthetic fixture password")
	}
	if u.Fragment != "" || u.RawFragment != "" {
		return errors.New("test database URL cannot include a fragment")
	}
	q := u.Query()
	if len(q) != 1 || q.Get("sslmode") != "disable" || len(q["sslmode"]) != 1 {
		return errors.New("only sslmode=disable is allowed for the loopback fixture")
	}
	return nil
}

func summarizeEvents(src io.Reader, output io.Writer) (map[string]bool, error) {
	passed := make(map[string]bool)
	dec := json.NewDecoder(src)
	for {
		var ev testEvent
		if err := dec.Decode(&ev); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("cannot parse go test JSON evidence: %w", err)
		}
		if ev.Output != "" {
			if _, err := fmt.Fprint(output, ev.Output); err != nil {
				return nil, fmt.Errorf("write integration output: %w", err)
			}
		}
		if ev.Action == "pass" && !strings.Contains(ev.Test, "/") {
			if (ev.Package == requiredPackage && strings.HasPrefix(ev.Test, "TestPostgres")) ||
				(ev.Package == identityPackage && strings.HasPrefix(ev.Test, "TestIdentity")) ||
				(ev.Package == identityIntegrationPackage &&
					(strings.HasPrefix(ev.Test, "TestIdentityMailgunMock") ||
						strings.HasPrefix(ev.Test, "TestIdentityHTTPEnrollmentDB") ||
						strings.HasPrefix(ev.Test, "TestIdentityZEnrollmentReconciliationDB") ||
						strings.HasPrefix(ev.Test, "TestIdentityZEnrollmentBacklogDB") ||
						strings.HasPrefix(ev.Test, "TestIdentityZEmailConfirmationHTTPDB") ||
						strings.HasPrefix(ev.Test, "TestIdentityZConfirmationAdmissionHTTPDB") ||
						strings.HasPrefix(ev.Test, "TestIdentityPasswordAuthentication") ||
						strings.HasPrefix(ev.Test, "TestIdentityOpaqueSessions"))) {
				passed[ev.Test] = true
			}

		}
	}
	return passed, nil
}

func stripPGEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, item := range env {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(key, "PG") {
			continue
		}
		out = append(out, item)
	}
	return out
}
