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
	"TestPostgresIdentityZAuthenticationGrantRole",
	"TestPostgresIdentityZPasswordChangeRole",
	"TestPostgresIdentityZSecurityNoticeRole",
	"TestPostgresIdentityZMailgunWebhookRole",
	"TestPostgresIdentityZNoticeMonitorRole",
	"TestPostgresIdentityZRecoveryIssuerRole",
	"TestPostgresIdentityZRecoveryDeliveryRole",
	"TestPostgresIdentityZRecoveryRedeemerRole",
	"TestPostgresIdentityZRecoveryAssuranceRole",
	"TestPostgresIdentityZRecoveryCodeManagerRole",
	"TestPostgresIdentityZRecoveryFactorRoles",
	"TestPostgresIdentityZPasskeyRegistrationIssuerRole",
	"TestPostgresIdentityZPasskeyRegistrationReaderRole",
	"TestPostgresIdentityZPasskeyRegistrationAdmissionRole",
	"TestPostgresIdentityZRecoveryCodeSecurityNoticeRoles",
	"TestPostgresIdentityZRecoveryCodeNoticeWebhookRole",
	"TestPostgresIdentityZRecoverySecurityNoticeRoles",
	"TestPostgresIdentityZRecoveryNoticeWebhookRole",
	"TestPostgresIdentityDeliveryRole",
	"TestPostgresIdentityAdmissionRole",
	"TestPostgresIdentityMaintenanceRole",
	"TestIdentityRegistrationPersistence",
	"TestIdentityPasswordAuthentication",
	"TestIdentityOpaqueSessions",
	"TestIdentityZSessionFreezeIssueInterleavings",
	"TestIdentityZDirectSessionMintThreatFixture",
	"TestIdentityZAuthenticationGrants",
	"TestIdentityZSessionIdleExpiry",
	"TestIdentityZMaxActiveSessions",
	"TestIdentityZAuthMaterialEpochFence",
	"TestIdentityZAuthenticatedPasswordChange",
	"TestIdentityZPasswordRecoveryRedemption",
	"TestIdentityZRecoveryOfflineAssurance",
	"TestIdentityZRecoveryCodeEnrollmentRotation",
	"TestIdentityZRecoveryFactorSignedChallenge",
	"TestIdentityZPasskeyRegistrationIntent",
	"TestIdentityZPasskeyRegistrationAdmission",
	"TestIdentityZSecurityNoticeOutbox",
	"TestIdentityEmailVerification",
	"TestIdentityProofDeliveryOutbox",
	"TestIdentityMailgunMockPipeline",
	"TestIdentityMailgunMockRecoveryDelivery",
	"TestIdentityMailgunMockRecoverySecurityNotice",
	"TestIdentityMailgunMockRecoveryCodeSecurityNotice",
	"TestIdentityMailgunMockRecoveryCodeNoticeWebhookReconciliation",
	"TestIdentityMailgunMockSecurityNoticePipeline",
	"TestIdentityMailgunMockWebhookReconciliation",
	"TestIdentityMailgunMockRecoveryNoticeWebhookReconciliation",
	"TestIdentityMailgunNoticeMonitorRole",
	"TestIdentityHTTPEnrollmentDB",
	"TestIdentityZEnrollmentReconciliationDB",
	"TestIdentityZEnrollmentBacklogDB",
	"TestIdentityZEmailConfirmationHTTPDB",
	"TestIdentityZConfirmationAdmissionHTTPDB",
	"TestIdentityResendAdmission",
	"TestIdentityAnonymousAdmission",
	"TestIdentityRecoveryIssuance",
	"TestIdentityRecoveryDelivery",
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
	identityAuthGrantDSN := os.Getenv("TEST_IDENTITY_AUTH_GRANT_URL")
	identityPasswordChangeDSN := os.Getenv("TEST_IDENTITY_PASSWORD_CHANGE_URL")
	identityRecoveryIssuerDSN := os.Getenv("TEST_IDENTITY_RECOVERY_ISSUER_URL")
	identityRecoveryDeliveryDSN := os.Getenv("TEST_IDENTITY_RECOVERY_DELIVERY_URL")
	identityRecoveryRedeemerDSN := os.Getenv("TEST_IDENTITY_RECOVERY_REDEEMER_URL")
	identityRecoveryAssuranceDSN := os.Getenv("TEST_IDENTITY_RECOVERY_ASSURANCE_URL")
	identityRecoveryCodeManagerDSN := os.Getenv("TEST_IDENTITY_RECOVERY_CODE_MANAGER_URL")
	identityRecoveryFactorIssuerDSN := os.Getenv("TEST_IDENTITY_RECOVERY_FACTOR_ISSUER_URL")
	identityRecoveryFactorVerifierDSN := os.Getenv("TEST_IDENTITY_RECOVERY_FACTOR_VERIFIER_URL")
	identityPasskeyIssuerDSN := os.Getenv("TEST_IDENTITY_PASSKEY_REGISTRATION_ISSUER_URL")
	identityPasskeyReaderDSN := os.Getenv("TEST_IDENTITY_PASSKEY_REGISTRATION_READER_URL")
	identityPasskeyAdmissionDSN := os.Getenv("TEST_IDENTITY_PASSKEY_REGISTRATION_ADMISSION_URL")
	identityRecoveryCodeNoticeDSN := os.Getenv("TEST_IDENTITY_RECOVERY_CODE_NOTICE_URL")
	identityRecoveryCodeNoticeWebhookDSN := os.Getenv("TEST_IDENTITY_RECOVERY_CODE_NOTICE_WEBHOOK_URL")
	identityRecoveryCodeNoticeMonitorDSN := os.Getenv("TEST_IDENTITY_RECOVERY_CODE_NOTICE_MONITOR_URL")
	identityRecoveryNoticeDSN := os.Getenv("TEST_IDENTITY_RECOVERY_NOTICE_URL")
	identityRecoveryNoticeMonitorDSN := os.Getenv("TEST_IDENTITY_RECOVERY_NOTICE_MONITOR_URL")
	identityRecoveryNoticeWebhookDSN := os.Getenv("TEST_IDENTITY_RECOVERY_NOTICE_WEBHOOK_URL")
	identitySessionDSN := os.Getenv("TEST_IDENTITY_SESSION_URL")
	identityDeliveryDSN := os.Getenv("TEST_IDENTITY_DELIVERY_URL")
	identityNoticeDSN := os.Getenv("TEST_IDENTITY_SECURITY_NOTICE_URL")
	identityWebhookDSN := os.Getenv("TEST_IDENTITY_NOTICE_WEBHOOK_URL")
	identityMonitorDSN := os.Getenv("TEST_IDENTITY_NOTICE_MONITOR_URL")
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
	if err := validateDSN(identityAuthGrantDSN, "mamony_identity_auth_grant_runtime"); err != nil {
		return fmt.Errorf("identity grant fixture: %w", err)
	}
	if err := validateDSN(identityRecoveryNoticeDSN, "mamony_identity_recovery_notice_runtime"); err != nil {
		return fmt.Errorf("identity recovery notice fixture: %w", err)
	}
	if err := validateDSN(identityRecoveryNoticeWebhookDSN, "mamony_identity_recovery_notice_webhook_runtime"); err != nil {
		return fmt.Errorf("identity recovery notice webhook fixture: %w", err)
	}
	if err := validateDSN(identityRecoveryNoticeMonitorDSN, "mamony_identity_recovery_notice_monitor_runtime"); err != nil {
		return fmt.Errorf("identity recovery notice monitor fixture: %w", err)
	}
	if err := validateDSN(identityRecoveryCodeNoticeWebhookDSN, "mamony_identity_recovery_code_notice_webhook_runtime"); err != nil {
		return fmt.Errorf("identity recovery code notice webhook fixture: %w", err)
	}
	if err := validateDSN(identityRecoveryCodeNoticeDSN, "mamony_identity_recovery_code_notice_runtime"); err != nil {
		return fmt.Errorf("identity recovery code notice fixture: %w", err)
	}
	if err := validateDSN(identityRecoveryCodeNoticeMonitorDSN, "mamony_identity_recovery_code_notice_monitor_runtime"); err != nil {
		return fmt.Errorf("identity recovery code notice monitor fixture: %w", err)
	}
	if err := validateDSN(identityRecoveryFactorIssuerDSN, "mamony_identity_recovery_factor_issuer_runtime"); err != nil {
		return fmt.Errorf("identity recovery factor issuer fixture: %w", err)
	}
	if err := validateDSN(identityRecoveryFactorVerifierDSN, "mamony_identity_recovery_factor_verifier_runtime"); err != nil {
		return fmt.Errorf("identity recovery factor verifier fixture: %w", err)
	}
	if err := validateDSN(identityPasskeyIssuerDSN, "mamony_identity_passkey_registration_issuer_runtime"); err != nil {
		return fmt.Errorf("identity passkey intent issuer fixture: %w", err)
	}
	if err := validateDSN(identityPasskeyReaderDSN, "mamony_identity_passkey_registration_reader_runtime"); err != nil {
		return fmt.Errorf("identity passkey evidence reader fixture: %w", err)
	}
	if err := validateDSN(identityPasskeyAdmissionDSN, "mamony_identity_passkey_registration_admission_runtime"); err != nil {
		return fmt.Errorf("identity passkey reauth admission fixture: %w", err)
	}
	if err := validateDSN(identityRecoveryCodeManagerDSN, "mamony_identity_recovery_code_manager_runtime"); err != nil {
		return fmt.Errorf("identity recovery code manager fixture: %w", err)
	}
	if err := validateDSN(identityRecoveryAssuranceDSN, "mamony_identity_recovery_assurance_runtime"); err != nil {
		return fmt.Errorf("identity recovery assurance fixture: %w", err)
	}
	if err := validateDSN(identityRecoveryRedeemerDSN, "mamony_identity_recovery_redeemer_runtime"); err != nil {
		return fmt.Errorf("identity recovery redeemer fixture: %w", err)
	}
	if err := validateDSN(identityRecoveryDeliveryDSN, "mamony_identity_recovery_delivery_runtime"); err != nil {
		return fmt.Errorf("identity recovery delivery fixture: %w", err)
	}
	if err := validateDSN(identityRecoveryIssuerDSN, "mamony_identity_recovery_issuer_runtime"); err != nil {
		return fmt.Errorf("identity recovery issuer fixture: %w", err)
	}
	if err := validateDSN(identityPasswordChangeDSN, "mamony_identity_password_change_runtime"); err != nil {
		return fmt.Errorf("identity password change fixture: %w", err)
	}
	if err := validateDSN(identitySessionDSN, "mamony_identity_session_runtime"); err != nil {
		return fmt.Errorf("identity session fixture: %w", err)
	}
	if err := validateDSN(identityMonitorDSN, "mamony_identity_notice_monitor_runtime"); err != nil {
		return fmt.Errorf("identity notice monitor fixture: %w", err)
	}
	if err := validateDSN(identityWebhookDSN, "mamony_identity_notice_webhook_runtime"); err != nil {
		return fmt.Errorf("identity notice webhook fixture: %w", err)
	}
	if err := validateDSN(identityNoticeDSN, "mamony_identity_security_notice_runtime"); err != nil {
		return fmt.Errorf("identity security notice fixture: %w", err)
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
	identityAuthGrantURL, _ := url.Parse(identityAuthGrantDSN)
	identityPasswordChangeURL, _ := url.Parse(identityPasswordChangeDSN)
	identityRecoveryIssuerURL, _ := url.Parse(identityRecoveryIssuerDSN)
	identityRecoveryDeliveryURL, _ := url.Parse(identityRecoveryDeliveryDSN)
	identityRecoveryRedeemerURL, _ := url.Parse(identityRecoveryRedeemerDSN)
	identityRecoveryAssuranceURL, _ := url.Parse(identityRecoveryAssuranceDSN)
	identityRecoveryCodeManagerURL, _ := url.Parse(identityRecoveryCodeManagerDSN)
	identityRecoveryFactorIssuerURL, _ := url.Parse(identityRecoveryFactorIssuerDSN)
	identityRecoveryFactorVerifierURL, _ := url.Parse(identityRecoveryFactorVerifierDSN)
	identityPasskeyIssuerURL, _ := url.Parse(identityPasskeyIssuerDSN)
	identityPasskeyReaderURL, _ := url.Parse(identityPasskeyReaderDSN)
	identityPasskeyAdmissionURL, _ := url.Parse(identityPasskeyAdmissionDSN)
	identityRecoveryCodeNoticeURL, _ := url.Parse(identityRecoveryCodeNoticeDSN)
	identityRecoveryCodeNoticeWebhookURL, _ := url.Parse(identityRecoveryCodeNoticeWebhookDSN)
	identityRecoveryCodeNoticeMonitorURL, _ := url.Parse(identityRecoveryCodeNoticeMonitorDSN)
	identityRecoveryNoticeURL, _ := url.Parse(identityRecoveryNoticeDSN)
	identityRecoveryNoticeMonitorURL, _ := url.Parse(identityRecoveryNoticeMonitorDSN)
	identityRecoveryNoticeWebhookURL, _ := url.Parse(identityRecoveryNoticeWebhookDSN)
	identitySessionURL, _ := url.Parse(identitySessionDSN)
	identityDeliveryURL, _ := url.Parse(identityDeliveryDSN)
	identityNoticeURL, _ := url.Parse(identityNoticeDSN)
	identityWebhookURL, _ := url.Parse(identityWebhookDSN)
	identityMonitorURL, _ := url.Parse(identityMonitorDSN)
	identityMaintenanceURL, _ := url.Parse(identityMaintenanceDSN)
	for _, other := range []*url.URL{adminURL, migratorURL, platformRuntimeURL, identityMigratorURL, identityRuntimeURL, identityVerifierURL, identityAuthenticatorURL, identityAuthGrantURL, identityPasswordChangeURL, identityRecoveryIssuerURL, identityRecoveryDeliveryURL, identityRecoveryRedeemerURL, identityRecoveryAssuranceURL, identityRecoveryFactorIssuerURL, identityRecoveryFactorVerifierURL, identityPasskeyIssuerURL, identityPasskeyReaderURL, identityPasskeyAdmissionURL, identityRecoveryCodeManagerURL, identityRecoveryCodeNoticeURL, identityRecoveryCodeNoticeWebhookURL, identityRecoveryCodeNoticeMonitorURL, identityRecoveryNoticeURL, identityRecoveryNoticeMonitorURL, identityRecoveryNoticeWebhookURL, identitySessionURL, identityNoticeURL, identityWebhookURL, identityMonitorURL, identityDeliveryURL, identityMaintenanceURL} {
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
			"-run", "^TestIdentity(MailgunMock|MailgunNoticeMonitorRole|HTTPEnrollmentDB|ZEnrollmentReconciliationDB|ZEnrollmentBacklogDB|ZEmailConfirmationHTTPDB|ZConfirmationAdmissionHTTPDB|PasswordAuthentication|OpaqueSessions|ZSessionFreezeIssueInterleavings|ZDirectSessionMintThreatFixture|ZAuthenticationGrants|ZSessionIdleExpiry|ZMaxActiveSessions|ZAuthMaterialEpochFence|ZAuthenticatedPasswordChange|ZPasswordRecoveryRedemption|ZRecoveryOfflineAssurance|ZRecoveryCodeEnrollmentRotation|ZRecoveryFactorSignedChallenge|ZPasskeyRegistrationIntent|ZPasskeyRegistrationAdmission|ZSecurityNoticeOutbox)", "-count=1", "-timeout=3m", "./internal/modules/identity"),
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
						strings.HasPrefix(ev.Test, "TestIdentityMailgunNoticeMonitorRole") ||
						strings.HasPrefix(ev.Test, "TestIdentityHTTPEnrollmentDB") ||
						strings.HasPrefix(ev.Test, "TestIdentityZEnrollmentReconciliationDB") ||
						strings.HasPrefix(ev.Test, "TestIdentityZEnrollmentBacklogDB") ||
						strings.HasPrefix(ev.Test, "TestIdentityZEmailConfirmationHTTPDB") ||
						strings.HasPrefix(ev.Test, "TestIdentityZConfirmationAdmissionHTTPDB") ||
						strings.HasPrefix(ev.Test, "TestIdentityPasswordAuthentication") ||
						strings.HasPrefix(ev.Test, "TestIdentityOpaqueSessions") ||
						strings.HasPrefix(ev.Test, "TestIdentityZSessionFreezeIssueInterleavings") ||
						strings.HasPrefix(ev.Test, "TestIdentityZDirectSessionMintThreatFixture") ||
						strings.HasPrefix(ev.Test, "TestIdentityZAuthenticationGrants") ||
						strings.HasPrefix(ev.Test, "TestIdentityZSessionIdleExpiry") ||
						strings.HasPrefix(ev.Test, "TestIdentityZMaxActiveSessions") ||
						strings.HasPrefix(ev.Test, "TestIdentityZAuthMaterialEpochFence") ||
						strings.HasPrefix(ev.Test, "TestIdentityZAuthenticatedPasswordChange") ||
						strings.HasPrefix(ev.Test, "TestIdentityZPasswordRecoveryRedemption") ||
						strings.HasPrefix(ev.Test, "TestIdentityZRecoveryOfflineAssurance") ||
						strings.HasPrefix(ev.Test, "TestIdentityZRecoveryCodeEnrollmentRotation") ||
						strings.HasPrefix(ev.Test, "TestIdentityZRecoveryFactorSignedChallenge") ||
						strings.HasPrefix(ev.Test, "TestIdentityZPasskeyRegistrationIntent") ||
						strings.HasPrefix(ev.Test, "TestIdentityZPasskeyRegistrationAdmission") ||
						strings.HasPrefix(ev.Test, "TestIdentityMailgunMockRecoveryCodeSecurityNotice") ||
						strings.HasPrefix(ev.Test, "TestIdentityZSecurityNoticeOutbox"))) {
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
