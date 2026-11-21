// Package postgres provides bounded runtime-only PostgreSQL primitives.
// It does not authorize users, identify tenants, or expose a payment schema.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	runtimeName = regexp.MustCompile(`^mamony_[a-z][a-z0-9_]*_runtime$`)
	tenantName  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
)

// Config holds an externally provisioned, limited runtime credential. Its URL
// must never be logged or passed to an HTTP response.
type Config struct {
	DSN         string
	RuntimeRole string
	MaxConns    int32
}

type Pool struct {
	raw *pgxpool.Pool
}

// Open validates connection bounds, requires TLS verification outside a
// disposable local fixture, and verifies the *actual* PostgreSQL login role.
// A privileged role must never be accepted, even if a caller misconfigures it.
func Open(ctx context.Context, cfg Config) (*Pool, error) {
	if err := validate(cfg); err != nil {
		return nil, err
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL runtime configuration")
	}
	// pgx also reads libpq PG* environment variables (including services and
	// fallback hosts). Do not allow ambient process configuration to silently
	// redirect an explicitly supplied runtime endpoint or credential.
	u, _ := url.Parse(cfg.DSN) // Validated by validate above.
	port, _ := strconv.Atoi(u.Port())
	password, _ := u.User.Password()
	if int(poolCfg.ConnConfig.Port) != port {
		return nil, errors.New("runtime PostgreSQL port does not match validated URL")
	}
	poolCfg.ConnConfig.Host = u.Hostname()
	poolCfg.ConnConfig.User = u.User.Username()
	poolCfg.ConnConfig.Password = password
	poolCfg.ConnConfig.Database = strings.TrimPrefix(u.Path, "/")
	poolCfg.ConnConfig.Fallbacks = nil
	poolCfg.MinConns = 0
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MaxConnIdleTime = time.Minute
	poolCfg.MaxConnLifetime = 15 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second
	poolCfg.ConnConfig.ConnectTimeout = 5 * time.Second
	// Overwrite inherited/user-specified session options. Untrusted SQL must
	// not supply role, search_path or arbitrary initialization parameters.
	poolCfg.ConnConfig.RuntimeParams = map[string]string{
		"statement_timeout":                   "5000",
		"idle_in_transaction_session_timeout": "5000",
		"search_path":                         "pg_catalog",
	}
	raw, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, errors.New("cannot initialize PostgreSQL runtime pool")
	}
	if err := raw.Ping(ctx); err != nil {
		raw.Close()
		return nil, errors.New("PostgreSQL runtime database is unavailable")
	}
	var name string
	var safe bool
	err = raw.QueryRow(ctx, `SELECT current_user,
		NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)
		AND NOT EXISTS (
			SELECT 1 FROM pg_roles privileged
			WHERE (privileged.rolsuper OR privileged.rolbypassrls
				OR privileged.rolcreaterole
				OR privileged.rolname LIKE 'mamony_%_migrator'
				OR privileged.rolname LIKE 'mamony_%_owner')
				AND pg_has_role(current_user, privileged.oid, 'MEMBER')
		)
		FROM pg_roles WHERE rolname = current_user`).Scan(&name, &safe)
	if err != nil || name != cfg.RuntimeRole || !safe {
		raw.Close()
		return nil, errors.New("PostgreSQL runtime privilege check failed")
	}
	return &Pool{raw: raw}, nil
}

func validate(cfg Config) error {
	if cfg.MaxConns < 1 || cfg.MaxConns > 8 {
		return errors.New("runtime pool MaxConns must be between 1 and 8")
	}
	if !runtimeName.MatchString(cfg.RuntimeRole) || cfg.DSN == "" {
		return errors.New("runtime database configuration must specify an unprivileged runtime role and URL")
	}
	u, err := url.Parse(cfg.DSN)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return errors.New("invalid PostgreSQL runtime URL")
	}
	if u.Hostname() == "" || strings.Contains(u.Hostname(), ",") ||
		u.Path == "" || u.Path == "/" || strings.Contains(strings.TrimPrefix(u.Path, "/"), "/") ||
		u.User == nil || u.User.Username() != cfg.RuntimeRole ||
		u.Fragment != "" || u.RawFragment != "" {
		return errors.New("invalid PostgreSQL runtime endpoint or role")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return errors.New("runtime PostgreSQL URL requires an explicit valid port")
	}
	if password, ok := u.User.Password(); !ok || password == "" {
		return errors.New("runtime database password is required")
	}
	q := u.Query()
	if len(q) != 1 || len(q["sslmode"]) != 1 {
		return errors.New("runtime URL must specify only an explicit sslmode")
	}
	mode := q.Get("sslmode")
	if mode == "disable" {
		database := strings.TrimPrefix(u.Path, "/")
		if (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") ||
			!strings.HasPrefix(database, "mamony_test_") {
			return errors.New("unencrypted PostgreSQL connections require a disposable loopback test database")
		}
	} else if mode != "verify-full" {
		return errors.New("runtime PostgreSQL connections must use sslmode=verify-full")
	}
	return nil
}

func validateTenantID(tenant string) error {
	if !tenantName.MatchString(tenant) {
		return errors.New("missing or invalid authorized tenant identifier")
	}
	return nil
}

// WithTx runs a bounded transaction against platform-owned data. It does not
// accept a caller-provided tenant, authorize an HTTP subject, or broaden SQL
// grants. The identity registration repository uses its dedicated DB role.
func (p *Pool) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	if p == nil || p.raw == nil || fn == nil {
		return errors.New("runtime pool and operation are required")
	}
	tx, err := p.raw.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx)
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction (outcome may be unknown): %w", err)
	}
	return nil
}

// WithTenantTx runs one transaction with transaction-local tenant context.
// The caller MUST authenticate and authorize the subject/action/resource/tenant
// before supplying authorizedTenantID. This primitive does not do that work,
// and PostgreSQL app.tenant_id is NOT an identity authority.
// Never return tx, start network I/O in fn, or retry an ambiguous commit here.
func (p *Pool) WithTenantTx(
	ctx context.Context,
	authorizedTenantID string,
	fn func(pgx.Tx) error,
) error {
	if p == nil || p.raw == nil || fn == nil {
		return errors.New("runtime pool and tenant operation are required")
	}
	if err := validateTenantID(authorizedTenantID); err != nil {
		return err
	}
	tx, err := p.raw.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tenant transaction: %w", err)
	}
	// Rollback is safe after Commit and prevents leaking an aborted session.
	// A canceled request receives bounded cleanup rather than a background hang.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx)
	}()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", authorizedTenantID); err != nil {
		return fmt.Errorf("set transaction tenant: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		// A failed/ambiguous commit must be reconciled by its owning usecase.
		return fmt.Errorf("commit tenant transaction (outcome may be unknown): %w", err)
	}
	return nil
}

// Close releases idle and active connections once the owning application stops.
func (p *Pool) Close() {
	if p != nil && p.raw != nil {
		p.raw.Close()
	}
}
