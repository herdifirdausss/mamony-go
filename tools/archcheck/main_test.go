package main

import "testing"

func TestImportPolicy(t *testing.T) {
	t.Parallel()
	base := module + "/"
	cases := []struct {
		name, from, to string
		allowed        bool
	}{
		{"app to capability facade", "internal/app", "internal/modules/ledger", true},
		{"handler to usecase", "internal/modules/ledger/internal/handler/httpapi", "internal/modules/ledger/internal/usecase", true},
		{"repository to usecase contract", "internal/modules/ledger/internal/repository/postgres", "internal/modules/ledger/internal/usecase", true},
		{"module crossing", "internal/modules/payment", "internal/modules/ledger", false},
		{"usecase importing repository", "internal/modules/ledger/internal/usecase", "internal/modules/ledger/internal/repository/postgres", false},
		{"handler importing repository", "internal/modules/ledger/internal/handler/httpapi", "internal/modules/ledger/internal/repository/postgres", false},
		{"domain importing platform", "internal/modules/ledger/internal/domain", "internal/platform/dbtx", false},
		{"platform importing module", "internal/platform/httpedge", "internal/modules/ledger", false},
		{"module importing app", "internal/modules/ledger", "internal/app", false},
		{"app importing private internals", "internal/app", "internal/modules/ledger/internal/domain", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := violation(base+tc.from, base+tc.to)
			if (got == "") != tc.allowed {
				t.Fatalf("violation=%q allowed=%v", got, tc.allowed)
			}
		})
	}
}

func TestProductionImportPolicy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, from, to string
		allowed        bool
	}{
		{"domain database sql", "internal/modules/ledger/internal/domain", "database/sql", false},
		{"domain HTTP", "internal/modules/ledger/internal/domain", "net/http", false},
		{"usecase pgx", "internal/modules/ledger/internal/usecase", "github.com/jackc/pgx/v5/pgxpool", false},
		{"usecase redis", "internal/modules/ledger/internal/usecase", "github.com/redis/go-redis/v9", false},
		{"usecase RabbitMQ", "internal/modules/ledger/internal/usecase", "github.com/rabbitmq/amqp091-go", false},
		{"usecase handler HTTP", "internal/modules/ledger/internal/usecase", "github.com/go-chi/chi/v5", false},
		{"domain standard values", "internal/modules/ledger/internal/domain", "math/big", true},
		{"domain own port", "internal/modules/ledger/internal/domain", module + "/internal/modules/ledger/internal/domain/money", true},
		{"repository SQL", "internal/modules/ledger/internal/repository/postgres", "database/sql", true},
		{"handler HTTP", "internal/modules/ledger/internal/handler/httpapi", "net/http", true},
		{"platform HTTP", "internal/platform/httpedge", "net/http", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from := module + "/" + tc.from
			got := productionViolation(from, tc.to)
			if (got == "") != tc.allowed {
				t.Fatalf("productionViolation(%q, %q)=%q; allowed=%v", tc.from, tc.to, got, tc.allowed)
			}
		})
	}
}
