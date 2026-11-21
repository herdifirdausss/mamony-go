//go:build integration

package postgrestest

import (
	"context"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresRuntimePoolIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, postgres.Config{
		DSN:         os.Getenv("TEST_DATABASE_URL"),
		RuntimeRole: "mamony_test_runtime",
		MaxConns:    1,
	})
	if err != nil {
		t.Fatalf("open least-privilege runtime pool: %v", err)
	}
	defer pool.Close()

	var firstPID int32
	if err := pool.WithTenantTx(ctx, "tenant_a", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&firstPID); err != nil {
			return err
		}
		for _, tc := range []struct {
			id   int
			want int
		}{{1, 1}, {2, 0}} {
			var count int
			if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM mamony_test_fixture.records WHERE id=$1", tc.id).Scan(&count); err != nil {
				return err
			}
			if count != tc.want {
				t.Errorf("tenant A: id=%d got=%d want=%d", tc.id, count, tc.want)
			}
		}
		for _, stmt := range []string{
			"UPDATE mamony_test_fixture.records SET note='forbidden' WHERE id=2",
			"DELETE FROM mamony_test_fixture.records WHERE id=2",
		} {
			tag, err := tx.Exec(ctx, stmt)
			if err != nil || tag.RowsAffected() != 0 {
				t.Errorf("cross-tenant mutation must affect zero rows: %v %v", tag, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("tenant A transaction: %v", err)
	}

	// MaxConns=1 and backend PID equality prove this is the same physical
	// PostgreSQL session after commit, not two unrelated connections.
	if err := pool.WithTenantTx(ctx, "tenant_b", func(tx pgx.Tx) error {
		var pid int32
		var selected string
		var countA, countB int
		if err := tx.QueryRow(ctx, "SELECT pg_backend_pid(), current_setting('app.tenant_id', true)").Scan(&pid, &selected); err != nil {
			return err
		}
		if pid != firstPID || selected != "tenant_b" {
			t.Errorf("pooled connection leaked tenant context: pid=%d vs %d tenant=%q", pid, firstPID, selected)
		}
		if err := tx.QueryRow(ctx, "SELECT COUNT(*) FILTER (WHERE id=1), COUNT(*) FILTER (WHERE id=2) FROM mamony_test_fixture.records").Scan(&countA, &countB); err != nil {
			return err
		}
		if countA != 0 || countB != 1 {
			t.Errorf("tenant B visibility: tenantA=%d tenantB=%d", countA, countB)
		}
		return nil
	}); err != nil {
		t.Fatalf("tenant B transaction: %v", err)
	}

	for _, tc := range []struct {
		name string
		sql  string
	}{
		{"cross-tenant insert", "INSERT INTO mamony_test_fixture.records(id,tenant_id,note) VALUES (801,'tenant_b','forbidden')"},
		{"cross-tenant reassign", "UPDATE mamony_test_fixture.records SET tenant_id='tenant_b' WHERE id=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := pool.WithTenantTx(ctx, "tenant_a", func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, tc.sql)
				return err
			})
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
				t.Fatalf("expected real PostgreSQL 42501, got %v", err)
			}
		})
	}

	rollback := errors.New("force application rollback")
	err = pool.WithTenantTx(ctx, "tenant_a", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO mamony_test_fixture.records(id,tenant_id,note) VALUES (802,'tenant_a','rolled-back')"); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("callback error not preserved: %v", err)
	}
	if err := pool.WithTenantTx(ctx, "tenant_a", func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM mamony_test_fixture.records WHERE id=802").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("rollback leaked persisted data: %d", count)
		}
		return nil
	}); err != nil {
		t.Fatalf("post-rollback transaction: %v", err)
	}

	for _, tenant := range []string{"", "tenant a", "tenant_a';SELECT 1;--"} {
		if err := pool.WithTenantTx(ctx, tenant, func(pgx.Tx) error { return nil }); err == nil {
			t.Errorf("invalid tenant accepted: %q", tenant)
		}
	}
	canceled, cancelNow := context.WithCancel(ctx)
	cancelNow()
	if err := pool.WithTenantTx(canceled, "tenant_a", func(pgx.Tx) error {
		t.Error("canceled transaction invoked callback")
		return nil
	}); err == nil {
		t.Fatal("canceled transaction accepted")
	}
	// A canceled/failed request must not poison the same limited pool.
	if err := pool.WithTenantTx(ctx, "tenant_b", func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM mamony_test_fixture.records WHERE id=2").Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("tenant B row inaccessible after cancellation: %d", count)
		}
		return nil
	}); err != nil {
		t.Fatalf("connection unusable after cancellation: %v", err)
	}
}

func TestPostgresRuntimePoolRejectsElevatedRole(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// This disposable role can log in but has CREATEDB. A real PostgreSQL
	// catalog check, rather than matching a mock role string, must refuse it.
	runtimeURL, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	runtimeURL.User = url.UserPassword("mamony_test_elevated_runtime", "local-test-elevated")
	pool, err := postgres.Open(ctx, postgres.Config{
		DSN:         runtimeURL.String(),
		RuntimeRole: "mamony_test_elevated_runtime",
		MaxConns:    1,
	})
	if pool != nil {
		pool.Close()
	}
	if err == nil {
		t.Fatal("actual CREATEDB role was accepted as runtime")
	}

	pool, err = postgres.Open(ctx, postgres.Config{
		DSN:         os.Getenv("TEST_DATABASE_ADMIN_URL"),
		RuntimeRole: "mamony_test_admin",
		MaxConns:    1,
	})
	if pool != nil {
		pool.Close()
	}
	if err == nil {
		t.Fatal("bootstrap admin credentials accepted for runtime pool")
	}
}
