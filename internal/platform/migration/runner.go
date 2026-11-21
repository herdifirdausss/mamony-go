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
// untrusted database value. The two roles must not own each other's schema.
type target struct {
	schema string
	role   string
	lockID int64
}

var (
	platformTarget = target{schema: "mamony_platform", role: "mamony_platform_migrator", lockID: 731198465908223}
	identityTarget = target{schema: "mamony_identity", role: "mamony_identity_migrator", lockID: 731198465908224}
)

// A session lock serializes manifest preflight and postflight for each target.
// golang-migrate also owns its own independent lock.
func Run(ctx context.Context, dsn, dir string) error {
	return runTarget(ctx, dsn, dir, platformTarget)
}

// RunIdentity uses a different owned schema, role and advisory lock.
func RunIdentity(ctx context.Context, dsn, dir string) error {
	return runTarget(ctx, dsn, dir, identityTarget)
}

func runTarget(ctx context.Context, dsn, dir string, target target) error {
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
	if err := checkRoleFor(ctx, conn, target); err != nil {
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

func sameFiles(a, b []File) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func checkRoleFor(ctx context.Context, conn *pgx.Conn, target target) error {
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
		return fmt.Errorf("platform schema not provisioned by admin: %w", err)
	}
	if strings.TrimSpace(schemaOwner) != target.role {
		return errors.New("schema owner is not the authorized migration role")
	}
	return nil
}

// Verify checks database-committed digests against the current SQL directory.
// Missing or inconsistent history fails closed, including after an interrupted
// migration. It does not repair dirty versions or detect superuser tampering.
func Verify(ctx context.Context, conn *pgx.Conn, files []File) error {
	return verifyFor(ctx, conn, files, platformTarget.schema)
}

// VerifyIdentity checks identity migration fingerprints with the same rules.
func VerifyIdentity(ctx context.Context, conn *pgx.Conn, files []File) error {
	return verifyFor(ctx, conn, files, identityTarget.schema)
}

func verifyFor(ctx context.Context, conn *pgx.Conn, files []File, schema string) error {
	var hasVersion, hasHistory bool
	err := conn.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL, to_regclass($2) IS NOT NULL`,
		schema+".schema_migrations", schema+".migration_fingerprints").Scan(&hasVersion, &hasHistory)
	if err != nil {
		return fmt.Errorf("read migration catalogs: %w", err)
	}
	if !hasVersion {
		if hasHistory {
			return errors.New("fingerprint history exists without a schema version table")
		}
		return nil // Fresh schema; only provisioning exists.
	}
	rows, err := conn.Query(ctx, fmt.Sprintf("SELECT version, dirty FROM %s.schema_migrations", schema))
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	var version int64
	count := 0
	dirty := false
	for rows.Next() {
		if err := rows.Scan(&version, &dirty); err != nil {
			rows.Close()
			return fmt.Errorf("scan version: %w", err)
		}
		count++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if count > 1 {
		return errors.New("multiple schema version records")
	}
	if dirty {
		return errors.New("migration version is dirty; manual recovery required")
	}
	if count == 0 || version < 0 {
		if hasHistory {
			return errors.New("fingerprint history exists without a committed schema version")
		}
		return nil // golang-migrate created an empty version table.
	}
	if !hasHistory {
		return errors.New("committed version exists without fingerprint history")
	}
	expected := make(map[int64]string)
	foundCurrent := false
	for _, file := range files {
		if file.Version <= version {
			expected[file.Version] = file.Digest
		}
		if file.Version == version {
			foundCurrent = true
		}
	}
	if !foundCurrent {
		return fmt.Errorf("applied schema version %d missing from local migration files", version)
	}
	recordRows, err := conn.Query(ctx, fmt.Sprintf("SELECT version, sha256 FROM %s.migration_fingerprints", schema))
	if err != nil {
		return fmt.Errorf("query fingerprint history: %w", err)
	}
	for recordRows.Next() {
		var v int64
		var got string
		if err := recordRows.Scan(&v, &got); err != nil {
			recordRows.Close()
			return fmt.Errorf("read migration fingerprint: %w", err)
		}
		want, ok := expected[v]
		if !ok || want != got {
			recordRows.Close()
			return fmt.Errorf("fingerprint mismatch for applied migration %d", v)
		}
		delete(expected, v)
	}
	recordRows.Close()
	if err := recordRows.Err(); err != nil {
		return err
	}
	if len(expected) != 0 {
		return fmt.Errorf("%d applied migrations have no verified fingerprint", len(expected))
	}
	return nil
}
