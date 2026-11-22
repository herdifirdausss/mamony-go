package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
)

type syntheticRecoveryCodeGrantStore struct {
	allowed bool
	err     error
	calls   int
}
func (f *syntheticRecoveryCodeGrantStore) StageRecoveryCodes(_ context.Context, subject string,
	epoch int64, digest []byte) (bool, error) {
	f.calls++
	if subject == "" || epoch <= 0 || len(digest) != 32 {
		return false, errors.New("unsafe grant parameters")
	}
	return f.allowed, f.err
}

func TestRecoveryCodeEnrollmentApprovalDigestAndRedaction(t *testing.T) {
	raw := bytes.Repeat([]byte{0x76}, 32)
	bearer := base64.RawURLEncoding.EncodeToString(raw)
	d, err := RecoveryEnrollmentApprovalDigest(bearer)
	if err != nil || d == ([32]byte{}) {
		t.Fatal("canonical 256-bit enrollment proof rejected")
	}
	offline, err := OfflineRecoveryCodeDigest(bearer)
	if err != nil || offline == d || d == sha256.Sum256(raw) {
		t.Fatal("bootstrap approval must not share the offline recovery code domain")
	}
	for _, invalid := range []string{"", bearer + "=", bearer + "!", bearer[:42],
		strings.Repeat("*", 43)} {
		if _, err := RecoveryEnrollmentApprovalDigest(invalid); !errors.Is(err, ErrRecoveryCodeManagerDenied) {
			t.Fatal("malformed external approval proof was accepted")
		}
	}
	var set RecoveryCodeSet
	seen := make(map[string]bool)
	for i := range set.codes {
		code, err := NewOfflineRecoveryCode()
		if err != nil {
			t.Fatal(err)
		}
		set.codes[i] = code
		seen[code.TrustedOfflineDisplay()] = true
	}
	if len(seen) != 5 || len(set.ForTrustedDisplay()) != 5 {
		t.Fatal("recovery codes were not generated as five independent 256-bit proofs")
	}
	for _, raw := range set.ForTrustedDisplay() {
		if strings.Contains(fmt.Sprintf("%v|%+v|%#v", set, set, set), raw) {
			t.Fatal("offline recovery code set leaked through fmt")
		}
		if val := set.LogValue(); val.Kind() != slog.KindString || strings.Contains(val.String(), raw) {
			t.Fatal("offline recovery code set leaked through slog")
		}
	}
	if out, err := json.Marshal(set); err == nil || len(out) != 0 {
		t.Fatal("offline recovery code set unexpectedly serialized")
	}
	if out, err := set.MarshalText(); err == nil || len(out) != 0 {
		t.Fatal("offline recovery code set unexpectedly supported text serialization")
	}
}

func TestRecoveryCodeRotationGrantPurposeAssertion(t *testing.T) {
	store := &syntheticRecoveryCodeGrantStore{allowed: true}
	a, err := NewRecoveryCodeGrantAuthority(store)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := a.Issue(context.Background(), "sub_AAAAAAAAAAAAAAAAAAAAAA", 2)
	if err != nil || grant.secret == "" || store.calls != 1 {
		t.Fatal("verified code grant did not create a single-purpose SQL assertion")
	}
	if grant.String() == grant.secret {
		t.Fatal("grant leaked through formatting")
	}
	store.allowed = false
	if _, err := a.Issue(context.Background(), "sub_AAAAAAAAAAAAAAAAAAAAAA", 2); !errors.Is(err, ErrSessionDenied) {
		t.Fatal("denied grant was issued")
	}
	store.err = errors.New("sensitive SQL DSN")
	if _, err := a.Issue(context.Background(), "sub_AAAAAAAAAAAAAAAAAAAAAA", 2);
		!errors.Is(err, ErrSessionUnavailable) || err.Error() != ErrSessionUnavailable.Error() {
		t.Fatal("grant failure leaked a database secret")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Issue(ctx, "sub_AAAAAAAAAAAAAAAAAAAAAA", 2);
		!errors.Is(err, ErrSessionUnavailable) {
		t.Fatal("cancelled grant staging was not rejected")
	}
	a.random = io.LimitReader(bytes.NewReader(nil), 1)
	if _, err := a.Issue(context.Background(), "sub_AAAAAAAAAAAAAAAAAAAAAA", 2);
		!errors.Is(err, ErrSessionUnavailable) {
		t.Fatal("broken entropy source issued an authentication grant")
	}
}
