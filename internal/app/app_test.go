package app

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

func TestRunRejectsUnsafeAddresses(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range []Config{
		{PublicAddr: "127.0.0.1:8080", PrivateAddr: "127.0.0.1:8080"},
		{PublicAddr: "127.0.0.1:8080", PrivateAddr: "0.0.0.0:8081"},
		{PublicAddr: "", PrivateAddr: "127.0.0.1:8081"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		err := Run(ctx, logger, tc)
		cancel()
		if err == nil {
			t.Fatalf("unsafe config accepted: %+v", tc)
		}
	}
}

func TestRunStartsAndStops(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Dynamically selected ports avoid colliding with default local listeners.
	addr1 := availableAddr(t)
	addr2 := availableAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- Run(ctx, logger, Config{PublicAddr: addr1, PrivateAddr: addr2}) }()
	defer cancel()
	// The app may take a small amount of time to bind both listeners.
	deadline := time.After(3 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr2, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		select {
		case <-deadline:
			t.Fatal("private listener did not start")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("shutdown error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown timed out")
	}
}

func availableAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}
