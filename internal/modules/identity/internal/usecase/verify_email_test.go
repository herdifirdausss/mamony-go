package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

type fakeEmailProofPort struct {
	issued     EmailProofRequest
	seen       []byte
	issues     int
	consumed   int
	issueErr   error
	consumeErr error
}

func (f *fakeEmailProofPort) Issue(_ context.Context, r EmailProofRequest) (EmailProofDestination, error) {
	f.issued = r
	f.issues++
	if f.issueErr != nil {
		return EmailProofDestination{}, f.issueErr
	}
	return EmailProofDestination{CanonicalEmail: "alice@example.test", ContactVersion: 1}, nil
}

func (f *fakeEmailProofPort) Consume(_ context.Context, digest []byte, event string) error {
	f.consumed++
	f.seen = digest
	if len(event) != 26 || event[:4] != "evt_" {
		return errors.New("missing security event")
	}
	return f.consumeErr
}

type fakeEnvelopeSealer struct{ fail bool }

func (s fakeEnvelopeSealer) Seal(_, _, _ string) (string, []byte, error) {
	if s.fail {
		return "", nil, errors.New("secret key access failed")
	}
	return "testkey", bytes.Repeat([]byte{0xA5}, 71), nil
}

func TestEmailProofPrepareAndConfirmNeverExposesPublicToken(t *testing.T) {
	fake := &fakeEmailProofPort{}
	s, err := NewEmailVerification(fake, fakeEnvelopeSealer{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Prepare(context.Background(), "sub_AAAAAAAAAAAAAAAAAAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	email, secret := out.ForTrustedDelivery()
	if email != "alice@example.test" || len(secret) != 43 || fake.issues != 1 ||
		fake.issued.SubjectID != "sub_AAAAAAAAAAAAAAAAAAAAAA" ||
		len(fake.issued.TokenDigest) != 32 || len(fake.issued.ChallengeID) != 26 ||
		len(fake.issued.SealedToken) != 71 || fake.issued.EnvelopeKeyID != "testkey" {
		t.Fatal("issued proof missing durable scoped digest or contact target")
	}
	if bytes.Contains(fake.issued.TokenDigest, []byte(secret)) {
		t.Fatal("plaintext bearer persisted in proof request")
	}
	if fmt.Sprint(out) != "[REDACTED email proof]" || fmt.Sprintf("%#v", out) != "[REDACTED email proof]" {
		t.Fatal("formatted proof leaked secret")
	}
	if _, err := json.Marshal(out); err == nil {
		t.Fatal("JSON serialization of proof must fail")
	}
	if err := s.Confirm(context.Background(), secret); err != nil {
		t.Fatalf("valid proof failed: %v", err)
	}
	if fake.consumed != 1 || !bytes.Equal(fake.seen, fake.issued.TokenDigest) {
		t.Fatal("verification digest does not match issuance")
	}
}

func TestEmailProofFailClosed(t *testing.T) {
	if _, err := NewEmailVerification(nil, fakeEnvelopeSealer{}); err == nil {
		t.Fatal("missing authority accepted")
	}
	store := &fakeEmailProofPort{}
	s, _ := NewEmailVerification(store, fakeEnvelopeSealer{})
	for _, secret := range []string{"", "short", "zz/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="} {
		if err := s.Confirm(context.Background(), secret); !errors.Is(err, ErrEmailProofInvalid) {
			t.Fatalf("noncanonical bearer accepted: %q %v", secret, err)
		}
	}
	if store.consumed != 0 {
		t.Fatal("bad token reached repository")
	}
	if _, err := s.Prepare(context.Background(), "not-a-subject"); err == nil {
		t.Fatal("malformed subject accepted")
	}
	if store.issues != 0 {
		t.Fatal("bad subject reached store")
	}
	if _, err := NewEmailVerification(store, nil); !errors.Is(err, ErrEmailProofUnavailable) {
		t.Fatal("missing encryption key accepted")
	}
	store.issueErr = errors.New("raw SQL error")
	if _, err := s.Prepare(context.Background(), "sub_AAAAAAAAAAAAAAAAAAAAAA"); !errors.Is(err, ErrEmailProofUnavailable) {
		t.Fatal("database issue details leaked")
	}
	store.issueErr = nil
	out, _ := s.Prepare(context.Background(), "sub_AAAAAAAAAAAAAAAAAAAAAA")
	_, secret := out.ForTrustedDelivery()
	store.consumeErr = errors.New("secret SQL error")
	if err := s.Confirm(context.Background(), secret); !errors.Is(err, ErrEmailProofUnavailable) ||
		err.Error() != ErrEmailProofUnavailable.Error() {
		t.Fatal("database consume error not classified or leaked sensitive details")
	}
	store.consumeErr = ErrEmailProofInvalid
	if err := s.Confirm(context.Background(), secret); !errors.Is(err, ErrEmailProofInvalid) ||
		err.Error() != ErrEmailProofInvalid.Error() {
		t.Fatal("invalid proof did not preserve generic invalid classification")
	}
	store.consumeErr = ErrEmailProofThrottled
	if err := s.Confirm(context.Background(), secret); !errors.Is(err, ErrEmailProofUnavailable) {
		t.Fatal("unexpected proof-store failure must fail closed as unavailable")
	}
	store.consumeErr = nil
	before := store.consumed
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if err := s.Confirm(canceled, secret); !errors.Is(err, ErrEmailProofUnavailable) ||
		store.consumed != before {
		t.Fatal("canceled confirmation reached verifier database")
	}
	var absent *EmailVerification
	if err := absent.Confirm(context.Background(), secret); !errors.Is(err, ErrEmailProofUnavailable) {
		t.Fatal("nil verifier was treated as invalid token")
	}
}

// TestResendDoesNotExposeBearerAndPreservesTrustedThrottle is an internal
// contract: no plaintext token is returned by the resend API.
func TestResendDoesNotExposeBearerAndPreservesTrustedThrottle(t *testing.T) {
	fake := &fakeEmailProofPort{}
	s, err := NewEmailVerification(fake, fakeEnvelopeSealer{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Resend(context.Background(), "sub_AAAAAAAAAAAAAAAAAAAAAA"); err != nil {
		t.Fatal(err)
	}
	if fake.issues != 1 || len(fake.issued.TokenDigest) != 32 {
		t.Fatal("resend did not create one digest-only delivery obligation")
	}
	fake.issueErr = ErrEmailProofThrottled
	if err := s.Resend(context.Background(), "sub_AAAAAAAAAAAAAAAAAAAAAA"); !errors.Is(err, ErrEmailProofThrottled) {
		t.Fatalf("internal trusted throttle not returned: %v", err)
	}
	fake.issueErr = errors.New("sensitive SQL database error")
	if err := s.Resend(context.Background(), "sub_AAAAAAAAAAAAAAAAAAAAAA"); !errors.Is(err, ErrEmailProofUnavailable) ||
		err.Error() != ErrEmailProofUnavailable.Error() {
		t.Fatalf("raw SQL details escaped internal resend: %v", err)
	}
}
