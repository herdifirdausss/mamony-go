package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

type fakeHasher struct{}

func (fakeHasher) Hash(_ context.Context, normalized string) (domain.PasswordVerifier, error) {
	return domain.PasswordVerifier{Version: 1, Digest: make([]byte, 32)}, nil
}

type fakeStore struct {
	calls int
	got   PendingRegistration
	err   error
}

func (f *fakeStore) CreatePendingWithObligation(_ context.Context, r PendingRegistration) error {
	f.calls++
	f.got = r
	return f.err
}

func TestRegisterPendingUsesUnverifiedAtomicPort(t *testing.T) {
	store := &fakeStore{}
	h := fakeHasher{}
	s, err := NewRegistration(store, h, domain.PasswordPolicy{IsBlocked: func(string, string) bool { return false }})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterPending(context.Background(), "ALICE@EXAMPLE.COM", strings.Repeat("correct horse", 2)); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || store.got.Status != "UNVERIFIED" || store.got.CanonicalEmail != "alice@example.com" {
		t.Fatalf("expected one pending atomic call, got %+v", store.got)
	}
	if store.got.SubjectID == "" || store.got.EventID == "" || len(store.got.PasswordVerifier.Digest) != 32 {
		t.Fatal("missing opaque authority identifiers or verifier")
	}
}

func TestRegistrationRejectsInvalidBeforeStoreAndMasksDuplicate(t *testing.T) {
	store := &fakeStore{}
	h := fakeHasher{}
	s, _ := NewRegistration(store, h, domain.PasswordPolicy{IsBlocked: func(string, string) bool { return false }})
	if err := s.RegisterPending(context.Background(), "not-email", strings.Repeat("x", 20)); err == nil || store.calls != 0 {
		t.Fatal("invalid email reached store")
	}
	store.err = ErrAliasAlreadyReserved
	if err := s.RegisterPending(context.Background(), "alice@example.com", strings.Repeat("x", 20)); err != nil {
		t.Fatalf("alias enumeration: %v", err)
	}
	store.err = errors.New("storage contains confidential SQL")
	if err := s.RegisterPending(context.Background(), "bob@example.com", strings.Repeat("x", 20)); !errors.Is(err, ErrRegistrationUnavailable) || strings.Contains(err.Error(), "confidential") {
		t.Fatalf("storage error leaked: %v", err)
	}
}
