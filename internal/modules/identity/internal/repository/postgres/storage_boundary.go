// Package postgres persists Identity state through credential-limited PostgreSQL store adapters.
// It never authenticates users or authorizes HTTP requests.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrStorageUnavailable is returned when an Identity PostgreSQL store cannot complete an operation.
var ErrStorageUnavailable = errors.New("identity registration storage unavailable")

// Transactor is the narrow transaction boundary shared by Identity PostgreSQL stores.
// Implementations must use a credential-limited runtime connection pool.
type Transactor interface {
	WithTx(context.Context, func(pgx.Tx) error) error
}
