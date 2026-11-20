package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/herdifirdausss/mamony-go/internal/app"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, logger, app.Config{
		PublicAddr: envOr("MAMONY_PUBLIC_ADDR", "127.0.0.1:8080"),
		PrivateAddr: envOr("MAMONY_PRIVATE_ADDR", "127.0.0.1:8081"),
	}); err != nil {
		logger.Error("mamony stopped", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" { return v }
	return fallback
}
