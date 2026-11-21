package identity

import (
	"context"
	"errors"
	"testing"
)

func TestRetentionFacadeFailsClosedWithoutPool(t *testing.T) {
	for _, tc := range []struct{ctx context.Context; dsn string}{
		{nil, "unused"},
		{context.Background(), ""},
		{context.Background(), "not a PostgreSQL URL"},
	} {
		_, err := PurgeRetention(tc.ctx, RetentionConfig{DSN: tc.dsn})
		if !errors.Is(err, ErrRetentionRuntime) {
			t.Fatalf("untrusted runtime configuration reached maintenance: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PurgeRetention(ctx, RetentionConfig{}); !errors.Is(err, ErrRetentionRuntime) {
		t.Fatal("cancelled maintenance request accepted")
	}
}
