package credential

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPasswordHashAndVerify(t *testing.T) {
	h, err := NewPasswordHasher(1)
	if err != nil {
		t.Fatal(err)
	}
	pw := strings.Repeat("secret phrase", 2)
	verifier, err := h.Hash(context.Background(), pw)
	if err != nil {
		t.Fatal(err)
	}
	if string(verifier.Digest) == pw || len(verifier.Salt) != 16 {
		t.Fatal("verifier contains unexpected material")
	}
	again, err := h.Hash(context.Background(), pw)
	if err != nil {
		t.Fatal(err)
	}
	if string(again.Salt) == string(verifier.Salt) {
		t.Fatal("salt must be random")
	}
	valid, err := h.Verify(context.Background(), pw, verifier)
	if err != nil || !valid {
		t.Fatalf("verification: %v, %v", valid, err)
	}
	valid, err = h.Verify(context.Background(), pw+"wrong", verifier)
	if err != nil || valid {
		t.Fatalf("wrong password accepted: %v, %v", valid, err)
	}
	altered := verifier
	altered.MemoryKiB = 1 << 30
	valid, err = h.Verify(context.Background(), pw, altered)
	if valid || !errors.Is(err, ErrVerifierInvalid) {
		t.Fatalf("unbounded DB parameter: %v", err)
	}
}

func TestPasswordAdmissionCancellation(t *testing.T) {
	h, err := NewPasswordHasher(1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Hash(ctx, strings.Repeat("hello", 4)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := NewPasswordHasher(0); !errors.Is(err, ErrKDFUnavailable) {
		t.Fatal("zero capacity accepted")
	}
}

func BenchmarkPasswordHashCandidate(b *testing.B) {
	h, err := NewPasswordHasher(1)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < b.N; i++ {
		_, err := h.Hash(context.Background(), "a benchmark-only secret passphrase")
		if err != nil {
			b.Fatal(err)
		}
	}
}
