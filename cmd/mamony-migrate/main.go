// Command mamony-migrate applies verified forward-only platform SQL migrations.
// It must run as a dedicated migrator outside the HTTP application process.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/platform/migration"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := migration.Run(ctx, os.Getenv("MAMONY_MIGRATION_DSN"), migrationDirectory()); err != nil {
		fmt.Fprintln(os.Stderr, "platform migration blocked:", err)
		os.Exit(1)
	}
}

func migrationDirectory() string {
	if dir := os.Getenv("MAMONY_MIGRATIONS_DIR"); dir != "" {
		return dir
	}
	return "db/migrations/platform"
}
