package migration

import (
	"crypto/sha256"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Every supported migration has a SQL trailer recording its canonical body
// digest. The trailer is part of the same PostgreSQL statement batch as DDL.
// golang-migrate owns execution and schema_migrations state.
const marker = "\n-- mamony:integrity:v1\n"

var (
	fileNamePattern     = regexp.MustCompile(`^([0-9]{6})_[a-z][a-z0-9-]*\.up\.sql$`)
	explicitTransaction = regexp.MustCompile(`(?im)^\s*(BEGIN|COMMIT|ROLLBACK|SAVEPOINT|RELEASE)\s*(;|$)`)
)

type File struct {
	Version int64
	Name    string
	Digest  string
}

// Load rejects malformed or altered migrations before any database write.
// A changed old file and recalculated trailer will still be rejected against
// the fingerprint previously committed inside PostgreSQL.
func Load(dir string) ([]File, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open migration directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}
	files := make([]File, 0, len(entries))
	seen := make(map[int64]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, fmt.Errorf("unexpected migration subdirectory %q", entry.Name())
		}
		name := entry.Name()
		m := fileNamePattern.FindStringSubmatch(name)
		if m == nil {
			return nil, fmt.Errorf("unexpected file %q; only named .up.sql migrations are supported", name)
		}
		version, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || version <= 0 || seen[version] {
			return nil, fmt.Errorf("invalid or duplicate migration version in %q", name)
		}
		seen[version] = true
		b, err := root.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", name, err)
		}
		idx := strings.Index(string(b), marker)
		if idx < 1 || strings.Count(string(b), marker) != 1 {
			return nil, fmt.Errorf("%q: missing or duplicate integrity trailer", name)
		}
		body := string(b[:idx])
		if explicitTransaction.MatchString(body) {
			return nil, fmt.Errorf("%q: explicit transaction control is forbidden", name)
		}
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
		trailer := fmt.Sprintf("%sINSERT INTO mamony_platform.migration_fingerprints (version, sha256) VALUES (%d, '%s');\n", marker, version, digest)
		if string(b[idx:]) != trailer {
			return nil, fmt.Errorf("%q: integrity trailer mismatch; historical SQL cannot be silently rewritten", name)
		}
		files = append(files, File{Version: version, Name: name, Digest: digest})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no SQL up migrations found")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Version < files[j].Version })
	return files, nil
}
