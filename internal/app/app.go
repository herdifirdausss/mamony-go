package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/platform/httpedge"
)

// Config describes the public and private listener addresses.
type Config struct {
	PublicAddr  string
	PrivateAddr string
}

// Run serves both HTTP listeners until the context is canceled.
func Run(ctx context.Context, logger *slog.Logger, cfg Config) error {
	if cfg.PublicAddr == "" || cfg.PrivateAddr == "" || cfg.PublicAddr == cfg.PrivateAddr {
		return errors.New("public and private listener addresses must be non-empty and different")
	}
	publicHost, _, err := net.SplitHostPort(cfg.PublicAddr)
	if err != nil {
		return fmt.Errorf("invalid public address: %w", err)
	}
	privateHost, privatePort, err := net.SplitHostPort(cfg.PrivateAddr)
	if err != nil {
		return fmt.Errorf("invalid private address: %w", err)
	}
	// Treat localhost as a loopback alias without using host resolution.
	if privateHost == "localhost" {
		privateHost = "127.0.0.1"
		cfg.PrivateAddr = net.JoinHostPort(privateHost, privatePort)
	}
	if privateHost != "127.0.0.1" && privateHost != "::1" {
		return errors.New("private listener must bind to loopback in this scaffold")
	}
	if publicHost == "" {
		return errors.New("public listener must have an explicit bind host")
	}

	publicLn, err := net.Listen("tcp", cfg.PublicAddr)
	if err != nil {
		return fmt.Errorf("bind public: %w", err)
	}
	defer func() {
		if err := publicLn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			logger.Warn("close public listener", "error", err)
		}
	}()
	privateLn, err := net.Listen("tcp", cfg.PrivateAddr)
	if err != nil {
		return fmt.Errorf("bind private: %w", err)
	}
	defer func() {
		if err := privateLn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			logger.Warn("close private listener", "error", err)
		}
	}()

	public := newServer(httpedge.PublicHandler())
	private := newServer(httpedge.PrivateHandler())
	var wg sync.WaitGroup
	serveErrors := make(chan error, 2)
	serve := func(name string, srv *http.Server, ln net.Listener) {
		defer wg.Done()
		if e := srv.Serve(ln); e != nil && !errors.Is(e, http.ErrServerClosed) {
			serveErrors <- fmt.Errorf("%s listener: %w", name, e)
		}
	}
	wg.Add(2)
	go serve("public", public, publicLn)
	go serve("private", private, privateLn)
	logger.Info("mamony simulator HTTP foundation started", "public", publicLn.Addr().String(), "private", privateLn.Addr().String())
	// Exit on cancellation or an unexpected listener failure; never wait indefinitely
	// for context cancellation after an HTTP listener has failed.
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-serveErrors:
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	e1 := public.Shutdown(shutdownCtx)
	e2 := private.Shutdown(shutdownCtx)
	wg.Wait()
	return errors.Join(e1, e2, serveErr)
}

func newServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}
