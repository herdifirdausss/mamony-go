//go:build integration

package postgres

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

// This test runs after TestPostgresIdentityMigration through tools/testgate.
// It rejects any direct invocation pointing to a non-disposable DB.
func TestIdentityRegistrationPersistence(t *testing.T) {
	adminDSN := fixtureDSN(t, "TEST_DATABASE_ADMIN_URL", "mamony_test_admin")
	runtimeDSN := fixtureDSN(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	adminURL, _ := url.Parse(adminDSN)
	runtimeURL, _ := url.Parse(runtimeDSN)
	if adminURL.Host != runtimeURL.Host || adminURL.Path != runtimeURL.Path {
		t.Fatal("identity runtime and admin must use exactly the same isolated database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("open disposable DB inspector: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: runtimeDSN, RuntimeRole: "mamony_identity_runtime", MaxConns: 4,
	})
	if err != nil {
		t.Fatalf("open limited Identity runtime: %v", err)
	}
	defer pool.Close()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("atomic pending enrollment", func(t *testing.T) {
		email := syntheticEmail(t)
		r := syntheticPending(t, email)
		if err := store.CreatePendingWithObligation(ctx, r); err != nil {
			t.Fatalf("insert Identity transaction: %v", err)
		}
		var count int
		err := admin.QueryRow(ctx, `SELECT count(*) FROM mamony_identity.subjects s
			JOIN mamony_identity.email_aliases a ON a.subject_id=s.subject_id
			JOIN mamony_identity.password_verifiers p ON p.subject_id=s.subject_id
			JOIN mamony_identity.security_outbox o ON o.subject_id=s.subject_id
			WHERE s.subject_id=$1 AND a.canonical_email=$2 AND
				s.status='UNVERIFIED' AND a.contact_version=1 AND
				a.verified_at IS NULL AND o.event_type='ENROLLMENT_PENDING'
				AND length(p.digest)=32 AND length(p.salt)=16`,
			r.SubjectID, r.CanonicalEmail).Scan(&count)
		if err != nil || count != 1 {
			t.Fatalf("atomic four-table enrollment missing: count=%d err=%v", count, err)
		}

		duplicate := syntheticPending(t, email)
		err = store.CreatePendingWithObligation(ctx, duplicate)
		if !errors.Is(err, usecase.ErrAliasAlreadyReserved) {
			t.Fatalf("canonical alias collision must use typed error: %v", err)
		}
		expectSubjectCount(t, ctx, admin, duplicate.SubjectID, 0)

		bad := syntheticPending(t, syntheticEmail(t))
		bad.Status = "ACTIVE"
		if err := store.CreatePendingWithObligation(ctx, bad); !errors.Is(err, ErrStorageUnavailable) {
			t.Fatalf("caller-controlled ACTIVE must be rejected: %v", err)
		}
		expectSubjectCount(t, ctx, admin, bad.SubjectID, 0)
	})

	t.Run("failed outbox insert rolls back subject alias and verifier", func(t *testing.T) {
		first := syntheticPending(t, syntheticEmail(t))
		if err := store.CreatePendingWithObligation(ctx, first); err != nil {
			t.Fatal(err)
		}
		second := syntheticPending(t, syntheticEmail(t))
		second.EventID = first.EventID // forces outbox unique violation at final write
		if err := store.CreatePendingWithObligation(ctx, second); !errors.Is(err, ErrStorageUnavailable) {
			t.Fatalf("failed outbox should fail closed: %v", err)
		}
		expectSubjectCount(t, ctx, admin, second.SubjectID, 0)
		var orphan int
		if err := admin.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM mamony_identity.email_aliases WHERE subject_id=$1) +
			(SELECT count(*) FROM mamony_identity.password_verifiers WHERE subject_id=$1)`,
			second.SubjectID).Scan(&orphan); err != nil || orphan != 0 {
			t.Fatalf("failed outbox left partial records: %d, %v", orphan, err)
		}
	})

	t.Run("concurrent same-email reservations create one subject", func(t *testing.T) {
		email := syntheticEmail(t)
		const attempts = 8
		var wg sync.WaitGroup
		results := make(chan error, attempts)
		candidates := make([]usecase.PendingRegistration, attempts)
		for i := range candidates {
			candidates[i] = syntheticPending(t, email)
		}
		for _, candidate := range candidates {
			wg.Add(1)
			go func(r usecase.PendingRegistration) {
				defer wg.Done()
				results <- store.CreatePendingWithObligation(ctx, r)
			}(candidate)
		}
		wg.Wait()
		close(results)
		success, duplicate := 0, 0
		for err := range results {
			switch {
			case err == nil:
				success++
			case errors.Is(err, usecase.ErrAliasAlreadyReserved):
				duplicate++
			default:
				t.Errorf("unexpected contention error: %v", err)
			}
		}
		if success != 1 || duplicate != attempts-1 {
			t.Fatalf("concurrent registrations success=%d duplicate=%d", success, duplicate)
		}
		var count int
		if err := admin.QueryRow(ctx,
			"SELECT count(*) FROM mamony_identity.email_aliases WHERE canonical_email=$1", email).Scan(&count); err != nil || count != 1 {
			t.Fatalf("one durable alias required: count=%d err=%v", count, err)
		}
	})

	t.Run("usecase plus limited store creates pending only", func(t *testing.T) {
		hasher := syntheticHasher{}
		// Synthetic fixture and fake verifier only; real Argon2id is tested in the credential adapter.
		enrollment, err := usecase.NewRegistration(store, hasher,
			domain.PasswordPolicy{IsBlocked: func(string, string) bool { return false }})
		if err != nil {
			t.Fatal(err)
		}
		email := syntheticEmail(t)
		password := "a synthetic non-production phrase"
		if err := enrollment.RegisterPending(ctx, email, password); err != nil {
			t.Fatalf("end-to-end internal enrollment: %v", err)
		}
		var digest []byte
		var status string
		var verified bool
		if err := admin.QueryRow(ctx, `SELECT p.digest, s.status, (a.verified_at IS NOT NULL)
			FROM mamony_identity.email_aliases a
			JOIN mamony_identity.subjects s ON s.subject_id=a.subject_id
			JOIN mamony_identity.password_verifiers p ON p.subject_id=a.subject_id
			WHERE a.canonical_email=$1`, email).Scan(&digest, &status, &verified); err != nil {
			t.Fatal(err)
		}
		if len(digest) != 32 || bytes.Equal(digest, []byte(password)) || verified || status != "UNVERIFIED" {
			t.Fatal("KDF or activation state violates pending enrollment contract")
		}
	})
}

func fixtureDSN(t *testing.T, env, user string) string {
	t.Helper()
	raw := os.Getenv(env)
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") ||
		u.User == nil || u.User.Username() != user ||
		(u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") ||
		!strings.HasPrefix(u.Path, "/mamony_test_") ||
		strings.Contains(strings.TrimPrefix(u.Path, "/"), "/") ||
		len(u.Query()) != 1 || u.Query().Get("sslmode") != "disable" || u.Fragment != "" {
		t.Fatalf("%s must target the isolated loopback mamony_test_ database", env)
	}
	if _, ok := u.User.Password(); !ok {
		t.Fatalf("%s must have synthetic credentials", env)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		t.Fatalf("%s requires an explicit valid port", env)
	}
	return raw
}

func randomID(t *testing.T, prefix string) string {
	t.Helper()
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf)
}

func syntheticEmail(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%s@example.test", strings.ToLower(randomID(t, "user_")))
}

func syntheticPending(t *testing.T, email string) usecase.PendingRegistration {
	t.Helper()
	return usecase.PendingRegistration{
		SubjectID: randomID(t, "sub_"),
		EventID: randomID(t, "evt_"),
		CanonicalEmail: email,
		Status: "UNVERIFIED",
		CreatedAt: time.Now().UTC(),
		PasswordVerifier: domain.PasswordVerifier{
			Version: 1,
			MemoryKiB: 65536,
			Iterations: 3,
			Parallelism: 1,
			Salt: []byte("synthetic-salt01"),
			Digest: bytes.Repeat([]byte{0x5a}, 32),
		},
	}
}

func expectSubjectCount(t *testing.T, ctx context.Context, admin *pgx.Conn, subject string, want int) {
	t.Helper()
	var got int
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM mamony_identity.subjects WHERE subject_id=$1", subject).Scan(&got); err != nil || got != want {
		t.Fatalf("subject rollback mismatch: got=%d want=%d err=%v", got, want, err)
	}
}

type syntheticHasher struct{}

func (syntheticHasher) Hash(_ context.Context, _ string) (domain.PasswordVerifier, error) {
	return domain.PasswordVerifier{
		Version: 1, MemoryKiB: 65536, Iterations: 3, Parallelism: 1,
		Salt: []byte("synthetic-salt01"), Digest: bytes.Repeat([]byte{0x5a}, 32),
	}, nil
}
