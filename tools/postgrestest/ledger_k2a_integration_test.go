//go:build integration

package postgrestest

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/ledger"
	"github.com/herdifirdausss/mamony-go/internal/platform/migration"
	"github.com/herdifirdausss/mamony-go/internal/platform/reference"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	k2aDefinition = "mamony-currency-v2:USD:1"
	k2aUnit       = "mamony-unit-v1:USD:1"
	k2aRelease    = "fnd02d-2026-11-25.1"
	k2aDigest     = "9c335b6f3362735bde0523de78153e8525c56d0d8ad04e4133155ca6c01cbcf1"
	k2aTables     = "mamony_ledger.currency_definitions, mamony_ledger.books, mamony_ledger.account_types, mamony_ledger.accounts"
	k2aCurrency   = "INSERT INTO mamony_ledger.currency_definitions VALUES ($1,$2,$3,$4,$5,$6)"
	k2aBook       = "INSERT INTO mamony_ledger.books VALUES ($1,$2)"
	k2aType       = "INSERT INTO mamony_ledger.account_types VALUES ($1,$2,$3,$4)"
	k2aAccount    = "INSERT INTO mamony_ledger.accounts VALUES ($1,$2,$3,$4,$5,$6,$7)"
)

func k2aDir() string {
	return filepath.Join("..", "..", "db", "migrations", "ledger")
}

func k2aGoldenCurrency() []any {
	return []any{k2aDefinition, "USD", int16(2), k2aUnit, k2aRelease, k2aDigest}
}

func k2aExpectedRows() []string {
	return []string{
		"C|" + k2aDefinition + "|USD|2|" + k2aUnit + "|" + k2aRelease + "|" + k2aDigest,
		"B|BOOK-001|" + k2aDefinition,
		"T|COLLECTION_CASH|ASSET|D|0",
		"T|CONTRIBUTED_CAPITAL|EQUITY|C|0",
		"T|CUSTOMER_AVAILABLE|LIABILITY|C|0",
		"A|ACC-001|BOOK-001|COLLECTION_CASH|SYN-SYSTEM|ACTIVE|0|0",
		"A|ACC-002|BOOK-001|CONTRIBUTED_CAPITAL|SYN-SYSTEM|ACTIVE|0|0",
		"A|ACC-003|BOOK-001|CUSTOMER_AVAILABLE|SYN-001|ACTIVE|0|0",
		"A|ACC-004|BOOK-001|CUSTOMER_AVAILABLE|SYN-002|ACTIVE|0|0",
	}
}

func k2aProvision(t *testing.T) (*k2aLab, *pgx.Conn) {
	t.Helper()
	l := k2aStart(t)
	admin := l.roles(t)
	l.exec(t, admin, "CREATE SCHEMA mamony_ledger AUTHORIZATION "+k2aMigrator)
	l.exec(t, admin, "REVOKE ALL ON SCHEMA mamony_ledger FROM PUBLIC")
	l.exec(t, admin, "CREATE SCHEMA k2a_other AUTHORIZATION "+k2aOther)
	l.exec(t, admin, "REVOKE ALL ON SCHEMA k2a_other FROM PUBLIC")
	l.exec(t, admin, "CREATE TABLE k2a_other.canary (id integer PRIMARY KEY); INSERT INTO k2a_other.canary VALUES (1); ALTER TABLE k2a_other.canary OWNER TO "+k2aOther)
	return l, admin
}

func k2aMigrate(t *testing.T, l *k2aLab) {
	t.Helper()
	_ = l.connect(t, k2aMigrator)
	if err := migration.RunLedger(l.ctx, l.dsn(k2aMigrator), k2aDir()); err != nil {
		t.Fatal("Ledger migration failed")
	}
}

func k2aOpenFixture(t *testing.T, l *k2aLab, admin *pgx.Conn) (*pgx.Conn, *pgx.Conn) {
	t.Helper()
	l.exec(t, admin, "GRANT USAGE ON SCHEMA mamony_ledger TO "+k2aLoader+", "+k2aReader)
	l.exec(t, admin, "GRANT INSERT ON "+k2aTables+" TO "+k2aLoader)
	l.exec(t, admin, "GRANT SELECT ON "+k2aTables+" TO "+k2aReader)
	return l.connect(t, k2aLoader), l.connect(t, k2aReader)
}

func k2aSeed(t *testing.T, l *k2aLab, loader *pgx.Conn) {
	t.Helper()
	tx, err := loader.Begin(l.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(l.ctx) }()
	insert := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(l.ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	insert(k2aCurrency, k2aGoldenCurrency()...)
	insert(k2aBook, "BOOK-001", k2aDefinition)
	insert(k2aType, "COLLECTION_CASH", "ASSET", "D", int64(0))
	insert(k2aType, "CONTRIBUTED_CAPITAL", "EQUITY", "C", int64(0))
	insert(k2aType, "CUSTOMER_AVAILABLE", "LIABILITY", "C", int64(0))
	for i, row := range [][]string{
		{"COLLECTION_CASH", "SYN-SYSTEM"},
		{"CONTRIBUTED_CAPITAL", "SYN-SYSTEM"},
		{"CUSTOMER_AVAILABLE", "SYN-001"},
		{"CUSTOMER_AVAILABLE", "SYN-002"},
	} {
		insert(k2aAccount, fmt.Sprintf("ACC-%03d", i+1), "BOOK-001", row[0], row[1], "ACTIVE", int64(0), int64(0))
	}
	if err := tx.Commit(l.ctx); err != nil {
		t.Fatal(err)
	}
}

func k2aSeal(t *testing.T, l *k2aLab, admin *pgx.Conn) {
	t.Helper()
	l.exec(t, admin, "REVOKE INSERT ON "+k2aTables+" FROM "+k2aLoader)
	l.exec(t, admin, "REVOKE USAGE ON SCHEMA mamony_ledger FROM "+k2aLoader)
	var stopped bool
	if err := admin.QueryRow(l.ctx, "SELECT COALESCE(bool_and(pg_terminate_backend(pid, 1000)), true) FROM pg_stat_activity WHERE usename = $1 AND datname = current_database()", k2aLoader).Scan(&stopped); err != nil || !stopped {
		t.Fatal("fixture loader sessions not terminated")
	}
	var remaining int
	if err := admin.QueryRow(l.ctx, "SELECT count(*) FROM pg_stat_activity WHERE usename = $1 AND datname = current_database()", k2aLoader).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("fixture loader session remains after seal")
	}
}

func k2aRows(t *testing.T, l *k2aLab, reader *pgx.Conn) []string {
	t.Helper()
	var result []string
	for _, q := range []string{
		"SELECT concat_ws('|','C',definition_id,code,exponent,monetary_unit_id,source_release_id,source_catalog_sha256) FROM mamony_ledger.currency_definitions ORDER BY definition_id",
		"SELECT concat_ws('|','B',book_id,definition_id) FROM mamony_ledger.books ORDER BY book_id",
		"SELECT concat_ws('|','T',type_code,accounting_class,normal_side,floor_minor) FROM mamony_ledger.account_types ORDER BY type_code",
		"SELECT concat_ws('|','A',account_id,book_id,type_code,owner_ref,status,balance_minor,account_seq) FROM mamony_ledger.accounts ORDER BY account_id",
	} {
		rows, err := reader.Query(l.ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		values, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, values...)
	}
	return result
}

func k2aAssertRows(t *testing.T, l *k2aLab, reader *pgx.Conn, expected []string) {
	t.Helper()
	if got := k2aRows(t, l, reader); !slices.Equal(got, expected) {
		t.Fatalf("fixture mismatch: got %v; want %v", got, expected)
	}
}

func k2aExpectState(t *testing.T, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("expected SQLSTATE %s, got %v", code, err)
	}
}

func k2aReject(t *testing.T, l *k2aLab, conn *pgx.Conn, code, sql string, args ...any) {
	t.Helper()
	_, err := conn.Exec(l.ctx, sql, args...)
	k2aExpectState(t, err, code)
}

func TestPostgresLedgerK2aContainment(t *testing.T) {
	a := k2aStart(t)
	b := k2aStart(t)
	if a.id == b.id || a.database == b.database || a.port == b.port {
		t.Fatal("K2a clusters are not independent")
	}
	for _, original := range []*k2aLab{a, b} {
		wrong := *original
		wrong.token = "not-the-owned-label"
		if _, err := wrong.inspect(); err == nil {
			t.Fatal("container identity mismatch accepted")
		}
		wrong = *original
		wrong.imageID = "not-the-expected-image"
		if _, err := wrong.inspect(); err == nil {
			t.Fatal("container image mismatch accepted")
		}
	}
}

func TestPostgresLedgerK2aCurrency(t *testing.T) {
	catalog, err := reference.Load()
	if err != nil {
		t.Fatal(err)
	}
	d, err := catalog.LookupCurrency("USD")
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := catalog.LookupDefinition(k2aDefinition)
	if err != nil || pinned != d || d.Code() != "USD" || d.Exponent() != 2 || d.Version() != k2aDefinition || d.UnitID() != k2aUnit || catalog.ReleaseID() != k2aRelease {
		t.Fatal("canonical frozen USD mismatch")
	}
	admission, err := ledger.NewCurrencyAdmission([]ledger.CurrencyDefinition{{Code: string(d.Code()), Exponent: d.Exponent(), Version: d.Version(), UnitID: d.UnitID()}})
	if err != nil {
		t.Fatal(err)
	}
	if admitted, err := admission.LookupCurrency("USD"); err != nil || admitted.Version != k2aDefinition || admitted.UnitID != k2aUnit || admitted.Exponent != 2 {
		t.Fatal("Ledger admission mismatch")
	}
	if _, err := admission.LookupCurrency("JPY"); err == nil {
		t.Fatal("catalog membership became Ledger enablement")
	}
	var lines []string
	for _, code := range []reference.CurrencyCode{"IDR", "JPY", "KWD", "SGD", "USD"} {
		definition, err := catalog.LookupCurrency(code)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, fmt.Sprintf("C,%s,%d,%s,%s\n", definition.Code(), definition.Exponent(), definition.Version(), definition.UnitID()))
	}
	for _, code := range []reference.CountryCode{"ID", "JP", "KW", "SG", "US"} {
		country, err := catalog.LookupCountry(code)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, fmt.Sprintf("N,%s\n", country))
	}
	slices.Sort(lines)
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(lines, "")))); got != k2aDigest {
		t.Fatal("curated provenance digest mismatch")
	}

	l, admin := k2aProvision(t)
	k2aMigrate(t, l)
	loader, reader := k2aOpenFixture(t, l, admin)
	for i, invalid := range []any{"mamony-currency-v1:USD:1", "JPY", int16(3), "mamony-unit-v1:USD:2", "wrong-release", strings.Repeat("0", 64)} {
		row := k2aGoldenCurrency()
		row[i] = invalid
		k2aReject(t, l, loader, "23514", k2aCurrency, row...)
		row[i] = nil
		k2aReject(t, l, loader, "23502", k2aCurrency, row...)
	}
	k2aAssertRows(t, l, reader, nil)
	k2aSeed(t, l, loader)
	k2aReject(t, l, loader, "23505", k2aCurrency, k2aGoldenCurrency()...)
	k2aAssertRows(t, l, reader, k2aExpectedRows())
	k2aSeal(t, l, admin)
}

func TestPostgresLedgerK2aStorage(t *testing.T) {
	l, admin := k2aProvision(t)
	k2aMigrate(t, l)
	loader, reader := k2aOpenFixture(t, l, admin)
	k2aAssertRows(t, l, reader, nil)
	tx, err := loader.Begin(l.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(l.ctx, k2aCurrency, k2aGoldenCurrency()...); err != nil {
		_ = tx.Rollback(l.ctx)
		t.Fatal(err)
	}
	_, err = tx.Exec(l.ctx, k2aBook, "bad", "absent")
	_ = tx.Rollback(l.ctx)
	k2aExpectState(t, err, "23503")
	k2aAssertRows(t, l, reader, nil)
	for _, tc := range []struct {
		name string
		sql  string
		row  []any
	}{
		{"book", k2aBook, []any{"BOOK-001", k2aDefinition}},
		{"type", k2aType, []any{"COLLECTION_CASH", "ASSET", "D", int64(0)}},
	} {
		t.Run("required_"+tc.name, func(t *testing.T) {
			for i := range tc.row {
				row := slices.Clone(tc.row)
				row[i] = nil
				k2aReject(t, l, loader, "23502", tc.sql, row...)
			}
		})
	}
	k2aAssertRows(t, l, reader, nil)
	k2aSeed(t, l, loader)
	k2aReject(t, l, loader, "23505", k2aBook, "BOOK-002", k2aDefinition)
	k2aReject(t, l, loader, "23503", k2aBook, "BOOK-002", "absent")
	k2aReject(t, l, loader, "23514", k2aType, "COLLECTION_CASH", "ASSET", "C", int64(0))
	k2aReject(t, l, loader, "23514", k2aType, "INVALID", "ASSET", "D", int64(0))
	base := []any{"ACC-NEW", "BOOK-001", "CUSTOMER_AVAILABLE", "SYN-003", "ACTIVE", int64(0), int64(0)}
	for i := range base {
		row := slices.Clone(base)
		row[i] = nil
		k2aReject(t, l, loader, "23502", k2aAccount, row...)
	}
	for _, tc := range []struct {
		column int
		value  any
		state  string
	}{
		{0, "ACC-001", "23505"},
		{0, "", "23514"},
		{0, strings.Repeat("a", 129), "23514"},
		{0, strings.Repeat("é", 65), "23514"},
		{1, "absent", "23503"},
		{2, "absent", "23503"},
		{3, "", "23514"},
		{3, strings.Repeat("a", 129), "23514"},
		{4, "CLOSED", "23514"},
		{4, "SUSPENDED", "23514"},
		{5, int64(1), "23514"},
		{5, int64(-1), "23514"},
		{6, int64(1), "23514"},
		{6, int64(-1), "23514"},
	} {
		row := slices.Clone(base)
		row[tc.column] = tc.value
		k2aReject(t, l, loader, tc.state, k2aAccount, row...)
	}
	// Named CHECK constraints distinguish the intended invariant from an
	// unrelated constraint with the same SQLSTATE.
	for _, tc := range []struct {
		column int
		value  any
		name   string
	}{
		{0, "", "account_id_bytes"},
		{3, "", "account_owner_bytes"},
		{4, "CLOSED", "account_k2a_status"},
		{5, int64(1), "account_k2a_balance"},
		{6, int64(1), "account_k2a_sequence"},
	} {
		row := slices.Clone(base)
		row[tc.column] = tc.value
		_, err := loader.Exec(l.ctx, k2aAccount, row...)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != tc.name {
			t.Fatalf("expected named invariant %s, got %v", tc.name, err)
		}
	}
	// The server must reject malformed UTF-8 and NUL in text parameters.
	for _, invalid := range []string{"bad\x00id", string([]byte{0xff})} {
		row := slices.Clone(base)
		row[0] = invalid
		k2aReject(t, l, loader, "22021", k2aAccount, row...)
	}
	tx, err = loader.Begin(l.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(l.ctx) }()
	for _, id := range []string{"A", strings.Repeat("é", 64), "acc-001", "é", "e\u0301"} {
		row := slices.Clone(base)
		row[0] = id
		if _, err := tx.Exec(l.ctx, k2aAccount, row...); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Rollback(l.ctx); err != nil {
		t.Fatal(err)
	}
	k2aAssertRows(t, l, reader, k2aExpectedRows())
	k2aSeal(t, l, admin)
}

func k2aCheckPrivileges(t *testing.T, l *k2aLab, admin *pgx.Conn) {
	t.Helper()
	reader := l.connect(t, k2aReader)
	k2aAssertRows(t, l, reader, k2aExpectedRows())
	for _, sql := range []string{
		"INSERT INTO mamony_ledger.accounts VALUES ('x','BOOK-001','CUSTOMER_AVAILABLE','x','ACTIVE',0,0)",
		"UPDATE mamony_ledger.accounts SET balance_minor=0",
		"DELETE FROM mamony_ledger.accounts",
		"TRUNCATE mamony_ledger.accounts",
		"CREATE TABLE mamony_ledger.unexpected (id integer)",
		"SELECT * FROM mamony_ledger.migration_fingerprints",
		"SELECT * FROM mamony_ledger.schema_migrations",
		"SELECT * FROM k2a_other.canary",
		"SET ROLE " + k2aMigrator,
	} {
		k2aReject(t, l, reader, "42501", sql)
	}
	loader := l.connect(t, k2aLoader)
	k2aReject(t, l, loader, "42501", k2aAccount, "x", "BOOK-001", "CUSTOMER_AVAILABLE", "x", "ACTIVE", int64(0), int64(0))
	other := l.connect(t, k2aOther)
	var id int
	if err := other.QueryRow(l.ctx, "SELECT id FROM k2a_other.canary").Scan(&id); err != nil || id != 1 {
		t.Fatal("other-owner positive control failed")
	}
	k2aReject(t, l, other, "42501", "SELECT * FROM mamony_ledger.accounts")
	migrator := l.connect(t, k2aMigrator)
	k2aReject(t, l, migrator, "42501", "SELECT * FROM k2a_other.canary")
	for _, role := range []string{k2aMigrator, k2aReader, k2aLoader, k2aOther} {
		var safe bool
		if err := admin.QueryRow(l.ctx, "SELECT NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls OR rolinherit) FROM pg_roles WHERE rolname=$1", role).Scan(&safe); err != nil || !safe {
			t.Fatal("unexpected effective role attributes")
		}
		var memberships, defaults int
		if err := admin.QueryRow(l.ctx, "SELECT count(*) FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.member WHERE r.rolname=$1", role).Scan(&memberships); err != nil || memberships != 0 {
			t.Fatal("unexpected role membership")
		}
		if err := admin.QueryRow(l.ctx, "SELECT count(*) FROM pg_default_acl a JOIN pg_roles r ON r.oid=a.defaclrole WHERE r.rolname=$1", role).Scan(&defaults); err != nil || defaults != 0 {
			t.Fatal("unexpected default privileges")
		}
	}
	for _, role := range []string{k2aMigrator, k2aReader, k2aLoader, k2aOther} {
		for _, privilege := range []string{"USAGE", "CREATE"} {
			var allowed bool
			want := role == k2aMigrator || (role == k2aReader && privilege == "USAGE")
			if err := admin.QueryRow(l.ctx, "SELECT has_schema_privilege($1,'mamony_ledger',$2)", role, privilege).Scan(&allowed); err != nil || allowed != want {
				t.Fatalf("unexpected schema privilege: %s %s", role, privilege)
			}
		}
		for _, table := range []string{"currency_definitions", "books", "account_types", "accounts", "migration_fingerprints", "schema_migrations"} {
			for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER", "MAINTAIN", "SELECT WITH GRANT OPTION", "INSERT WITH GRANT OPTION"} {
				var allowed bool
				business := table != "migration_fingerprints" && table != "schema_migrations"
				want := role == k2aMigrator || (role == k2aReader && business && privilege == "SELECT")
				if err := admin.QueryRow(l.ctx, "SELECT has_table_privilege($1,$2,$3)", role, "mamony_ledger."+table, privilege).Scan(&allowed); err != nil || allowed != want {
					t.Fatalf("unexpected table privilege: %s %s %s", role, table, privilege)
				}
			}
		}
	}
	// Check the database boundary as well as object-level grants.
	for _, role := range []string{k2aMigrator, k2aReader, k2aLoader, k2aOther} {
		var connect, create, temporary bool
		if err := admin.QueryRow(l.ctx, "SELECT has_database_privilege($1,current_database(),'CONNECT'), has_database_privilege($1,current_database(),'CREATE'), has_database_privilege($1,current_database(),'TEMP')", role).Scan(&connect, &create, &temporary); err != nil || !connect || create || temporary {
			t.Fatalf("unexpected database privileges for %s", role)
		}
	}
	var owned int
	if err := admin.QueryRow(l.ctx, "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_roles r ON r.oid=c.relowner WHERE n.nspname='mamony_ledger' AND c.relkind='r' AND r.rolname=$1", k2aMigrator).Scan(&owned); err != nil || owned != 6 {
		t.Fatal("Ledger table ownership mismatch")
	}
	k2aAssertRows(t, l, reader, k2aExpectedRows())
}

func TestPostgresLedgerK2aPrivileges(t *testing.T) {
	l, admin := k2aProvision(t)
	k2aMigrate(t, l)
	loader, reader := k2aOpenFixture(t, l, admin)
	k2aReject(t, l, loader, "42501", "SELECT * FROM mamony_ledger.accounts")
	k2aReject(t, l, loader, "42501", "UPDATE mamony_ledger.accounts SET balance_minor=0")
	k2aReject(t, l, loader, "42501", "DELETE FROM mamony_ledger.accounts")
	k2aReject(t, l, loader, "42501", "CREATE TABLE mamony_ledger.unexpected(id integer)")
	k2aReject(t, l, loader, "42501", "SELECT * FROM mamony_ledger.migration_fingerprints")
	k2aReject(t, l, loader, "42501", "SELECT * FROM k2a_other.canary")
	k2aSeed(t, l, loader)
	k2aAssertRows(t, l, reader, k2aExpectedRows())
	k2aSeal(t, l, admin)
	k2aCheckPrivileges(t, l, admin)
}

func k2aHistory(t *testing.T, l *k2aLab) {
	t.Helper()
	migrator := l.connect(t, k2aMigrator)
	files, err := migration.LoadLedger(k2aDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := migration.VerifyLedger(l.ctx, migrator, files); err != nil {
		t.Fatal(err)
	}
	var version int
	var dirty bool
	if err := migrator.QueryRow(l.ctx, "SELECT version,dirty FROM mamony_ledger.schema_migrations").Scan(&version, &dirty); err != nil || version != 1 || dirty {
		t.Fatal("unexpected migration metadata")
	}
}

func TestPostgresLedgerK2aMigration(t *testing.T) {
	l, admin := k2aProvision(t)
	if err := migration.RunLedger(l.ctx, l.dsn(k2aAdmin), k2aDir()); err == nil {
		t.Fatal("wrong migration role accepted")
	}
	if err := migration.Run(l.ctx, l.dsn(k2aMigrator), k2aDir()); err == nil {
		t.Fatal("wrong migration target accepted")
	}
	var exists bool
	if err := admin.QueryRow(l.ctx, "SELECT to_regclass('mamony_ledger.schema_migrations') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatal("wrong migration attempt changed schema")
	}
	k2aMigrate(t, l)
	k2aHistory(t, l)
	k2aMigrate(t, l)
	k2aHistory(t, l)
	migrator := l.connect(t, k2aMigrator)
	l.exec(t, migrator, "SELECT pg_advisory_lock(731198465908225)")
	if err := migration.RunLedger(l.ctx, l.dsn(k2aMigrator), k2aDir()); err == nil {
		t.Fatal("concurrent Ledger migration accepted")
	}
	l.exec(t, migrator, "SELECT pg_advisory_unlock(731198465908225)")
	ctx, cancel := context.WithCancel(l.ctx)
	cancel()
	if err := migration.RunLedger(ctx, l.dsn(k2aMigrator), k2aDir()); err == nil {
		t.Fatal("canceled migration accepted")
	}
	k2aHistory(t, l)
	// Deliberately inconsistent metadata is confined to rollback-only
	// transactions in this disposable lab. No recovery repair path is added.
	files, err := migration.LoadLedger(k2aDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		sql  string
	}{
		{"missing_fingerprint", "DELETE FROM mamony_ledger.migration_fingerprints WHERE version=1"},
		{"extra_fingerprint", "INSERT INTO mamony_ledger.migration_fingerprints(version,sha256) VALUES (2,repeat('0',64))"},
		{"unknown_version", "UPDATE mamony_ledger.schema_migrations SET version=2"},
		{"changed_fingerprint", "UPDATE mamony_ledger.migration_fingerprints SET sha256=repeat('0',64) WHERE version=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l.exec(t, migrator, "BEGIN")
			defer func() {
				if _, err := migrator.Exec(l.ctx, "ROLLBACK"); err != nil {
					t.Error("negative metadata fixture rollback failed")
				}
			}()
			l.exec(t, migrator, tc.sql)
			if err := migration.VerifyLedger(l.ctx, migrator, files); err == nil {
				t.Fatal("inconsistent migration history accepted")
			}
		})
	}
	k2aHistory(t, l)
	dir := t.TempDir()
	name := "000001_ledger-storage.up.sql"
	original, err := os.ReadFile(filepath.Join(k2aDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	sign := func(version int, body string) []byte {
		return []byte(body + fmt.Sprintf("\n-- mamony:integrity:v1\nINSERT INTO mamony_ledger.migration_fingerprints (version, sha256) VALUES (%d, '%x');\n", version, sha256.Sum256([]byte(body))))
	}
	body, _, ok := strings.Cut(string(original), "\n-- mamony:integrity:v1\n")
	if !ok {
		t.Fatal("baseline fingerprint missing")
	}
	write(name, sign(1, body+"-- altered historical fixture\n"))
	if err := migration.RunLedger(l.ctx, l.dsn(k2aMigrator), dir); err == nil {
		t.Fatal("resigned historical migration accepted")
	}
	write(name, original)
	// A synthetic failure-only migration in a temporary directory is not a
	// committed version-to-version Ledger upgrade.
	write("000002_failure-fixture.up.sql", sign(2, "CREATE TABLE mamony_ledger.k2a_failure_fixture(id integer);\nSELECT 1/0;\n"))
	if err := migration.RunLedger(l.ctx, l.dsn(k2aMigrator), dir); err == nil {
		t.Fatal("failing migration accepted")
	}
	var dirty bool
	var version int
	if err := migrator.QueryRow(l.ctx, "SELECT version,dirty FROM mamony_ledger.schema_migrations").Scan(&version, &dirty); err != nil || version != 2 || !dirty {
		t.Fatal("failed migration did not leave expected dirty state")
	}
	if err := migrator.QueryRow(l.ctx, "SELECT to_regclass('mamony_ledger.k2a_failure_fixture') IS NOT NULL OR EXISTS(SELECT 1 FROM mamony_ledger.migration_fingerprints WHERE version=2)").Scan(&exists); err != nil || exists {
		t.Fatal("failed batch left partial DDL or fingerprint")
	}
	if err := migration.RunLedger(l.ctx, l.dsn(k2aMigrator), dir); err == nil {
		t.Fatal("dirty migration retried automatically")
	}

	// Model the committed-body/pre-clean window in a separate disposable
	// lab. This is a dirty-metadata fixture, not an actual process kill.
	committed, _ := k2aProvision(t)
	k2aMigrate(t, committed)
	owner := committed.connect(t, k2aMigrator)
	committed.exec(t, owner, "UPDATE mamony_ledger.schema_migrations SET dirty=true WHERE version=1")
	if err := migration.RunLedger(committed.ctx, committed.dsn(k2aMigrator), k2aDir()); err == nil {
		t.Fatal("committed-body dirty fixture was automatically retried")
	}
	var committedBody bool
	if err := owner.QueryRow(committed.ctx, "SELECT to_regclass('mamony_ledger.accounts') IS NOT NULL AND EXISTS(SELECT 1 FROM mamony_ledger.migration_fingerprints WHERE version=1 AND sha256=$1) AND EXISTS(SELECT 1 FROM mamony_ledger.schema_migrations WHERE version=1 AND dirty)", files[0].Digest).Scan(&committedBody); err != nil || !committedBody {
		t.Fatal("committed-body dirty evidence was changed")
	}
}

func k2aNormalizedSchema(data []byte) []string {
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "\\restrict ") && !strings.HasPrefix(line, "\\unrestrict ") {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestPostgresLedgerK2aRestore(t *testing.T) {
	source, admin := k2aProvision(t)
	k2aMigrate(t, source)
	loader, reader := k2aOpenFixture(t, source, admin)
	k2aSeed(t, source, loader)
	k2aSeal(t, source, admin)
	k2aAssertRows(t, source, reader, k2aExpectedRows())
	k2aCheckPrivileges(t, source, admin)
	k2aHistory(t, source)
	backupStart := time.Now().UTC()
	dump := source.dump(t, false)
	backupEnd := time.Now().UTC()
	t.Logf("K2A backup sealed fixture start=%s end=%s elapsed=%s sha256=%x", backupStart.Format(time.RFC3339Nano), backupEnd.Format(time.RFC3339Nano), backupEnd.Sub(backupStart), sha256.Sum256(dump))
	expectedSchema := k2aNormalizedSchema(source.dump(t, true))
	target := k2aStart(t)
	if source.id == target.id || source.port == target.port || source.database == target.database {
		t.Fatal("restore destination is not independent")
	}
	if err := target.restore(t, []byte("not a PostgreSQL custom archive")); err == nil {
		t.Fatal("invalid recovery archive accepted")
	}
	// Missing destination role definitions must cause a transactional
	// restore failure, not silently drop ownership or ACLs.
	if err := target.restore(t, dump); err == nil {
		t.Fatal("restore without required roles accepted")
	}
	initialAdmin := target.connect(t, k2aAdmin)
	var exists bool
	if err := initialAdmin.QueryRow(target.ctx, "SELECT to_regnamespace('mamony_ledger') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatal("failed restore left partial Ledger schema")
	}
	targetAdmin := target.roles(t)
	restoreStart := time.Now().UTC()
	if err := target.restore(t, dump); err != nil {
		t.Fatal("K2a independent restore failed")
	}
	if got := k2aNormalizedSchema(target.dump(t, true)); !reflect.DeepEqual(got, expectedSchema) {
		t.Fatal("restored schema/ownership/ACL manifest differs")
	}
	k2aCheckPrivileges(t, target, targetAdmin)
	k2aHistory(t, target)
	k2aMigrate(t, target)
	k2aHistory(t, target)
	restoreEnd := time.Now().UTC()
	t.Logf("K2A restore verified start=%s end=%s elapsed=%s archive_sha256=%x", restoreStart.Format(time.RFC3339Nano), restoreEnd.Format(time.RFC3339Nano), restoreEnd.Sub(restoreStart), sha256.Sum256(dump))
}
