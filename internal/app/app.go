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

// Config owns listener endpoints for the single-process simulator.
type Config struct { PublicAddr, PrivateAddr string }

// Run serves public and private endpoints until cancellation or listener failure.
func Run(ctx context.Context, logger *slog.Logger, cfg Config) error {
	if cfg.PublicAddr == "" || cfg.PrivateAddr == "" || cfg.PublicAddr == cfg.PrivateAddr {
		return errors.New("public/private listener addresses must be distinct and nonempty")
	}
	publicHost, _, err := net.SplitHostPort(cfg.PublicAddr)
	if err != nil { return fmt.Errorf("public address: %w", err) }
	privateHost, _, err := net.SplitHostPort(cfg.PrivateAddr)
	if err != nil { return fmt.Errorf("private address: %w", err) }
	if publicHost == "" { return errors.New("public listener requires explicit bind host") }
	if privateHost != "127.0.0.1" && privateHost != "::1" && privateHost != "localhost" {
		return errors.New("private listener must bind to loopback in this starter")
	}
	pubLn, err := net.Listen("tcp", cfg.PublicAddr)
	if err != nil { return fmt.Errorf("bind public: %w", err) }
	defer pubLn.Close()
	privLn, err := net.Listen("tcp", cfg.PrivateAddr)
	if err != nil { return fmt.Errorf("bind private: %w", err) }
	defer privLn.Close()
	pubSrv, privSrv := newServer(httpedge.PublicHandler()), newServer(httpedge.PrivateHandler())
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	serve := func(name string, srv *http.Server, ln net.Listener) {
		defer wg.Done()
		if e := srv.Serve(ln); e != nil && !errors.Is(e, http.ErrServerClosed) {
			errs <- fmt.Errorf("%s: %w", name, e)
		}
	}
	wg.Add(2)
	go serve("public", pubSrv, pubLn)
	go serve("private", privSrv, privLn)
	logger.Info("mamony HTTP foundation started", "public", pubLn.Addr().String(), "private", privLn.Addr().String())
	var serveErr error
	select { case <-ctx.Done(): case serveErr = <-errs: }
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	e1, e2 := pubSrv.Shutdown(shutdownCtx), privSrv.Shutdown(shutdownCtx)
	wg.Wait()
	return errors.Join(e1, e2, serveErr)
}

func newServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler: handler, ReadHeaderTimeout: 5*time.Second, ReadTimeout: 10*time.Second,
		WriteTimeout: 15*time.Second, IdleTimeout: 30*time.Second, MaxHeaderBytes: 1<<20,
	}
}
