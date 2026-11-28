package migration

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

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

// VerifyLedger verifies Ledger migration history using its own manifest.
func VerifyLedger(ctx context.Context, conn *pgx.Conn, files []File) error {
	return verifyFor(ctx, conn, files, ledgerTarget.schema)
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
