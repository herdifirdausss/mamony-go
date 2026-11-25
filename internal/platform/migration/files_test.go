package migration

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
)

func fixtureRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	return root
}

func writeTestMigration(t *testing.T, dir string, version int, body string) string {
	t.Helper()
	name := fmt.Sprintf("%06d_testing.up.sql", version)
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
	sql := body + marker + fmt.Sprintf(
		"INSERT INTO mamony_platform.migration_fingerprints (version, sha256) VALUES (%d, '%s');\n", version, digest)
	if err := fixtureRoot(t, dir).WriteFile(name, []byte(sql), 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestMigrationFileIntegrity(t *testing.T) {
	t.Parallel()
	t.Run("valid migration", func(t *testing.T) {
		dir := t.TempDir()
		writeTestMigration(t, dir, 1, "CREATE TABLE example(id integer);\n")
		files, err := Load(dir)
		if err != nil || len(files) != 1 || files[0].Version != 1 || len(files[0].Digest) != 64 {
			t.Fatalf("valid SQL rejected: files=%v err=%v", files, err)
		}
	})
	t.Run("mutated content", func(t *testing.T) {
		dir := t.TempDir()
		name := writeTestMigration(t, dir, 1, "CREATE TABLE example(id integer);\n")
		root := fixtureRoot(t, dir)
		data, err := root.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.Replace(string(data), "example(id", "attacker(id", 1))
		if err := root.WriteFile(name, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(dir); err == nil {
			t.Fatal("edited migration accepted")
		}
	})
	t.Run("missing trailer", func(t *testing.T) {
		dir := t.TempDir()
		if err := fixtureRoot(t, dir).WriteFile("000001_init.up.sql", []byte("SELECT 1;\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(dir); err == nil {
			t.Fatal("unsigned migration accepted")
		}
	})
	t.Run("duplicate version", func(t *testing.T) {
		dir := t.TempDir()
		writeTestMigration(t, dir, 1, "SELECT 1;\n")
		a, err := fixtureRoot(t, dir).ReadFile("000001_testing.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if err := fixtureRoot(t, dir).WriteFile("000001_second.up.sql", a, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(dir); err == nil {
			t.Fatal("duplicate version accepted")
		}
	})
	t.Run("transaction split", func(t *testing.T) {
		dir := t.TempDir()
		writeTestMigration(t, dir, 1, "CREATE TABLE example(id integer);\nCOMMIT;\n")
		if _, err := Load(dir); err == nil {
			t.Fatal("explicit COMMIT accepted")
		}
	})
	t.Run("unrecognized files", func(t *testing.T) {
		dir := t.TempDir()
		writeTestMigration(t, dir, 1, "SELECT 1;\n")
		if err := fixtureRoot(t, dir).WriteFile("000001_testing.down.sql", []byte("DROP TABLE"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(dir); err == nil {
			t.Fatal("unapproved down SQL accepted")
		}
	})
}

func TestMigrationFileComparison(t *testing.T) {
	t.Parallel()
	a := []File{{Version: 1, Name: "one", Digest: "abc"}}
	b := []File{{Version: 1, Name: "one", Digest: "abc"}}
	if !sameFiles(a, b) {
		t.Fatal("matching manifests differed")
	}
	b[0].Digest = "mutated"
	if sameFiles(a, b) {
		t.Fatal("changed manifest was accepted")
	}
}

func TestLedgerMigrationManifestTarget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := "SELECT 1;\n"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
	sql := body + marker + fmt.Sprintf("INSERT INTO mamony_ledger.migration_fingerprints (version, sha256) VALUES (1, '%s');\n", digest)
	if err := fixtureRoot(t, dir).WriteFile("000001_testing.up.sql", []byte(sql), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := LoadLedger(dir)
	if err != nil || len(files) != 1 || files[0].Digest != digest {
		t.Fatalf("Ledger manifest rejected: %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("Platform accepted Ledger manifest")
	}
	if _, err := LoadIdentity(dir); err == nil {
		t.Fatal("Identity accepted Ledger manifest")
	}
}
