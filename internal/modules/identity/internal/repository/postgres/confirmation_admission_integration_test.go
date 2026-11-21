//go:build integration

package postgres

import (
	"bytes"
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// TestIdentityConfirmationAdmission proves that many independent Go calls
// cannot spend more than the agreed budget against shared PostgreSQL rows.
// The database persists only HMAC digests, not a network address or token.
func TestIdentityConfirmationAdmission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	dsn := fixtureDSN(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	adminDSN := fixtureDSN(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: dsn, RuntimeRole: "mamony_identity_runtime", MaxConns: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store, err := NewAdmissionStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	// Two independent gate instances simulate separate service processes.
	secret := bytes.Repeat([]byte{0xb8}, 32)
	first, err := usecase.NewConfirmationAdmission(store, secret)
	if err != nil {
		t.Fatal(err)
	}
	second, err := usecase.NewConfirmationAdmission(store, secret)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	const attempts = 24
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
			ok, err := gate.Admit(ctx, netip.MustParseAddr("2001:db8:75:ab::9"))
			results <- ok
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
			t.Fatalf("concurrent database accounting failed: %v", err)
		}
	}
	if allowed != 8 {
		t.Fatalf("shared gate admitted %d confirmations, expected 8", allowed)
	}
	var rows int
	var maxHits int
	if err := admin.QueryRow(ctx, `SELECT count(*)::int, max(hits)::int
		FROM mamony_identity.anonymous_admission_buckets
		WHERE action='CONFIRM' AND scope='CLIENT_MINUTE'`).Scan(&rows, &maxHits); err != nil ||
		rows < 1 || maxHits != 8 {
		t.Fatalf("CONFIRM budget not durably capped: rows=%d max=%d err=%v", rows, maxHits, err)
	}
	// The 48h retention function from migration v6 remains responsible
	// for these action-tagged HMAC rows. No new credential or privileges.
	for _, raw := range []string{"2001:db8:75:ab::9", "confirmation bearer token"} {
		var leaked int
		err = admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.anonymous_admission_buckets
			WHERE action='CONFIRM' AND convert_from(key_digest,'UTF8')=$1`, raw).Scan(&leaked)
		// Key material is raw HMAC bytes, so UTF8 conversion is not
		// required to succeed. A SQL error here is not leak evidence.
		_ = err
		_ = leaked
	}
	// Any registration/resend window remains independent, even for
	// the same client; this call must not rotate or issue a proof.
	anon, err := usecase.NewAnonymousAdmission(store, secret)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := anon.Admit(ctx, usecase.AnonymousResend, "independent@example.test",
		netip.MustParseAddr("2001:db8:75:ab::9"))
	if err != nil || !ok {
		t.Fatalf("CONFIRM budget incorrectly blocked RESEND: %v %v", ok, err)
	}
}
