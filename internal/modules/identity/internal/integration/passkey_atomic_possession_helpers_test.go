//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync/atomic"

	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	"github.com/jackc/pgx/v5"
)

// syntheticPostCommitAckLoss runs the real PostgreSQL SQL transaction to
// confirmed COMMIT, then deliberately discards the success acknowledgement
// before the repository adapter sees it. This test-only transport boundary
// is not a simulated network/TCP failure; it proves the durable SQL state
// is not retried when application acknowledgement is uncertain.
type syntheticPostCommitAckLoss struct {
	delegate identitypg.Transactor
}

func (s syntheticPostCommitAckLoss) WithTx(ctx context.Context, work func(pgx.Tx) error) error {
	if err := s.delegate.WithTx(ctx, work); err != nil {
		return err
	}
	return errors.New("synthetic lost acknowledgement after definite database commit")
}

// wireCommitCutPoint is a deterministic, TEST-ONLY TCP failure point.
// Pre-COMMIT prevents even sending the request. Post-write closes
// immediately after the full COMMIT request is accepted by the local TCP
// socket but before any server response; the outcome is intentionally
// uncertain. Post-COMMIT closes after PostgreSQL confirms COMMIT.
type wireCommitCutPoint uint8

const (
	wireNoCut wireCommitCutPoint = iota
	wireBeforeCommit
	wireAfterCommit
	wireAfterCommitWrite
)

// wireCommitFaultConn reads raw PostgreSQL frames on disposable loopback
// only (TLS disabled); it cannot be used on production connections.
type wireCommitFaultConn struct {
	net.Conn
	cut              wireCommitCutPoint
	armed            atomic.Bool
	sawCommitRequest atomic.Bool
	sentCommitWrite  atomic.Bool
	sawCommitted     atomic.Bool
	pending          []byte
}

func (c *wireCommitFaultConn) Write(p []byte) (int, error) {
	if bytes.Contains(bytes.ToUpper(p), []byte("COMMIT\x00")) {
		c.sawCommitRequest.Store(true)
		if c.cut == wireBeforeCommit {
			// No COMMIT request bytes reach the server. Closing this
			// session aborts its outstanding transaction.
			_ = c.Conn.Close()
			return 0, io.ErrClosedPipe
		}
		if c.cut == wireAfterCommit {
			c.armed.Store(true)
		}
		if c.cut == wireAfterCommitWrite {
			n, err := c.Conn.Write(p)
			if n == len(p) && err == nil {
				// The local TCP stack accepted the complete COMMIT
				// request. The PostgreSQL server may or may not have
				// committed before its socket is disconnected.
				c.sentCommitWrite.Store(true)
				_ = c.Conn.Close()
			}
			return n, err
		}
	}
	return c.Conn.Write(p)
}

func (c *wireCommitFaultConn) Read(p []byte) (int, error) {
	if c.cut != wireAfterCommit || !c.armed.Load() {
		return c.Conn.Read(p)
	}
	if len(c.pending) == 0 {
		var header [5]byte
		if _, err := io.ReadFull(c.Conn, header[:]); err != nil {
			return 0, err
		}
		length := binary.BigEndian.Uint32(header[1:])
		if length < 4 || length > 1<<20 {
			_ = c.Conn.Close()
			return 0, errors.New("invalid PostgreSQL commit response frame")
		}
		body := make([]byte, int(length)-4)
		if _, err := io.ReadFull(c.Conn, body); err != nil {
			return 0, err
		}
		if header[0] == 'C' && bytes.Equal(body, []byte("COMMIT\x00")) {
			c.sawCommitted.Store(true)
			_ = c.Conn.Close()
			return 0, io.ErrUnexpectedEOF
		}
		c.pending = append(header[:], body...)
	}
	n := copy(p, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}

// wireCommitFaultTransactor uses the original restricted finisher role,
// real v36 SQL and repository adapter with a TEST-ONLY TCP dialer.
// It does not alter the platform pool, privilege grants or public routes.
type wireCommitFaultTransactor struct {
	dsn        string
	cut        wireCommitCutPoint
	wire       *wireCommitFaultConn
	backendPID int32
}

func (t *wireCommitFaultTransactor) WithTx(
	ctx context.Context, operation func(pgx.Tx) error) error {
	cfg, err := pgx.ParseConfig(t.dsn)
	if err != nil {
		return err
	}
	cfg.Fallbacks = nil
	cfg.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil || (host != "localhost" && host != "127.0.0.1") {
			return nil, errors.New("wire-fault test must use a loopback PostgreSQL endpoint")
		}
		raw, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		t.wire = &wireCommitFaultConn{Conn: raw, cut: t.cut}
		return t.wire, nil
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var principal string
	if err := conn.QueryRow(ctx, "SELECT current_user, pg_backend_pid()").
		Scan(&principal, &t.backendPID); err != nil {
		return err
	}
	if principal != "mamony_identity_passkey_activation_finisher_runtime" {
		return errors.New("wire-fault test did not connect as the isolated finisher")
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := operation(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// resolveThenGateActivationIssuer proves that the genuine v10 session
// Resolve completed BEFORE the v34 SQL issuer is released. Without this
// barrier, an admin-held subject row would block v10's FOR SHARE rather
// than the actual issuer SQL whose lock ordering we need to exercise.
type resolveThenGateActivationIssuer struct {
	delegate usecase.PasskeyActivationChallengeStore
	arrived  chan<- struct{}
	release  <-chan struct{}
}

func (s resolveThenGateActivationIssuer) Issue(ctx context.Context,
	sessionDigest [32]byte, credentialID []byte, challengeID string,
	nonce [32]byte, rpID, origin string) (bool, error) {
	select {
	case s.arrived <- struct{}{}:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	select {
	case <-s.release:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return s.delegate.Issue(ctx, sessionDigest, credentialID, challengeID,
		nonce, rpID, origin)
}

// resolveThenGateActivationReader uses the same post-Resolve barrier to
// ensure that the real v34 reader, not only the shared session resolver,
// contends for the exact subject lock held by the disposable test owner.
type resolveThenGateActivationReader struct {
	delegate usecase.PasskeyActivationEvidenceStore
	arrived  chan<- struct{}
	release  <-chan struct{}
}

func (s resolveThenGateActivationReader) Read(ctx context.Context,
	sessionDigest [32]byte, challengeID string) (usecase.PasskeyActivationEvidence, bool, error) {
	select {
	case s.arrived <- struct{}{}:
	case <-ctx.Done():
		return usecase.PasskeyActivationEvidence{}, false, ctx.Err()
	}
	select {
	case <-s.release:
	case <-ctx.Done():
		return usecase.PasskeyActivationEvidence{}, false, ctx.Err()
	}
	return s.delegate.Read(ctx, sessionDigest, challengeID)
}
