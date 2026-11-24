//go:build integration

package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Lab-only binding and decision oracle. No application imports this code,
// and it cannot enable a login or release a recovery target.
type restoreReviewBinding struct {
	AttemptID    string
	ContainerID  string
	ClusterID    string
	Database     string
	DatabaseOID  string
	BackupSHA    string
	SchemaSHA    string
	RoleNamesSHA string
	Checkpoint   string
}

func (b restoreReviewBinding) complete() bool {
	return b.AttemptID != "" && b.ContainerID != "" && b.ClusterID != "" &&
		b.Database != "" && b.DatabaseOID != "" && b.BackupSHA != "" &&
		b.SchemaSHA != "" && b.RoleNamesSHA != "" && b.Checkpoint != ""
}

// restoreReviewDecision is a test oracle, not an authentication or storage
// integrity mechanism. Even matching metadata cannot establish history
// completeness or authorize release. The only positive result is REVIEW_ONLY.
func restoreReviewDecision(recordPath string, expected, observed restoreReviewBinding, held, evidenceComplete bool) string {
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		return "HOLD_RECORD_UNAVAILABLE"
	}
	var record restoreReviewBinding
	if err := json.Unmarshal(raw, &record); err != nil || !record.complete() {
		return "HOLD_RECORD_INVALID"
	}
	if record != expected || record != observed {
		return "HOLD_BINDING_MISMATCH"
	}
	if !held {
		return "HOLD_BARRIER_UNVERIFIED"
	}
	if !evidenceComplete {
		return "HOLD_EVIDENCE_INCOMPLETE"
	}
	return "REVIEW_ONLY"
}

func restoreReviewDigest(value []byte) string {
	d := sha256.Sum256(value)
	return hex.EncodeToString(d[:])
}

// The schema and role name summaries are deliberately limited. They do not
// attest actual DDL, effective grants, role membership or full history.
func restoreReviewCatalog(t *testing.T, ctx context.Context, conn *pgx.Conn) (string, string) {
	t.Helper()
	var schema, roles string
	if err := conn.QueryRow(ctx, `SELECT
		(SELECT coalesce(string_agg(version::text||':'||sha256, ',' ORDER BY version),'')
		 FROM mamony_identity.migration_fingerprints)
		|| '|' ||
		(SELECT coalesce(string_agg(version::text||':'||dirty::text, ',' ORDER BY version),'')
		 FROM mamony_identity.schema_migrations)`).Scan(&schema); err != nil || schema == "" {
		t.Fatal("cannot read synthetic migration evidence")
	}
	if err := conn.QueryRow(ctx, `SELECT coalesce(string_agg(rolname, ',' ORDER BY rolname),'')
		FROM pg_catalog.pg_roles WHERE rolname LIKE 'mamony_%' AND rolname<>'mamony_test_admin'`).Scan(&roles); err != nil || roles == "" {
		t.Fatal("cannot read synthetic role name evidence")
	}
	return restoreReviewDigest([]byte(schema)), restoreReviewDigest([]byte(roles))
}

// The process-local file survives database restore only in the lab. Its
// existence is not proof of independent custody, durability or completeness.
func checkHeldRestoreReviewRecord(t *testing.T, ctx context.Context, source, target *pgx.Conn,
	containerID string, dump []byte, checkpoint string, held, checkpointPresent bool) {
	t.Helper()
	sourceSchema, sourceRoles := restoreReviewCatalog(t, ctx, source)
	targetSchema, targetRoles := restoreReviewCatalog(t, ctx, target)
	var cluster, database, oid string
	if err := target.QueryRow(ctx, `SELECT system_identifier::text,current_database(),
		(SELECT oid::text FROM pg_database WHERE datname=current_database())
		FROM pg_control_system()`).Scan(&cluster, &database, &oid); err != nil {
		t.Fatal("cannot identify restored cluster/database")
	}
	var sourceCluster string
	if err := source.QueryRow(ctx, "SELECT system_identifier::text FROM pg_control_system()").Scan(&sourceCluster); err != nil || sourceCluster == cluster {
		t.Fatal("restore cluster not distinct")
	}
	expected := restoreReviewBinding{
		AttemptID: randomSyntheticID(t, "rec_"), ContainerID: containerID,
		ClusterID: cluster, Database: database, DatabaseOID: oid,
		BackupSHA: restoreReviewDigest(dump), SchemaSHA: sourceSchema,
		RoleNamesSHA: sourceRoles, Checkpoint: checkpoint,
	}
	observed := expected
	observed.SchemaSHA, observed.RoleNamesSHA = targetSchema, targetRoles
	if expected != observed {
		t.Fatal("source and restored metadata summaries differ")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "restore-review.json")
	assertDecision := func(recordPath string, candidate restoreReviewBinding, barrier, complete bool, want string) {
		t.Helper()
		if got := restoreReviewDecision(recordPath, expected, candidate, barrier, complete); got != want {
			t.Fatalf("review decision: got %s want %s", got, want)
		}
	}
	assertDecision(path, observed, held, false, "HOLD_RECORD_UNAVAILABLE")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal("cannot create corrupt synthetic record")
	}
	assertDecision(path, observed, held, false, "HOLD_RECORD_INVALID")
	raw, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal("cannot write synthetic review record")
	}
	// The actual restored checkpoint is absent. Never set completeness
	// true for this target, even if all identity metadata matches.
	if checkpointPresent {
		t.Fatal("stale checkpoint fixture unexpectedly complete")
	}
	assertDecision(path, observed, held, false, "HOLD_EVIDENCE_INCOMPLETE")
	for _, mutate := range []func(*restoreReviewBinding){
		func(b *restoreReviewBinding) { b.AttemptID = "different-attempt" },
		func(b *restoreReviewBinding) { b.ContainerID = "different-container" },
		func(b *restoreReviewBinding) { b.ClusterID = "different-cluster" },
		func(b *restoreReviewBinding) { b.Database = "different-database" },
		func(b *restoreReviewBinding) { b.DatabaseOID = "different-oid" },
		func(b *restoreReviewBinding) { b.BackupSHA = "different-backup" },
		func(b *restoreReviewBinding) { b.SchemaSHA = "different-schema" },
		func(b *restoreReviewBinding) { b.RoleNamesSHA = "different-role-names" },
		func(b *restoreReviewBinding) { b.Checkpoint = "different-checkpoint" },
	} {
		other := observed
		mutate(&other)
		assertDecision(path, other, held, false, "HOLD_BINDING_MISMATCH")
	}
	assertDecision(path, observed, false, false, "HOLD_BARRIER_UNVERIFIED")
	// Exercise the positive branch only as a synthetic oracle input; this
	// is NOT an observation of complete evidence on the real restore.
	assertDecision(path, observed, true, true, "REVIEW_ONLY")
	t.Log("IRF-REVIEW RECORD MECHANISM PASS: actual stale target held for missing checkpoint; missing corrupt and mismatched bindings rejected; synthetic review only")
}
