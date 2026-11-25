//go:build integration

package postgrestest

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	k2aImage    = "postgres:18.7"
	k2aAdmin    = "mamony_k2a_admin"
	k2aMigrator = "mamony_ledger_migrator"
	k2aLoader   = "mamony_k2a_loader"
	k2aReader   = "mamony_k2a_reader"
	k2aOther    = "mamony_k2a_other"
)

type k2aBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string
}

type k2aInspection struct {
	ID     string `json:"Id"`
	Image  string
	Config struct {
		Image  string
		Labels map[string]string
	}
	State struct {
		Running bool
	}
	HostConfig struct {
		NetworkMode  string
		PidMode      string
		IpcMode      string
		Privileged   bool
		Binds        []string
		PortBindings map[string][]k2aBinding
	}
	Mounts []struct {
		Type        string
		Destination string
	}
	NetworkSettings struct {
		Ports map[string][]k2aBinding
	}
}

type k2aLab struct {
	id, token, imageID, database, port string
	passwords                          map[string]string
	ctx                                context.Context
}

// Docker is deliberately local and fixed. No caller DSN, Docker context,
// host mount, host network, external database or production secret is used.
func k2aDocker(input []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", append([]string{"--host", "unix:///var/run/docker.sock"}, args...)...)
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		if !strings.HasPrefix(key, "DOCKER_") && !strings.HasPrefix(key, "PG") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Stdin = bytes.NewReader(input)
	// Do not print command arguments or raw inspect output containing fixture
	// passwords, even though all credentials are disposable.
	return cmd.Output()
}

func k2aRandom(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal("cannot generate lab identity")
	}
	return hex.EncodeToString(b[:])
}

func (l *k2aLab) inspect() (k2aInspection, error) {
	var info []k2aInspection
	raw, err := k2aDocker(nil, "inspect", l.id)
	if err != nil || json.Unmarshal(raw, &info) != nil || len(info) != 1 {
		return k2aInspection{}, fmt.Errorf("cannot inspect owned lab container")
	}
	i := info[0]
	if i.ID != l.id || i.Config.Labels["mamony.synthetic-k2a"] != l.token || i.Config.Image != k2aImage ||
		i.Image != l.imageID || i.HostConfig.NetworkMode != "bridge" || i.HostConfig.Privileged || len(i.HostConfig.Binds) != 0 ||
		i.HostConfig.PidMode == "host" || i.HostConfig.IpcMode == "host" {
		return k2aInspection{}, fmt.Errorf("lab container identity or boundary mismatch")
	}
	for _, mount := range i.Mounts {
		if mount.Type != "volume" || mount.Destination != "/var/lib/postgresql" {
			return k2aInspection{}, fmt.Errorf("unexpected lab mount")
		}
	}
	b := i.HostConfig.PortBindings["5432/tcp"]
	if len(i.HostConfig.PortBindings) != 1 || len(b) != 1 || b[0].HostIP != "127.0.0.1" {
		return k2aInspection{}, fmt.Errorf("unexpected lab port binding")
	}
	return i, nil
}

func k2aStart(t *testing.T) *k2aLab {
	t.Helper()
	token := k2aRandom(t)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	t.Cleanup(cancel)
	l := &k2aLab{
		token:     token,
		database:  "mamony_test_k2a_" + token,
		passwords: make(map[string]string),
		ctx:       ctx,
	}
	for _, role := range []string{k2aAdmin, k2aMigrator, k2aLoader, k2aReader, k2aOther} {
		l.passwords[role] = k2aRandom(t)
	}
	image, err := k2aDocker(nil, "image", "inspect", "--format", "{{.Id}}", k2aImage)
	if err != nil || !strings.HasPrefix(string(image), "sha256:") {
		t.Fatal("K2a requires the existing local PostgreSQL 18.7 fixture image")
	}
	l.imageID = strings.TrimSpace(string(image))
	raw, err := k2aDocker(nil, "create", "--pull=never", "--network", "bridge",
		"--label", "mamony.synthetic-k2a="+token, "--publish", "127.0.0.1::5432",
		"-e", "POSTGRES_DB="+l.database, "-e", "POSTGRES_USER="+k2aAdmin,
		"-e", "POSTGRES_PASSWORD="+l.passwords[k2aAdmin], k2aImage)
	if err != nil {
		t.Fatal("cannot create K2a disposable container")
	}
	l.id = strings.TrimSpace(string(raw))
	if b, err := hex.DecodeString(l.id); err != nil || len(b) != 32 {
		t.Fatal("unexpected Docker container identity")
	}
	t.Cleanup(func() {
		if _, err := l.inspect(); err != nil {
			t.Error("owned lab identity uncertain; no cleanup attempted")
			return
		}
		if _, err := k2aDocker(nil, "rm", "-f", "-v", l.id); err != nil {
			t.Error("owned lab cleanup failed")
		}
	})
	i, err := l.inspect()
	if err != nil || i.State.Running {
		t.Fatal("K2a pre-start containment refused")
	}
	if _, err := k2aDocker(nil, "start", l.id); err != nil {
		t.Fatal("cannot start verified disposable lab")
	}
	i, err = l.inspect()
	if err != nil || !i.State.Running {
		t.Fatal("K2a running containment refused")
	}
	b := i.NetworkSettings.Ports["5432/tcp"]
	if len(i.NetworkSettings.Ports) != 1 || len(b) != 1 || b[0].HostIP != "127.0.0.1" {
		t.Fatal("unexpected published lab port")
	}
	port, err := strconv.Atoi(b[0].HostPort)
	if err != nil || port <= 0 || port > 65535 || port == 5432 {
		t.Fatal("invalid isolated lab port")
	}
	l.port = b[0].HostPort
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := pgx.Connect(l.ctx, l.dsn(k2aAdmin))
		if err == nil {
			var database, role, encoding string
			var version int
			err = conn.QueryRow(l.ctx, "SELECT current_database(), current_user, current_setting('server_encoding'), current_setting('server_version_num')::int").Scan(&database, &role, &encoding, &version)
			_ = conn.Close(l.ctx)
			if err != nil || database != l.database || role != k2aAdmin || encoding != "UTF8" || version != 180007 {
				t.Fatal("lab database identity/version/encoding mismatch")
			}
			return l
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("verified disposable lab did not become ready")
	return nil
}

func (l *k2aLab) dsn(role string) string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(role, l.passwords[role]),
		Host:     net.JoinHostPort("127.0.0.1", l.port),
		Path:     "/" + l.database,
		RawQuery: "sslmode=disable&connect_timeout=2",
	}
	return u.String()
}

func (l *k2aLab) verifyRunning(t *testing.T) {
	t.Helper()
	i, err := l.inspect()
	if err != nil || !i.State.Running {
		t.Fatal("K2a running container identity no longer established")
	}
	b := i.NetworkSettings.Ports["5432/tcp"]
	if len(i.NetworkSettings.Ports) != 1 || len(b) != 1 || b[0].HostIP != "127.0.0.1" || b[0].HostPort != l.port {
		t.Fatal("K2a published endpoint changed")
	}
}

func (l *k2aLab) connect(t *testing.T, role string) *pgx.Conn {
	t.Helper()
	l.verifyRunning(t)
	conn, err := pgx.Connect(l.ctx, l.dsn(role))
	if err != nil {
		t.Fatal("K2a principal connection failed")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = conn.Close(ctx)
	})
	var database, user string
	if err := conn.QueryRow(l.ctx, "SELECT current_database(), current_user").Scan(&database, &user); err != nil || database != l.database || user != role {
		t.Fatal("unexpected K2a principal or database")
	}
	return conn
}

func (l *k2aLab) exec(t *testing.T, conn *pgx.Conn, sql string, args ...any) {
	t.Helper()
	if _, err := conn.Exec(l.ctx, sql, args...); err != nil {
		t.Fatalf("K2a statement failed: %v", err)
	}
}

func (l *k2aLab) roles(t *testing.T) *pgx.Conn {
	t.Helper()
	admin := l.connect(t, k2aAdmin)
	for _, role := range []string{k2aMigrator, k2aLoader, k2aReader, k2aOther} {
		l.exec(t, admin, fmt.Sprintf("CREATE ROLE %s LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD '%s'", role, l.passwords[role]))
	}
	db := pgx.Identifier{l.database}.Sanitize()
	l.exec(t, admin, "REVOKE ALL ON DATABASE "+db+" FROM PUBLIC")
	l.exec(t, admin, "GRANT CONNECT ON DATABASE "+db+" TO "+strings.Join([]string{k2aMigrator, k2aLoader, k2aReader, k2aOther}, ", "))
	l.exec(t, admin, "REVOKE ALL ON SCHEMA public FROM PUBLIC")
	return admin
}

func (l *k2aLab) dump(t *testing.T, schemaOnly bool) []byte {
	t.Helper()
	l.verifyRunning(t)
	args := []string{"exec", "-e", "PGPASSWORD=" + l.passwords[k2aAdmin], l.id, "pg_dump",
		"-h", "127.0.0.1", "-U", k2aAdmin, "-d", l.database, "--no-password"}
	if schemaOnly {
		args = append(args, "--schema-only")
	} else {
		args = append(args, "--format=custom")
	}
	data, err := k2aDocker(nil, args...)
	if err != nil || len(data) == 0 {
		t.Fatal("K2a backup failed")
	}
	return data
}

func (l *k2aLab) restore(t *testing.T, data []byte) error {
	t.Helper()
	l.verifyRunning(t)
	if len(data) == 0 {
		t.Fatal("missing backup")
	}
	_, err := k2aDocker(data, "exec", "-i", "-e", "PGPASSWORD="+l.passwords[k2aAdmin], l.id, "pg_restore",
		"-h", "127.0.0.1", "-U", k2aAdmin, "-d", l.database, "--no-password", "--exit-on-error", "--single-transaction")
	return err
}
