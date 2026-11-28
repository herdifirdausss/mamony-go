//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/handler"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/integration/internal/fixture"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailcrypto"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/herdifirdausss/mamony-go/internal/platform/httpedge"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

type syntheticConfirmationAdmission struct{}

func (syntheticConfirmationAdmission) Admit(context.Context, netip.Addr) (bool, error) {
	return true, nil
}

// TestIdentityZEmailConfirmationHTTPDB exercises the UNMOUNTED handler,
// restricted verifier role, real PostgreSQL transactions and AES-GCM token
// issuance. No Mailgun API call, login session, or browser route is involved.
func TestIdentityZEmailConfirmationHTTPDB(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	adminDSN := fixture.GuardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := fixture.GuardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	verifyDSN := fixture.GuardedDB(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	base, _ := url.Parse(adminDSN)
	for _, raw := range []string{regDSN, verifyDSN} {
		u, _ := url.Parse(raw)
		if u.Host != base.Host || u.Path != base.Path {
			t.Fatal("verification roles do not target same disposable PostgreSQL database")
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
	reg, err := identitypg.NewRegistrationStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	proofs, err := identitypg.NewProofStore(open(verifyDSN, "mamony_identity_verifier_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	keys, err := mailcrypto.NewKeyring("synthetic-http", map[string][]byte{
		"synthetic-http": bytes.Repeat([]byte{0x92}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	var proofPort usecase.EmailProofPort = proofs
	var sealPort usecase.ProofEnvelopeSealer = keys
	confirmation, err := usecase.NewEmailVerification(proofPort, sealPort)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := handler.NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	var admissionPort handler.ConfirmationAdmission = syntheticConfirmationAdmission{}
	var confirmationPort handler.EmailConfirmer = confirmation
	h, err := handler.NewEmailConfirmationHTTP(peer, admissionPort, confirmationPort, "https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}
	newPending := func() (string, string) {
		t.Helper()
		subject := fixture.RandomID(t, "sub_")
		email := strings.ToLower(fixture.RandomID(t, "confirm_")) + "@example.test"
		err := reg.CreatePendingWithObligation(ctx, usecase.PendingRegistration{
			SubjectID: subject, EventID: fixture.RandomID(t, "evt_"),
			CanonicalEmail: email, Status: "UNVERIFIED", CreatedAt: time.Now().UTC(),
			PasswordVerifier: domain.PasswordVerifier{
				Version: 1, MemoryKiB: 65536, Iterations: 3, Parallelism: 1,
				Salt: bytes.Repeat([]byte{0x21}, 16), Digest: bytes.Repeat([]byte{0x42}, 32),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		out, err := confirmation.Prepare(ctx, subject)
		if err != nil {
			t.Fatal(err)
		}
		recipient, bearer := out.ForTrustedDelivery()
		if recipient != email || len(bearer) != 43 {
			t.Fatal("proof was not bound to the registered contact")
		}
		t.Cleanup(func() {
			cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			if _, err := admin.Exec(cleanupCtx, `UPDATE mamony_identity.email_proof_deliveries d
				SET status='DEAD' FROM mamony_identity.email_verification_challenges c
				WHERE c.challenge_id=d.challenge_id AND c.subject_id=$1
				  AND d.status='PENDING'`, subject); err != nil {
				t.Errorf("failed to quarantine synthetic confirmation mail: %v", err)
			}
		})
		return subject, bearer
	}
	postWith := func(target http.Handler, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "https://app.example.test/v1/identity/verify-email",
			strings.NewReader(`{"token":"`+token+`"}`))
		r.TLS = &tls.ConnectionState{}
		r.RemoteAddr = "198.51.100.42:40000"
		r.Header.Set("Origin", "https://app.example.test")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		target.ServeHTTP(w, r)
		return w
	}
	post := func(token string) *httptest.ResponseRecorder {
		return postWith(h, token)
	}
	checkState := func(subject string, expectedStatus string, expectedEvents int) {
		t.Helper()
		var status string
		var verified bool
		var events int
		err := admin.QueryRow(ctx, `SELECT s.status, a.verified_at IS NOT NULL,
			(SELECT count(*) FROM mamony_identity.security_outbox o
				WHERE o.subject_id=s.subject_id AND o.event_type='CONTACT_VERIFIED')
			FROM mamony_identity.subjects s
			JOIN mamony_identity.email_aliases a USING(subject_id)
			WHERE s.subject_id=$1`, subject).Scan(&status, &verified, &events)
		if err != nil || status != expectedStatus ||
			verified != (expectedStatus == "ACTIVE") || events != expectedEvents {
			t.Fatalf("verification authority mismatch: %s verified=%v events=%d err=%v", status, verified, events, err)
		}
	}

	validSubject, secret := newPending()

	// The registration-role credential is intentionally prohibited from
	// reading proof material. A correctly shaped bearer with a broken
	// verifier authority must return 503, not pretend the token is invalid.
	unprivilegedProof, err := identitypg.NewProofStore(regPool)
	if err != nil {
		t.Fatal(err)
	}
	var unprivilegedProofPort usecase.EmailProofPort = unprivilegedProof
	unprivilegedConfirm, err := usecase.NewEmailVerification(unprivilegedProofPort, sealPort)
	if err != nil {
		t.Fatal(err)
	}
	var unprivilegedConfirmPort handler.EmailConfirmer = unprivilegedConfirm
	unavailableHandler, err := handler.NewEmailConfirmationHTTP(peer, admissionPort,
		unprivilegedConfirmPort, "https://app.example.test")
	if err != nil {
		t.Fatal(err)
	}
	unavailable := postWith(unavailableHandler, secret)
	if unavailable.Code != http.StatusServiceUnavailable ||
		unavailable.Header().Get("Cache-Control") != "no-store" ||
		unavailable.Header().Get("Referrer-Policy") != "no-referrer" ||
		strings.Contains(unavailable.Body.String(), secret) {
		t.Fatalf("inaccessible verifier authority was exposed as a bad proof or leaked secret: %d %q",
			unavailable.Code, unavailable.Body.String())
	}
	checkState(validSubject, "UNVERIFIED", 0)

	success := post(secret)
	if success.Code != 204 || success.Body.Len() != 0 ||
		success.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("valid subject not activated: %d %q", success.Code, success.Body.String())
	}
	checkState(validSubject, "ACTIVE", 1)
	replay := post(secret)
	unknown := post(base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x7f}, 32)))
	if replay.Code != 400 || unknown.Code != 400 ||
		replay.Body.String() != unknown.Body.String() {
		t.Fatal("replayed and nonexistent proofs did not have generic failure")
	}
	checkState(validSubject, "ACTIVE", 1)

	expiredSubject, expiredToken := newPending()
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_verification_challenges
		SET requested_at=clock_timestamp()-interval '30 minutes',
		    expires_at=clock_timestamp()-interval '1 minute'
		WHERE subject_id=$1`, expiredSubject); err != nil {
		t.Fatal(err)
	}
	expired := post(expiredToken)
	if expired.Code != 400 || expired.Body.String() != replay.Body.String() {
		t.Fatal("expired proof received a distinguishable response")
	}
	checkState(expiredSubject, "UNVERIFIED", 0)

	concurrentSubject, concurrentToken := newPending()
	const attempts = 8
	results := make(chan int, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- post(concurrentToken).Code
		}()
	}
	wg.Wait()
	close(results)
	winners := 0
	for code := range results {
		if code == 204 {
			winners++
		} else if code != 400 {
			t.Errorf("unexpected concurrent confirmation outcome %d", code)
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent proof consumption must have one winner, got=%d", winners)
	}
	checkState(concurrentSubject, "ACTIVE", 1)
	// Both actually running HTTP routers remain health-only.
	for _, router := range []http.Handler{httpedge.PublicHandler(), httpedge.PrivateHandler()} {
		req := httptest.NewRequest(http.MethodPost, "https://app.example.test/v1/identity/verify-email", nil)
		reply := httptest.NewRecorder()
		router.ServeHTTP(reply, req)
		if reply.Code != http.StatusNotFound {
			t.Fatalf("running router unexpectedly mounted Identity confirmation: %d", reply.Code)
		}
	}
}
