package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
)

type passwordCredentialReaderFake struct {
	record AuthenticationCredential
	found  bool
	err    error
	calls  int
	email  string
}

func (f *passwordCredentialReaderFake) LookupCredential(_ context.Context, email string) (AuthenticationCredential, bool, error) {
	f.calls++
	f.email = email
	return f.record, f.found, f.err
}

type passwordVerifierFake struct {
	calls int
	pass  bool
	err   error
	got   domain.PasswordVerifier
	input string
}

func (f *passwordVerifierFake) Verify(_ context.Context, pw string, verifier domain.PasswordVerifier) (bool, error) {
	f.calls++
	f.got = verifier
	f.input = pw
	return f.pass, f.err
}

func TestPasswordAuthenticationDecisionMatrix(t *testing.T) {
	for _, tc := range []struct {
		name     string
		found    bool
		match    bool
		status   string
		verified bool
		wantOK   bool
	}{
		{"verified active and matched", true, true, "ACTIVE", true, true},
		{"active but password wrong", true, false, "ACTIVE", true, false},
		{"active without verified contact", true, true, "ACTIVE", false, false},
		{"unverified account", true, true, "UNVERIFIED", false, false},
		{"suspended account", true, true, "SUSPENDED", true, false},
		{"frozen account", true, true, "FROZEN", true, false},
		{"deleted account", true, true, "DELETED", true, false},
		{"unknown alias", false, false, "", false, false},
		// Even if a dummy verifier's synthetic digest matched, absence must
		// NEVER yield a subject or a candidate session.
		{"unknown alias dummy collision", false, true, "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			real := dummyAuthenticationVerifier()
			real.Salt = []byte("different-salt01")
			reader := &passwordCredentialReaderFake{
				found: tc.found,
				record: AuthenticationCredential{
					SubjectID:     "sub_AAAAAAAAAAAAAAAAAAAAAA",
					SessionEpoch:  3,
					Status:        tc.status,
					EmailVerified: tc.verified,
					Verifier:      real,
				},
			}
			verifier := &passwordVerifierFake{pass: tc.match}
			auth, err := NewPasswordAuthentication(reader, verifier)
			if err != nil {
				t.Fatal(err)
			}
			subject, err := auth.Authenticate(context.Background(), "ALICE@EXAMPLE.COM", "password phrase")
			if tc.wantOK {
				if err != nil || subject != "sub_AAAAAAAAAAAAAAAAAAAAAA" {
					t.Fatal("active verified credential did not yield internal subject candidate")
				}
			} else if !errors.Is(err, ErrPasswordAuthenticationDenied) ||
				subject != "" || err.Error() != ErrPasswordAuthenticationDenied.Error() {
				t.Fatal("negative password outcome was distinguishable or leaked subject")
			}
			if reader.calls != 1 || reader.email != "alice@example.com" ||
				verifier.calls != 1 || verifier.input != "password phrase" {
				t.Fatal("validly shaped known/unknown aliases must run one lookup and one KDF")
			}
			if tc.found {
				if string(verifier.got.Salt) != "different-salt01" {
					t.Fatal("known alias did not use stored verifier")
				}
			} else if len(verifier.got.Salt) != 16 || len(verifier.got.Digest) != 32 ||
				verifier.got.MemoryKiB != 65536 || verifier.got.Iterations != 3 {
				t.Fatal("unknown alias was not charged a real Argon2id parameter set")
			}
		})
	}
}

func TestPasswordAuthenticationUnavailableAndInputPolicy(t *testing.T) {
	reader := &passwordCredentialReaderFake{
		found: true,
		record: AuthenticationCredential{
			SubjectID:     "sub_AAAAAAAAAAAAAAAAAAAAAA",
			SessionEpoch:  3,
			Status:        "ACTIVE",
			EmailVerified: true,
			Verifier:      dummyAuthenticationVerifier(),
		},
	}
	verifier := &passwordVerifierFake{pass: true}
	auth, err := NewPasswordAuthentication(reader, verifier)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct{ email, password string }{
		{"not-an-email", "password phrase"},
		{"alice@example.com", ""},
		{"alice@example.com", strings.Repeat("x", 2049)},
		{"alice@example.com", "invalid\xffUTF8"},
	} {
		before := reader.calls
		subject, err := auth.Authenticate(context.Background(), input.email, input.password)
		if !errors.Is(err, ErrPasswordAuthenticationDenied) || subject != "" ||
			reader.calls != before {
			t.Fatal("malformed login must reject before DB/KDF without granting identity")
		}
	}
	reader.err = errors.New("private postgres DSN with secret password")
	if subject, err := auth.Authenticate(context.Background(), "alice@example.com", "password phrase"); subject != "" ||
		!errors.Is(err, ErrPasswordAuthenticationUnavailable) ||
		strings.Contains(err.Error(), "postgres") {
		t.Fatal("SQL outage error leaked")
	}
	reader.err = nil
	verifier.err = errors.New("KDF corrupted private salt")
	if subject, err := auth.Authenticate(context.Background(), "alice@example.com", "password phrase"); subject != "" ||
		!errors.Is(err, ErrPasswordAuthenticationUnavailable) ||
		strings.Contains(err.Error(), "salt") {
		t.Fatal("KDF failure was reported as wrong password or leaked secret")
	}
	verifier.err = nil
	ctx, stop := context.WithCancel(context.Background())
	stop()
	before := reader.calls
	if subject, err := auth.Authenticate(ctx, "alice@example.com", "password phrase"); subject != "" ||
		!errors.Is(err, ErrPasswordAuthenticationUnavailable) || reader.calls != before {
		t.Fatal("canceled login queried DB")
	}
	var absent *PasswordAuthentication
	if subject, err := absent.Authenticate(context.Background(), "alice@example.com", "password phrase"); subject != "" ||
		!errors.Is(err, ErrPasswordAuthenticationUnavailable) {
		t.Fatal("absent credential authority was allowed")
	}
	if _, err := NewPasswordAuthentication(nil, verifier); !errors.Is(err, ErrPasswordAuthenticationUnavailable) {
		t.Fatal("nil credential loader accepted")
	}
	if _, err := NewPasswordAuthentication(reader, nil); !errors.Is(err, ErrPasswordAuthenticationUnavailable) {
		t.Fatal("nil Argon2id verifier accepted")
	}
}

func TestPasswordAuthenticationCarriesVerifierSnapshotEpoch(t *testing.T) {
	reader := &passwordCredentialReaderFake{
		found: true,
		record: AuthenticationCredential{
			SubjectID: "sub_AAAAAAAAAAAAAAAAAAAAAA", SessionEpoch: 47,
			Status: "ACTIVE", EmailVerified: true,
			Verifier: dummyAuthenticationVerifier(),
		},
	}
	kdf := &passwordVerifierFake{pass: true}
	auth, err := NewPasswordAuthentication(reader, kdf)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := auth.AuthenticateWithEpoch(context.Background(), "alice@example.test", "password phrase")
	if err != nil || candidate.SubjectID != reader.record.SubjectID || candidate.SessionEpoch != 47 {
		t.Fatalf("password snapshot lost epoch or subject: %+v %v", candidate, err)
	}
	if reader.calls != 1 || kdf.calls != 1 {
		t.Fatal("one bounded lookup must precede one KDF")
	}
	reader.record.SessionEpoch = 0
	if result, err := auth.AuthenticateWithEpoch(context.Background(), "alice@example.test", "password phrase");
		result.SubjectID != "" || !errors.Is(err, ErrPasswordAuthenticationDenied) {
		t.Fatal("missing epoch cannot become a trusted login candidate")
	}
}
