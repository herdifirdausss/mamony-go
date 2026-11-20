package app

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

func TestRejectUnsafeAddresses(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, cfg := range []Config{
		{PublicAddr:"127.0.0.1:8080", PrivateAddr:"127.0.0.1:8080"},
		{PublicAddr:"127.0.0.1:8080", PrivateAddr:"0.0.0.0:8081"},
		{PublicAddr:"", PrivateAddr:"127.0.0.1:8081"},
	} {
		if err := Run(context.Background(), log, cfg); err == nil { t.Fatalf("accepted unsafe config: %+v", cfg) }
	}
}

func TestStartsStops(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	addr1, addr2 := availableAddr(t), availableAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := make(chan error, 1)
	go func(){ errs <- Run(ctx, log, Config{PublicAddr:addr1, PrivateAddr:addr2}) }()
	deadline := time.After(3*time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr2, 50*time.Millisecond)
		if err == nil { _=c.Close(); break }
		select { case <-deadline: t.Fatal("private listener never started"); default: time.Sleep(10*time.Millisecond) }
	}
	cancel()
	select { case err := <-errs: if err != nil {t.Fatal(err)}; case <-time.After(3*time.Second): t.Fatal("shutdown timed out") }
}

func availableAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {t.Fatal(err)}
	a := ln.Addr().String()
	if err := ln.Close(); err != nil { t.Fatal(err) }
	return a
}
