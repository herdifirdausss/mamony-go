//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const rh03ExecArchive = "/tmp/mamony-rh03-exec-archive"

type rh03NativeEvidence struct {
	dir, socket, admin, db, lineage, timeline, boundary string
}

func rh03NativeCommand(t *testing.T, ctx context.Context, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("isolated native fixture command failed: %s", name)
	}
}

func newRH03NativeEvidence(t *testing.T, ctx context.Context, source *pgx.Conn, admin, db string) *rh03NativeEvidence {
	t.Helper()
	root := t.TempDir()
	n := &rh03NativeEvidence{
		dir: filepath.Join(root, "base"), socket: filepath.Join(root, "socket"),
		admin: admin, db: db,
	}
	var mode string
	if err := source.QueryRow(ctx, "SHOW archive_mode").Scan(&mode); err != nil || mode != "on" {
		t.Fatal("isolated source archive mode missing")
	}
	if err := source.QueryRow(ctx, "SELECT system_identifier::text FROM pg_control_system()").Scan(&n.lineage); err != nil {
		t.Fatal("source lineage observation failed")
	}
	if err := source.QueryRow(ctx, "SELECT timeline_id::text FROM pg_control_checkpoint()").Scan(&n.timeline); err != nil ||
		!regexp.MustCompile("^[0-9]+$").MatchString(n.timeline) {
		t.Fatal("source timeline observation failed")
	}
	rh03NativeCommand(t, ctx, "pg_basebackup", "--host=/var/run/postgresql", "--username="+admin,
		"--no-password", "--pgdata="+n.dir, "--wal-method=stream", "--checkpoint=fast")
	rh03NativeCommand(t, ctx, "pg_verifybackup", n.dir)
	if _, err := source.Exec(ctx, "SELECT pg_switch_wal()"); err != nil {
		t.Fatal("post-base WAL switch failed")
	}
	return n
}

func (n *rh03NativeEvidence) recover(t *testing.T, ctx context.Context, source *pgx.Conn) *pgx.Conn {
	t.Helper()
	if err := source.QueryRow(ctx, "SELECT pg_create_restore_point('mamony_rh03_exec_boundary')::text").Scan(&n.boundary); err != nil {
		t.Fatal("native application boundary missing")
	}
	var segment string
	if err := source.QueryRow(ctx, "SELECT pg_walfile_name($1::pg_lsn)", n.boundary).Scan(&segment); err != nil ||
		!regexp.MustCompile("^[0-9A-F]{24}$").MatchString(segment) {
		t.Fatal("native application segment missing")
	}
	if _, err := os.Stat(filepath.Join(n.dir, "pg_wal", segment)); !os.IsNotExist(err) {
		t.Fatal("native boundary WAL unexpectedly already in base")
	}
	if _, err := source.Exec(ctx, "SELECT pg_switch_wal()"); err != nil {
		t.Fatal("native boundary archive switch failed")
	}
	archived := false
	for i := 0; i < 45; i++ {
		if info, err := os.Stat(filepath.Join(rh03ExecArchive, segment)); err == nil && info.Size() > 0 {
			archived = true
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("native boundary archive wait expired")
		case <-time.After(time.Second):
		}
	}
	if !archived {
		t.Fatal("required post-base WAL not archived")
	}
	if err := os.Mkdir(n.socket, 0700); err != nil {
		t.Fatal("private recovery socket creation failed")
	}
	// This independent physical observation cluster never accepts TCP
	// connections and remains paused. No role/ACL mutation or promotion.
	conf := fmt.Sprintf("\nlisten_addresses = ''\nport = 55432\nunix_socket_directories = '%s'\nunix_socket_permissions = 0700\narchive_mode = off\nrestore_command = 'cp %s/%%f %%p'\nrecovery_target_name = 'mamony_rh03_exec_boundary'\nrecovery_target_timeline = '%s'\nrecovery_target_action = 'pause'\nhot_standby = on\n", n.socket, rh03ExecArchive, n.timeline)
	f, err := os.OpenFile(filepath.Join(n.dir, "postgresql.conf"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("fixture recovery config unavailable")
	}
	_, writeErr := f.WriteString(conf)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("fixture recovery config write failed")
	}
	if err := os.WriteFile(filepath.Join(n.dir, "recovery.signal"), nil, 0600); err != nil {
		t.Fatal("fixture recovery signal creation failed")
	}
	logPath := filepath.Join(filepath.Dir(n.dir), "recovery.log")
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(stopCtx, "pg_ctl", "-D", n.dir, "-m", "fast", "-w", "-t", "10", "stop")
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}
		_ = cmd.Run() // outer disposable container cleanup remains mandatory
	})
	rh03NativeCommand(t, ctx, "pg_ctl", "-D", n.dir, "-l", logPath, "-w", "-t", "45", "start")
	cfg, err := pgx.ParseConfig("postgres:///postgres?sslmode=disable")
	if err != nil {
		t.Fatal("private observation config invalid")
	}
	cfg.Host, cfg.Port, cfg.User, cfg.Database = n.socket, 55432, n.admin, n.db
	cfg.Password = ""
	cfg.Fallbacks = nil
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal("private native observation connection failed")
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	paused := false
	for i := 0; i < 30; i++ {
		if err := conn.QueryRow(ctx, "SELECT pg_is_in_recovery() AND pg_get_wal_replay_pause_state()='paused'").Scan(&paused); err == nil && paused {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("native pause wait expired")
		case <-time.After(time.Second):
		}
	}
	n.assertHeld(t, ctx, conn)
	logBytes, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(logBytes), "recovery stopping at restore point") {
		t.Fatal("named native target not established")
	}
	return conn
}

func (n *rh03NativeEvidence) assertHeld(t *testing.T, ctx context.Context, c *pgx.Conn) {
	t.Helper()
	var held bool
	if err := c.QueryRow(ctx, "SELECT pg_is_in_recovery() AND pg_get_wal_replay_pause_state()='paused' AND current_setting('transaction_read_only')='on' AND current_setting('listen_addresses')='' AND current_database()=$1 AND current_user=$2 AND pg_last_wal_replay_lsn()>=$3::pg_lsn AND (SELECT system_identifier::text FROM pg_control_system())=$4", n.db, n.admin, n.boundary, n.lineage).Scan(&held); err != nil || !held {
		t.Fatal("native evidence target not at held boundary")
	}
}

func (n *rh03NativeEvidence) dump(t *testing.T, ctx context.Context) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "native-derived.dump")
	rh03NativeCommand(t, ctx, "pg_dump", "--host="+n.socket, "--port=55432", "--username="+n.admin,
		"--no-password", "--format=custom", "--file="+path, "--dbname="+n.db)
	return path
}
