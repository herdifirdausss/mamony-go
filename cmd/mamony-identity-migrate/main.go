// Command mamony-identity-migrate applies forward-only Identity SQL with a
// dedicated least-privileged migrator. Never call this from the HTTP app.
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
	dir := os.Getenv("MAMONY_IDENTITY_MIGRATIONS_DIR")
	if dir == "" {
		dir = "db/migrations/identity"
	}
	if err := migration.RunIdentity(ctx, os.Getenv("MAMONY_IDENTITY_MIGRATION_DSN"), dir); err != nil {
		fmt.Fprintln(os.Stderr, "identity migration blocked:", err)
		os.Exit(1)
	}
}
