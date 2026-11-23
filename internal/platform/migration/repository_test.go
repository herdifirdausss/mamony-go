package migration

import (
	"path/filepath"
	"testing"
)

// A repo-level canary: ordinary go test must fail if an applied SQL fixture
// was edited without a matching canonical fingerprint.
func TestRepositoryMigrationFiles(t *testing.T) {
	t.Parallel()
	files, err := Load(filepath.Join("..", "..", "..", "db", "migrations", "platform"))
	if err != nil {
		t.Fatalf("committed platform SQL must have valid integrity metadata: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no committed SQL migrations")
	}
}

func TestRepositoryIdentityMigrationFiles(t *testing.T) {
	t.Parallel()
	files, err := LoadIdentity(filepath.Join("..", "..", "..", "db", "migrations", "identity"))
	if err != nil || len(files) != 33 {
		t.Fatalf("committed identity SQL must pass fingerprint verification: files=%v err=%v", files, err)
	}
}
