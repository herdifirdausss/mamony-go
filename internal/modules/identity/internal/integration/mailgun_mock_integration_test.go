//go:build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/integration/internal/fixture"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailcrypto"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/mailgun"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

func TestIdentityMailgunMockPipeline(t *testing.T) {
	adminDSN := fixture.GuardedDB(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	regDSN := fixture.GuardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	verifyDSN := fixture.GuardedDB(t, "TEST_IDENTITY_VERIFIER_URL", "mamony_identity_verifier_runtime")
	deliveryDSN := fixture.GuardedDB(t, "TEST_IDENTITY_DELIVERY_URL", "mamony_identity_delivery_runtime")
	base, _ := url.Parse(adminDSN)
	for _, dsn := range []string{regDSN, verifyDSN, deliveryDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != base.Host || u.Path != base.Path {
			t.Fatal("test roles did not target one isolated disposable DB")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	open := func(dsn, role string) *platformpg.Pool {
		t.Helper()
		pool, err := platformpg.Open(ctx, platformpg.Config{DSN: dsn, RuntimeRole: role, MaxConns: 4})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		return pool
	}
	reg, err := identitypg.NewRegistrationStore(open(regDSN, "mamony_identity_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	proof, err := identitypg.NewProofStore(open(verifyDSN, "mamony_identity_verifier_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := identitypg.NewDeliveryStore(open(deliveryDSN, "mamony_identity_delivery_runtime"))
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := mailcrypto.NewKeyring("fixture-key", map[string][]byte{
		"fixture-key": bytes.Repeat([]byte{0x83}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := usecase.NewEmailVerification(proof, keyring)
	if err != nil {
		t.Fatal(err)
	}
	subject := fixture.RandomID(t, "sub_")
	email := strings.ToLower(fixture.RandomID(t, "test_")) + "@example.test"
	pending := usecase.PendingRegistration{
		SubjectID:      subject,
		EventID:        fixture.RandomID(t, "evt_"),
		CanonicalEmail: email,
		Status:         "UNVERIFIED",
		CreatedAt:      time.Now().UTC(),
		PasswordVerifier: domain.PasswordVerifier{
			Version: 1, MemoryKiB: 65536, Iterations: 3, Parallelism: 1,
			Salt:   bytes.Repeat([]byte{0x23}, 16),
			Digest: bytes.Repeat([]byte{0x35}, 32),
		},
	}
	if err := reg.CreatePendingWithObligation(ctx, pending); err != nil {
		t.Fatal(err)
	}
	prepared, err := issuer.Prepare(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	recipient, secret := prepared.ForTrustedDelivery()
	if recipient != email || len(secret) != 43 {
		t.Fatal("issue did not bind verified email")
	}

	var requests atomic.Int32
	var responseCode atomic.Int32
	responseCode.Store(429)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		user, key, ok := r.BasicAuth()
		if !ok || user != "api" || key != "test-mailgun-domain-key" {
			t.Error("missing Mailgun Basic Auth")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if err := r.ParseMultipartForm(8192); err != nil {
			t.Error(err)
		}
		if r.FormValue("to") != email || r.FormValue("o:tracking") != "no" ||
			r.FormValue("o:deliver-within") != "5m" ||
			!strings.Contains(r.FormValue("text"), "#token="+secret) {
			t.Error("Mailgun mock did not receive the expected verification message")
		}
		status := int(responseCode.Load())
		w.WriteHeader(status)
		if status == 200 {
			_, _ = fmt.Fprint(w, `{"id":"<mock-id@mailgun.test>","message":"Queued. Thank you."}`)
		} else {
			_, _ = fmt.Fprint(w, `{"message":"synthetic provider rate limit"}`)
		}
	}))
	defer mock.Close()
	mockURL, err := url.Parse(mock.URL)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := mailgun.NewSender(mailgun.Config{
		Region:          mailgun.RegionUS,
		Domain:          "mg.example.test",
		From:            "Mamony Security <security@mg.example.test>",
		APIKey:          "test-mailgun-domain-key",
		VerificationURL: "https://app.example.test/verify-email",
	}, &http.Client{
		Timeout:   2 * time.Second,
		Transport: fixture.MailgunTransport{Origin: mockURL},
	})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := usecase.NewProofDeliveryWorker(delivery, keyring, sender)
	if err != nil {
		t.Fatal(err)
	}
	worked, err := worker.DispatchOnce(ctx)
	if err != nil || !worked || requests.Load() != 1 {
		t.Fatalf("first mocked delivery failure: worked=%v err=%v calls=%d", worked, err, requests.Load())
	}
	var challengeID string
	var status string
	var attempts int
	var encrypted []byte
	if err := admin.QueryRow(ctx, `SELECT d.challenge_id,d.status,d.attempts,d.sealed_token
		FROM mamony_identity.email_proof_deliveries d
		JOIN mamony_identity.email_verification_challenges c USING(challenge_id)
		WHERE c.subject_id=$1`, subject).
		Scan(&challengeID, &status, &attempts, &encrypted); err != nil {
		t.Fatal(err)
	}
	if status != "PENDING" || attempts != 1 || len(encrypted) != 71 ||
		bytes.Contains(encrypted, []byte(secret)) {
		t.Fatal("failed mocked delivery not stored as encrypted retry")
	}
	// The database, not the provider response, remains the retry authority.
	if _, err := admin.Exec(ctx, `UPDATE mamony_identity.email_proof_deliveries
		SET next_attempt_at=clock_timestamp()-interval '1 second'
		WHERE challenge_id=$1`, challengeID); err != nil {
		t.Fatal(err)
	}
	responseCode.Store(200)
	worked, err = worker.DispatchOnce(ctx)
	if err != nil || !worked || requests.Load() != 2 {
		t.Fatalf("retry to mock provider failed: worked=%v err=%v calls=%d", worked, err, requests.Load())
	}
	if err := admin.QueryRow(ctx, `SELECT status,attempts FROM mamony_identity.email_proof_deliveries
		WHERE challenge_id=$1`, challengeID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "ACCEPTED" || attempts != 2 {
		t.Fatal("mock provider acceptance not committed in PostgreSQL")
	}
	if err := issuer.Confirm(ctx, secret); err != nil {
		t.Fatal(err)
	}
	var active string
	if err := admin.QueryRow(ctx, `SELECT status FROM mamony_identity.subjects
		WHERE subject_id=$1`, subject).Scan(&active); err != nil || active != "ACTIVE" {
		t.Fatal("mocked email challenge could not activate persisted subject")
	}

	// Now exercise the newly executable facade/runner (rather than only
	// constructing the usecase worker directly) against the same isolated
	// PostgreSQL database and the same local Mailgun HTTP mock.
	subject2 := fixture.RandomID(t, "sub_")
	email2 := strings.ToLower(fixture.RandomID(t, "test_")) + "@example.test"
	pending2 := pending
	pending2.SubjectID = subject2
	pending2.EventID = fixture.RandomID(t, "evt_")
	pending2.CanonicalEmail = email2
	pending2.CreatedAt = time.Now().UTC()
	if err := reg.CreatePendingWithObligation(ctx, pending2); err != nil {
		t.Fatal(err)
	}
	prepared2, err := issuer.Prepare(ctx, subject2)
	if err != nil {
		t.Fatal(err)
	}
	_, secret2 := prepared2.ForTrustedDelivery()
	// No previous request is in flight; only the next local test server
	// request can observe these values.
	email, secret = email2, secret2
	responseCode.Store(200)

	runCtx, stopRun := context.WithCancel(ctx)
	runDone := make(chan error, 1)
	go func() {
		runDone <- identity.RunDelivery(runCtx, identity.DeliveryConfig{
			DSN: deliveryDSN, Region: "US", Domain: "mg.example.test",
			From:            "Mamony Security <security@mg.example.test>",
			VerificationURL: "https://app.example.test/verify-email",
			MailgunKey:      "test-mailgun-domain-key",
			ActiveKeyID:     "fixture-key",
			EnvelopeKeys:    map[string][]byte{"fixture-key": bytes.Repeat([]byte{0x83}, 32)},
			PollInterval:    100 * time.Millisecond,
			HTTPClient: &http.Client{
				Timeout:   2 * time.Second,
				Transport: fixture.MailgunTransport{Origin: mockURL},
			},
		})
	}()
	defer stopRun()

	// Poll only the authoritative DB state; the worker never reports token,
	// recipient or provider response details via its process interface.
	deadline := time.Now().Add(5 * time.Second)
	var runStatus string
	for {
		if err := admin.QueryRow(ctx, `SELECT d.status FROM mamony_identity.email_proof_deliveries d
			JOIN mamony_identity.email_verification_challenges c USING(challenge_id)
			WHERE c.subject_id=$1`, subject2).Scan(&runStatus); err != nil {
			t.Fatal(err)
		}
		if runStatus == "ACCEPTED" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("facade runner did not accept a local mock message: %s", runStatus)
		}
		time.Sleep(20 * time.Millisecond)
	}
	stopRun()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("facade runner stopped with error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("facade runner did not stop after cancellation")
	}
	if err := issuer.Confirm(ctx, secret2); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT status FROM mamony_identity.subjects
		WHERE subject_id=$1`, subject2).Scan(&active); err != nil || active != "ACTIVE" {
		t.Fatal("facade worker delivery did not permit single-use activation")
	}
}
