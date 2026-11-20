package main
import "testing"
func TestPolicy(t *testing.T){
	base:=module+"/"
	for _,tc:=range []struct{from,to string;allowed bool}{
		{"internal/app","internal/modules/ledger",true},
		{"internal/modules/ledger/internal/handler/httpapi","internal/modules/ledger/internal/usecase",true},
		{"internal/modules/payment","internal/modules/ledger",false},
		{"internal/modules/ledger/internal/usecase","internal/modules/ledger/internal/repository/postgres",false},
		{"internal/modules/ledger/internal/domain","internal/platform/dbtx",false},
		{"internal/platform/httpedge","internal/modules/ledger",false},
		{"internal/app","internal/modules/ledger/internal/domain",false},
	}{
		v:=violation(base+tc.from,base+tc.to)
		if (v=="")!=tc.allowed {t.Fatalf("%s -> %s: %q",tc.from,tc.to,v)}
	}
}
