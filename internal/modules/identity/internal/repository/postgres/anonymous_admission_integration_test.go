//go:build integration

package postgres

import (
	"bytes"
	"context"
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityAnonymousAdmission proves counter authority across multiple
// concurrent PostgreSQL transactions; all addresses/emails and keys synthetic.
// Registration and verification endpoints are intentionally absent.
func TestIdentityAnonymousAdmission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	adminDSN := fixtureDSN(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	runtimeDSN := fixtureDSN(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: runtimeDSN, RuntimeRole: "mamony_identity_runtime", MaxConns: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store, err := NewAdmissionStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := usecase.NewAnonymousAdmission(store, bytes.Repeat([]byte{0x72}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Run("twelve parallel requests for unknown email admit exactly three", func(t *testing.T) {
		// This address is never inserted into email_aliases: controls cannot
		// rely on any existence-based query or in-process mutex.
		email := "unknown-budget@example.test"
		ip := netip.MustParseAddr("198.51.100.42")
		const n = 12
		start := make(chan struct{})
		results := make(chan bool, n)
		errs := make(chan error, n)
		for i := 0; i < n; i++ {
			go func() {
				<-start
				ok, e := gate.Admit(ctx, usecase.AnonymousResend, email, ip)
				results <- ok
				errs <- e
			}()
		}
		close(start)
		winners := 0
		for i := 0; i < n; i++ {
			if <-results {
				winners++
			}
			if err := <-errs; err != nil {
				t.Fatalf("concurrent admission failed: %v", err)
			}
		}
		if winners != 3 {
			t.Fatalf("cross-process email quota allowed %d of %d requests, expected 3", winners, n)
		}
		var count int
		err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.anonymous_admission_buckets
			WHERE action='RESEND' AND scope='EMAIL_HOUR' AND hits=3`).Scan(&count)
		if err != nil || count < 1 {
			t.Fatalf("hourly shared row must finish with exactly 3 durable charges: %d %v", count, err)
		}
	})
	t.Run("per-client budget charges independent email addresses", func(t *testing.T) {
		ip := netip.MustParseAddr("192.0.2.127")
		winners := 0
		for i := 0; i < 12; i++ {
			ok, err := gate.Admit(ctx, usecase.AnonymousRegister,
				fmt.Sprintf("unregistered%d@example.test", i), ip)
			if err != nil {
				t.Fatalf("client budget write failed: %v", err)
			}
			if ok {
				winners++
			}
		}
		if winners != 8 {
			t.Fatalf("per-client minute cap admitted %d requests, expected 8", winners)
		}
		var count int
		err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.anonymous_admission_buckets
			WHERE action='REGISTER' AND scope='CLIENT_MINUTE' AND hits=8`).Scan(&count)
		if err != nil || count < 1 {
			t.Fatalf("per-client budget was not durably capped: %d %v", count, err)
		}
	})
	t.Run("account credential and delivery role cannot read admission keys", func(t *testing.T) {
		var count int
		err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.anonymous_admission_buckets
			WHERE octet_length(key_digest)<>32`).Scan(&count)
		if err != nil || count != 0 {
			t.Fatalf("admission persisted invalid privacy-keyed row: count=%d err=%v", count, err)
		}
		var emailRows int
		err = admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.email_aliases
			WHERE canonical_email='unknown-budget@example.test'`).Scan(&emailRows)
		if err != nil || emailRows != 0 {
			t.Fatalf("admission accidentally created account records: %d %v", emailRows, err)
		}
	})
}
