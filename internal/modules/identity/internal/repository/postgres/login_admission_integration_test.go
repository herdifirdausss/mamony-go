//go:build integration

package postgres

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityLoginAdmission proves database-shared, action-isolated pre-KDF
// LOGIN budgets against a disposable PostgreSQL database, including two
// independent connection pools and validly shaped NONEXISTENT email aliases.
// It never queries the credential table, runs a KDF or issues a session.
func TestIdentityLoginAdmission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	adminDSN := fixtureDSN(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	runtimeDSN := fixtureDSN(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	open := func() *platformpg.Pool {
		t.Helper()
		p, err := platformpg.Open(ctx, platformpg.Config{
			DSN: runtimeDSN, RuntimeRole: "mamony_identity_runtime", MaxConns: 4,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	storeA, err := NewAdmissionStore(open())
	if err != nil {
		t.Fatal(err)
	}
	storeB, err := NewAdmissionStore(open())
	if err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{0xe3}, 32)
	first, err := usecase.NewLoginAdmission(storeA, key)
	if err != nil {
		t.Fatal(err)
	}
	second, err := usecase.NewLoginAdmission(storeB, key)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("multiple instances cap unknown alias atomically", func(t *testing.T) {
		email := "unknown-login-budget@example.test"
		const attempts = 18
		start := make(chan struct{})
		results := make(chan bool, attempts)
		failures := make(chan error, attempts)
		for i := 0; i < attempts; i++ {
			gate := first
			if i%2 == 1 {
				gate = second
			}
			go func() {
				<-start
				allowed, err := gate.Admit(ctx, strings.ToUpper(email),
					netip.MustParseAddr("2001:db8:51:ab::9"))
				results <- allowed
				failures <- err
			}()
		}
		close(start)
		allowed := 0
		for i := 0; i < attempts; i++ {
			if <-results {
				allowed++
			}
			if err := <-failures; err != nil {
				t.Fatalf("login admission could not serialize across DB instances: %v", err)
			}
		}
		if allowed != 5 {
			t.Fatalf("nonexistent canonical alias admitted %d, expected exactly 5", allowed)
		}
		var emailCount, hitCount int
		if err := admin.QueryRow(ctx, `SELECT count(*)::int, max(hits)::int
			FROM mamony_identity.anonymous_admission_buckets
			WHERE action='LOGIN' AND scope='EMAIL_HOUR'`).Scan(&emailCount, &hitCount); err != nil ||
			emailCount < 1 || hitCount != 5 {
			t.Fatalf("durable unknown-email quota not capped: rows=%d hits=%d %v", emailCount, hitCount, err)
		}
		var accounts int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.email_aliases
			WHERE canonical_email=$1`, email).Scan(&accounts); err != nil || accounts != 0 {
			t.Fatalf("unknown login admission created an account: %d %v", accounts, err)
		}
	})
	t.Run("IPv6 subnet cap and independent actions", func(t *testing.T) {
		ip := netip.MustParseAddr("2001:db8:51:ab::beef")
		// Five accepted for the same /64 subnet above. Only three slots
		// remain per CLIENT_MINUTE across distinct email addresses.
		winners := 0
		for i := 0; i < 6; i++ {
			ok, err := second.Admit(ctx,
				fmt.Sprintf("unknown-other%d@example.test", i), ip)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				winners++
			}
		}
		if winners != 3 {
			t.Fatalf("IPv6 /64 rotation evaded client budget: %d", winners)
		}
		// A login quota must not spend a registration/resend window.
		anonymous, err := usecase.NewAnonymousAdmission(storeA, key)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := anonymous.Admit(ctx, usecase.AnonymousResend,
			"unknown-login-budget@example.test", ip)
		if err != nil || !ok {
			t.Fatalf("LOGIN incorrectly consumed independent RESEND budget: %v %v", ok, err)
		}
	})
	t.Run("global bound and privacy-safe persistent counters", func(t *testing.T) {
		// A different synthetic key isolates this test from previous LOGIN
		// buckets, even when the same PostgreSQL fixture is reused.
		global, err := usecase.NewLoginAdmission(storeB, bytes.Repeat([]byte{0xe7}, 32))
		if err != nil {
			t.Fatal(err)
		}
		winners := 0
		for i := 1; i <= 85; i++ {
			// Different IPv4 addresses and aliases mean only GLOBAL can
			// reject these attempts, independently of account existence.
			ip := netip.MustParseAddr(fmt.Sprintf("198.19.2.%d", i))
			ok, err := global.Admit(ctx, fmt.Sprintf("random%d@example.test", i), ip)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				winners++
			}
		}
		if winners < 80 || winners > 85 {
			t.Fatalf("global budget result is outside fixed-window bounds: %d", winners)
		}
		// Bind the query to the independent global test key. Fixed
		// windows may legitimately cross a minute boundary under a
		// loaded CI runner; enforce the cap PER bin and exact total
		// admitted, without assuming one wall-clock minute.
		mac := hmac.New(sha256.New, bytes.Repeat([]byte{0xe7}, 32))
		_, _ = mac.Write([]byte("mamony:identity:login-admission:v1\x00GLOBAL\x00all"))
		globalDigest := mac.Sum(nil)
		var windows, totalHits, maxHits, minBytes, invalid int
		err = admin.QueryRow(ctx, `SELECT count(*)::int,
			coalesce(sum(hits), 0)::int,
			coalesce(max(hits), 0)::int,
			coalesce(min(octet_length(key_digest)), 0)::int,
			count(*) FILTER (WHERE octet_length(key_digest)<>32)::int
			FROM mamony_identity.anonymous_admission_buckets
			WHERE action='LOGIN' AND scope='GLOBAL_MINUTE'
			  AND key_digest=$1`, globalDigest).
			Scan(&windows, &totalHits, &maxHits, &minBytes, &invalid)
		if err != nil || windows < 1 || windows > 2 || totalHits != winners ||
			maxHits > 80 || minBytes != 32 || invalid != 0 ||
			(windows == 1 && (maxHits != 80 || winners != 80)) {
			t.Fatalf("global limit or privacy violated: bins=%d total=%d allowed=%d max=%d bytes=%d invalid=%d err=%v",
				windows, totalHits, winners, maxHits, minBytes, invalid, err)
		}
		// Only the migrating role can change action and scope policy; LOGIN
		// storage must accept no arbitrary caller-defined quota vectors.
		_, err = storeA.Spend(ctx, usecase.AnonymousLogin, []usecase.AdmissionBucket{{
			Scope: "GLOBAL_MINUTE", WindowSeconds: 60, Limit: 80,
			Key: [32]byte{1},
		}})
		if !errors.Is(err, ErrStorageUnavailable) {
			t.Fatal("login store accepted an incomplete set of budget obligations")
		}
	})
}
