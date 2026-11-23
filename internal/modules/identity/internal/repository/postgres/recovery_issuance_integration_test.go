//go:build integration

package postgres

import (
	"bytes"
	"context"
	"net/netip"
	"net/url"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

func TestIdentityRecoveryIssuance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	adminDSN := fixtureDSN(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	admissionDSN := fixtureDSN(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	issuerDSN := fixtureDSN(t, "TEST_IDENTITY_RECOVERY_ISSUER_URL", "mamony_identity_recovery_issuer_runtime")
	adminURL, _ := url.Parse(adminDSN)
	for _, dsn := range []string{admissionDSN, issuerDSN} {
		u, _ := url.Parse(dsn)
		if u.Host != adminURL.Host || u.Path != adminURL.Path {
			t.Fatal("recovery test roles must target one disposable database")
		}
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	issuerPool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: issuerDSN, RuntimeRole: "mamony_identity_recovery_issuer_runtime", MaxConns: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer issuerPool.Close()
	admissionPool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: admissionDSN, RuntimeRole: "mamony_identity_runtime", MaxConns: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer admissionPool.Close()
	issuer, err := NewRecoveryIssuerStore(issuerPool)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := NewAdmissionStore(admissionPool)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := usecase.NewRecoveryAdmission(admission, bytes.Repeat([]byte{0x9c}, 32))
	if err != nil {
		t.Fatal(err)
	}
	subject := randomID(t, "sub_")
	email := syntheticEmail(t)
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES ($1,clock_timestamp())",
		subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.email_aliases(canonical_email,subject_id,verified_at,created_at) "+
			"VALUES ($1,$2,clock_timestamp(),clock_timestamp())", email, subject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.subjects SET status='ACTIVE' WHERE subject_id=$1", subject); err != nil {
		t.Fatal(err)
	}
	mk := func(n byte) usecase.RecoveryIssue {
		return usecase.RecoveryIssue{
			CanonicalEmail: email, ChallengeID: randomID(t, "rcv_"),
			TokenDigest: [32]byte{n},
			KeyID:       "synthetic-aead", SealedToken: bytes.Repeat([]byte{n}, 72),
		}
	}
	first := mk(1)
	issued, err := issuer.Issue(ctx, first)
	if err != nil || !issued {
		t.Fatalf("eligible subject was not issued a sealed challenge: %v %v", issued, err)
	}
	var total int
	var epoch, version int64
	var pending string
	var expirySeconds float64
	err = admin.QueryRow(ctx, `SELECT count(*),max(c.subject_epoch),max(c.contact_version),
		max(d.status),max(EXTRACT(EPOCH FROM (c.expires_at-c.requested_at)))
		FROM mamony_identity.password_recovery_challenges c
		JOIN mamony_identity.password_recovery_deliveries d USING(challenge_id)
		WHERE c.subject_id=$1 AND c.canonical_email=$2 AND c.consumed_at IS NULL
		  AND c.revoked_at IS NULL AND c.token_digest=$3
		  AND d.key_id='synthetic-aead' AND octet_length(d.sealed_token)=72`,
		subject, email, first.TokenDigest[:]).Scan(&total, &epoch, &version, &pending, &expirySeconds)
	if err != nil || total != 1 || epoch <= 0 || version != 1 ||
		pending != "PENDING" || expirySeconds != 900 {
		t.Fatalf("encrypted recovery obligation not atomic: n=%d epoch=%d v=%d status=%s ttl=%.0f err=%v",
			total, epoch, version, pending, expirySeconds, err)
	}
	second := mk(2)
	issued, err = issuer.Issue(ctx, second)
	if err != nil || issued {
		t.Fatalf("cooldown incorrectly issued second challenge: %v %v", issued, err)
	}
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM mamony_identity.password_recovery_challenges WHERE subject_id=$1",
		subject).Scan(&total); err != nil || total != 1 {
		t.Fatalf("cooldown created a challenge or revoked existing: %d %v", total, err)
	}
	unknown := mk(3)
	unknown.CanonicalEmail = syntheticEmail(t)
	issued, err = issuer.Issue(ctx, unknown)
	if err != nil || issued {
		t.Fatalf("unknown alias incorrectly accepted: %v %v", issued, err)
	}
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM mamony_identity.password_recovery_challenges WHERE challenge_id=$1",
		unknown.ChallengeID).Scan(&total); err != nil || total != 0 {
		t.Fatalf("unknown alias created recovery proof: %d %v", total, err)
	}
	// Move past 60s cooldown, then induce a late PK collision after the
	// function would have revoked the earlier proof. Rollback must leave
	// the original challenge LIVE and no partial second delivery.
	if _, err := admin.Exec(ctx,
		"UPDATE mamony_identity.password_recovery_challenges "+
			"SET requested_at=clock_timestamp()-interval '65 seconds' WHERE challenge_id=$1",
		first.ChallengeID); err != nil {
		t.Fatal(err)
	}
	conflict := mk(4)
	conflict.ChallengeID = first.ChallengeID
	issued, err = issuer.Issue(ctx, conflict)
	if err == nil || issued {
		t.Fatal("late conflict did not roll back the entire recovery issuance")
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.password_recovery_challenges
		WHERE subject_id=$1 AND revoked_at IS NULL AND consumed_at IS NULL`,
		subject).Scan(&total); err != nil || total != 1 {
		t.Fatalf("failed late insert invalidated valid original proof: %d %v", total, err)
	}
	// A separate suspended account cannot request recovery by mailbox.
	inactiveSubject := randomID(t, "sub_")
	inactiveEmail := syntheticEmail(t)
	if _, err := admin.Exec(ctx,
		"INSERT INTO mamony_identity.subjects(subject_id,created_at) VALUES($1,clock_timestamp())",
		inactiveSubject); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO mamony_identity.email_aliases
		(canonical_email,subject_id,verified_at,created_at)
		VALUES($1,$2,clock_timestamp(),clock_timestamp())`, inactiveEmail, inactiveSubject); err != nil {
		t.Fatal(err)
	}
	inactive := mk(5)
	inactive.CanonicalEmail = inactiveEmail
	if issued, err := issuer.Issue(ctx, inactive); err != nil || issued {
		t.Fatalf("inactive account was eligible for reset: %v %v", issued, err)
	}
	// The HMAC-keyed global/client/alias gate rejects after two requests
	// from one email even when that email never appears in Identity.
	unknownEmail := syntheticEmail(t)
	peer := netip.MustParseAddr("2001:db8:bb:19::7")
	for i := 0; i < 5; i++ {
		allowed, err := gate.Admit(ctx, unknownEmail, peer)
		if err != nil || allowed != (i < 2) {
			t.Fatalf("recovery per-alias quota broke at attempt %d: %v %v", i, allowed, err)
		}
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.anonymous_admission_buckets
		WHERE action='RECOVERY' AND scope='EMAIL_HOUR' AND hits=2
		  AND octet_length(key_digest)=32`).Scan(&total); err != nil || total < 1 {
		t.Fatalf("recovery admission did not persist privacy-preserving budget: %d %v", total, err)
	}
}
