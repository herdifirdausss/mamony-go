package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
)

// Migration targets are fixed by code, never constructed from HTTP or an
// untrusted database value. Roles must not own one another's schemas.
type migrationTarget struct {
	schema string
	role   string
	lockID int64
}

var (
	platformTarget = migrationTarget{schema: "mamony_platform", role: "mamony_platform_migrator", lockID: 731198465908223}
	identityTarget = migrationTarget{schema: "mamony_identity", role: "mamony_identity_migrator", lockID: 731198465908224}
	ledgerTarget   = migrationTarget{schema: "mamony_ledger", role: "mamony_ledger_migrator", lockID: 731198465908225}
)

// A session lock serializes manifest preflight and postflight for each target.
// golang-migrate also owns its own independent lock.
func Run(ctx context.Context, dsn, dir string) error {
	return runMigrationTarget(ctx, dsn, dir, platformTarget)
}

// RunIdentity uses a different owned schema, role and advisory lock.
func RunIdentity(ctx context.Context, dsn, dir string) error {
	return runMigrationTarget(ctx, dsn, dir, identityTarget)
}

// RunLedger is the fixed Ledger migration target. K2a calls it only from
// its disposable integration lab; no application or command wiring exists.
func RunLedger(ctx context.Context, dsn, dir string) error {
	return runMigrationTarget(ctx, dsn, dir, ledgerTarget)
}

func runMigrationTarget(ctx context.Context, dsn, dir string, target migrationTarget) error {
	if dsn == "" {
		return errors.New("migration credentials not provided")
	}
	files, err := loadForSchema(dir, target.schema)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect as migration role: %w", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if err := checkMigrationTarget(ctx, conn, target); err != nil {
		return err
	}

	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", target.lockID).Scan(&locked); err != nil {
		return fmt.Errorf("acquire fingerprint lock: %w", err)
	}
	if !locked {
		return errors.New("a migration verification is already running")
	}
	defer func() {
		var unlocked bool
		_ = conn.QueryRow(context.Background(), "SELECT pg_advisory_unlock($1)", target.lockID).Scan(&unlocked)
	}()

	if err := verifyFor(ctx, conn, files, target.schema); err != nil {
		return fmt.Errorf("before migration: %w", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open migration driver: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(2)
	db.SetConnMaxLifetime(2 * time.Minute)
	driver, err := migratepgx.WithInstance(db, &migratepgx.Config{
		SchemaName:       target.schema,
		MigrationsTable:  "schema_migrations",
		StatementTimeout: 45 * time.Second,
		// False ensures each SQL file is executed in one server-side
		// batch. PostgreSQL treats its statements as one implicit
		// transaction unless the file contains explicit tx control.
		MultiStatementEnabled: false,
	})
	if err != nil {
		return fmt.Errorf("initialize golang-migrate: %w", err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve migration directory: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance((&url.URL{Scheme: "file", Path: abs}).String(), "pgx5", driver)
	if err != nil {
		return fmt.Errorf("initialize migration source: %w", err)
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration failed (manual dirty-state review required): %w", err)
	}
	// Files must not change between the preflight read and execution.
	reloaded, err := loadForSchema(dir, target.schema)
	if err != nil || !sameFiles(files, reloaded) {
		return errors.New("migration SQL changed during execution; inspect applied state")
	}
	if err := verifyFor(ctx, conn, files, target.schema); err != nil {
		return fmt.Errorf("after migration: %w", err)
	}
	fmt.Printf("Migrations verified: %d (%s schema)\n", files[len(files)-1].Version, target.schema)
	return nil
}

func checkMigrationTarget(ctx context.Context, conn *pgx.Conn, target migrationTarget) error {
	var name string
	var safe bool
	err := conn.QueryRow(ctx, `SELECT rolname,
		NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)
		FROM pg_roles WHERE rolname = current_user`).Scan(&name, &safe)
	if err != nil {
		return fmt.Errorf("inspect migration role: %w", err)
	}
	if name != target.role || !safe {
		return errors.New("migration connection must use the dedicated unprivileged schema migrator role")
	}
	var schemaOwner string
	err = conn.QueryRow(ctx, "SELECT pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname = $1", target.schema).Scan(&schemaOwner)
	if err != nil {
		return fmt.Errorf("migration schema %q not provisioned by admin: %w", target.schema, err)
	}
	if strings.TrimSpace(schemaOwner) != target.role {
		return errors.New("schema owner is not the authorized migration role")
	}
	return nil
}

