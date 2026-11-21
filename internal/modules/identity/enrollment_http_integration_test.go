//go:build integration

package identity

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
		"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/handler"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailcrypto"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/herdifirdausss/mamony-go/internal/platform/httpedge"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

type syntheticEnrollmentHasher struct{}

func (syntheticEnrollmentHasher) Hash(_ context.Context, _ string) (domain.PasswordVerifier, error) {
	return domain.PasswordVerifier{
		Version: 1, MemoryKiB: 65536, Iterations: 3, Parallelism: 1,
		Salt: bytes.Repeat([]byte{0x27}, 16), Digest: bytes.Repeat([]byte{0x48}, 32),
	}, nil
}

// TestIdentityHTTPEnrollmentDB is unmounted-handler integration against an
// actual disposable PostgreSQL; no network-exposed identity endpoint exists.
// Real token encryption is used, while KDF and delivery remain synthetic.
func TestIdentityHTTPEnrollmentDB(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	adminDSN := guardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := guardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	verifyDSN := guardedDB(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	root, _ := url.Parse(adminDSN)
	for _, dsn := range []string{regDSN, verifyDSN} {
		u, _ := url.Parse(dsn)
		if root.Host != u.Host || root.Path != u.Path {
			t.Fatal("all Identity fixtures must use exactly one disposable database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: 4})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	regPool := open(regDSN, "mamony_identity_runtime")
	verifyPool := open(verifyDSN, "mamony_identity_verifier_runtime")
	regStore, err := identitypg.NewStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	anonymousStore, err := identitypg.NewAdmissionStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	proofStore, err := identitypg.NewProofStore(verifyPool)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := usecase.NewRegistration(regStore, syntheticEnrollmentHasher{},
		domain.PasswordPolicy{IsBlocked: func(string, string) bool { return false }})
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := mailcrypto.NewKeyring("fixture-key", map[string][]byte{
		"fixture-key": bytes.Repeat([]byte{0x91}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := usecase.NewEmailVerification(proofStore, keyring)
	if err != nil {
		t.Fatal(err)
	}
	var regPort usecase.EnrollmentRegistrar = registration
	var lookupPort usecase.EnrollmentSubjectLookup = proofStore
	var proofPort usecase.EnrollmentProofIssuer = issuer
	flow, err := usecase.NewEnrollmentFlow(regPort, lookupPort, proofPort)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := usecase.NewAnonymousAdmission(anonymousStore, bytes.Repeat([]byte{0xa4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := handler.NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	var gatePort handler.AnonymousAdmissionGate = admission
	boundary, err := handler.NewAnonymousBoundary(resolver, gatePort)
	if err != nil {
		t.Fatal(err)
	}
	var workPort handler.EnrollmentWorkflow = flow
	h, err := handler.NewEnrollmentHTTP(boundary, workPort, "https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}

	email := strings.ToLower(randomSyntheticID(t, "browser_")) + "@example.test"
	unknown := strings.ToLower(randomSyntheticID(t, "unknown_")) + "@example.test"
	post := func(route string, email string, password string) *httptest.ResponseRecorder {
		t.Helper()
		values := map[string]string{"email": email}
		if route == "/v1/identity/register" {
			values["password"] = password
		}
		b, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "https://app.example.test"+route, bytes.NewReader(b))
		req.TLS = &tls.ConnectionState{}
		req.RemoteAddr = "198.51.100.74:34567"
		req.Header.Set("Origin", "https://app.example.test")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	register := post("/v1/identity/register", email, "valid synthetic password phrase")
	if register.Code != http.StatusAccepted {
		t.Fatalf("new registration rejected: %d %s", register.Code, register.Body.String())
	}
	var subject string
	if err := admin.QueryRow(ctx, `SELECT s.subject_id FROM mamony_identity.subjects s
		JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
		WHERE a.canonical_email=$1 AND s.status='UNVERIFIED'`, email).Scan(&subject); err != nil {
		t.Fatalf("registration did not persist a pending subject: %v", err)
	}
	t.Cleanup(func() {
		// Earlier suite tests share this disposable database with the later
		// Mailgun mock suite. Prevent it claiming synthetic browser jobs.
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanupCtx, `UPDATE mamony_identity.email_proof_deliveries d
			SET status='DEAD' FROM mamony_identity.email_verification_challenges c
			WHERE c.challenge_id=d.challenge_id AND c.subject_id=$1
			  AND d.status='PENDING'`, subject); err != nil {
			t.Errorf("synthetic browser fixture cleanup failed: %v", err)
		}
	})
	count := func() (int, int, int) {
		t.Helper()
		var challenges, envelopes, events int
		err := admin.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM mamony_identity.email_verification_challenges
			WHERE subject_id=$1),
			(SELECT count(*) FROM mamony_identity.email_proof_deliveries d
			JOIN mamony_identity.email_verification_challenges c USING(challenge_id)
			WHERE c.subject_id=$1 AND octet_length(d.sealed_token)=71),
			(SELECT count(*) FROM mamony_identity.security_outbox
			WHERE subject_id=$1 AND event_type='EMAIL_PROOF_ISSUED')`, subject).
			Scan(&challenges, &envelopes, &events)
		if err != nil {
			t.Fatal(err)
		}
		return challenges, envelopes, events
	}
	if a, b, c := count(); a != 1 || b != 1 || c != 1 {
		t.Fatalf("registration did not create exactly one encrypted proof obligation: %d %d %d", a, b, c)
	}
	duplicate := post("/v1/identity/register", email, "valid synthetic password phrase")
	unknownReply := post("/v1/identity/resend-verification", unknown, "")
	knownReply := post("/v1/identity/resend-verification", email, "")
	if duplicate.Code != 202 || unknownReply.Code != 202 || knownReply.Code != 202 ||
		duplicate.Body.String() != register.Body.String() ||
		unknownReply.Body.String() != register.Body.String() ||
		knownReply.Body.String() != register.Body.String() {
		t.Fatal("duplicate/missing/limited public receipts expose account eligibility")
	}
	if a, b, c := count(); a != 1 || b != 1 || c != 1 {
		t.Fatalf("duplicate registration or immediate resend bypassed proof cooldown: %d %d %d", a, b, c)
	}
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_verification_challenges
		SET requested_at=clock_timestamp()-interval '65 seconds'
		WHERE subject_id=$1`, subject); err != nil {
		t.Fatal(err)
	}
	resend := post("/v1/identity/resend-verification", email, "")
	if resend.Code != 202 || resend.Body.String() != register.Body.String() {
		t.Fatal("eligible resend changed public response")
	}
	if a, b, c := count(); a != 2 || b != 2 || c != 2 {
		t.Fatalf("resend did not generate exactly one new encrypted proof obligation: %d %d %d", a, b, c)
	}
	var live int
	err = admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.email_verification_challenges
		WHERE subject_id=$1 AND revoked_at IS NULL AND consumed_at IS NULL`, subject).Scan(&live)
	if err != nil || live != 1 {
		t.Fatalf("resend did not revoke earlier proof: live=%d err=%v", live, err)
	}
	// Public and private running routers are intentionally unchanged.
	for _, router := range []http.Handler{httpedge.PublicHandler(), httpedge.PrivateHandler()} {
		req := httptest.NewRequest(http.MethodPost, "https://app.example.test/v1/identity/register", nil)
		reply := httptest.NewRecorder()
		router.ServeHTTP(reply, req)
		if reply.Code != 404 {
			t.Fatalf("health router unexpectedly exposes Identity registration: %d", reply.Code)
		}
	}
}
