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
