package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func TestRunRejectsUnsafeAddresses(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range []Config{
		{PublicAddr: "127.0.0.1:8080", PrivateAddr: "127.0.0.1:8080"},
		{PublicAddr: "127.0.0.1:8080", PrivateAddr: "0.0.0.0:8081"},
		{PublicAddr: "", PrivateAddr: "127.0.0.1:8081"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		err := Run(ctx, logger, tc)
		cancel()
		if err == nil {
			t.Fatalf("unsafe config accepted: %+v", tc)
		}
	}
}

type listenerAddresses struct {
	Public  string `json:"public"`
	Private string `json:"private"`
}

// startLogWriter records bound addresses from the startup message. Both
// listeners are bound on port zero, so the test never releases and reclaims a
// preselected port.
type startLogWriter struct {
	started chan<- listenerAddresses
}

func (w startLogWriter) Write(p []byte) (int, error) {
	var entry struct {
		Msg string `json:"msg"`
		listenerAddresses
	}
	if json.Unmarshal(p, &entry) == nil && entry.Msg == "mamony simulator HTTP foundation started" {
		select {
		case w.started <- entry.listenerAddresses:
		default:
		}
	}
	return len(p), nil
}

func TestRunStartsAndStops(t *testing.T) {
	started := make(chan listenerAddresses, 1)
	logger := slog.New(slog.NewJSONHandler(startLogWriter{started: started}, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, logger, Config{
			PublicAddr:  "127.0.0.1:0",
			PrivateAddr: "localhost:0",
		})
	}()

	var addresses listenerAddresses
	select {
	case addresses = <-started:
	case err := <-errCh:
		t.Fatalf("server stopped before startup: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("listeners did not start")
	}

	client := &http.Client{Timeout: time.Second}
	for _, tc := range []struct {
		name string
		url  string
		code int
	}{
		{"private readiness", "http://" + addresses.Private + "/readyz", http.StatusOK},
		{"readiness not public", "http://" + addresses.Public + "/readyz", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.Get(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.code {
				t.Fatalf("status=%d want=%d", resp.StatusCode, tc.code)
			}
		})
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("shutdown error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown timed out")
	}
}
