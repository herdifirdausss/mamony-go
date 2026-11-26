//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/handler"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/integration/internal/fixture"
	identitypg "github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/repository/postgres"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
	platformpg "github.com/herdifirdausss/mamony-go/internal/platform/postgres"
)

type syntheticConfirmationRejection struct{ calls atomic.Int64 }

func (f *syntheticConfirmationRejection) Confirm(context.Context, string) error {
	f.calls.Add(1)
	return usecase.ErrEmailProofInvalid
}

// TestIdentityZConfirmationAdmissionHTTPDB uses real admission database writes
// before a synthetic no-op confirmer. The HTTP layer does not get access to
// credential data, delivery keys or a live verification token.
func TestIdentityZConfirmationAdmissionHTTPDB(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	dsn := fixture.GuardedDB(t, "TEST_IDENTITY_RUNTIME_URL", "mamony_identity_runtime")
	pool, err := platformpg.Open(ctx, platformpg.Config{
		DSN: dsn, RuntimeRole: "mamony_identity_runtime", MaxConns: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store, err := identitypg.NewAdmissionStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := handler.NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	confirmer := &syntheticConfirmationRejection{}
	var proofPort handler.EmailConfirmer = confirmer
	handlers := make([]http.Handler, 2)
	for i := range handlers {
		gate, err := usecase.NewConfirmationAdmission(store,
			bytes.Repeat([]byte{0xe1}, 32))
		if err != nil {
			t.Fatal(err)
		}
		var admissionPort handler.ConfirmationAdmission = gate
		handlers[i], err = handler.NewEmailConfirmationHTTP(peer, admissionPort,
			proofPort, "https://app.example.test")
		if err != nil {
			t.Fatal(err)
		}
	}
	secret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x3d}, 32))
	const requests = 20
	start := make(chan struct{})
	results := make(chan int, requests)
	var group sync.WaitGroup
	for i := 0; i < requests; i++ {
		h := handlers[i%2]
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			req := httptest.NewRequest(http.MethodPost, "https://app.example.test/v1/identity/verify-email",
				strings.NewReader(`{"token":"`+secret+`"}`))
			req.TLS = &tls.ConnectionState{}
			req.RemoteAddr = "[2001:db8:cc:dd::20]:4567"
			req.Header.Set("Origin", "https://app.example.test")
			req.Header.Set("Sec-Fetch-Site", "same-origin")
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, req.WithContext(ctx))
			results <- recorder.Code
		}()
	}
	close(start)
	group.Wait()
	close(results)
	denied, attempted := 0, 0
	for status := range results {
		switch status {
		case http.StatusBadRequest: // allowed; synthetic verifier returns invalid
			attempted++
		case http.StatusTooManyRequests:
			denied++
		default:
			t.Fatalf("unexpected HTTP confirmation response %d", status)
		}
	}
	if attempted != 8 || denied != requests-8 || confirmer.calls.Load() != 8 {
		t.Fatalf("shared budget not enforced: attempted=%d denied=%d verifierCalls=%d",
			attempted, denied, confirmer.calls.Load())
	}
}
