package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestAuthenticationGrantSecretAndAmbiguousStageFailClosed(t *testing.T) {
	grant, store := testAuthenticationGrantAuthority(t)
	subject := "sub_AAAAAAAAAAAAAAAAAAAAAA"
	issued, err := grant.Issue(context.Background(), subject)
	if err != nil || store.stages != 1 || store.subject != subject {
		t.Fatal("valid grant stage failed")
	}
	if len(issued.secret) != 43 || len(store.digest) != 32 {
		t.Fatal("grant has wrong entropy or storage format")
	}
	raw, err := base64.RawURLEncoding.DecodeString(issued.secret)
	if err != nil || len(raw) != 32 || bytes.Equal(store.digest, raw) {
		t.Fatal("grant digest contained or copied raw proof secret")
	}
	want := sha256.Sum256(append([]byte(passwordLoginGrantDomain), raw...))
	if !bytes.Equal(want[:], store.digest) {
		t.Fatal("grant proof requires a distinct domain-separated digest")
	}
	if strings.Contains(fmt.Sprint(issued), issued.secret) ||
		strings.Contains(fmt.Sprintf("%#v", issued), issued.secret) {
		t.Fatal("grant leaked through unstructured formatting")
	}
	if b, err := json.Marshal(issued); err == nil || bytes.Contains(b, []byte(issued.secret)) {
		t.Fatal("grant must refuse JSON serialization")
	}
	if _, err := authenticationGrantDigest("invalid"); !errors.Is(err, ErrSessionDenied) {
		t.Fatal("malformed proof was accepted")
	}
	count := store.stages
	if _, err := grant.Issue(context.Background(), "not-a-subject"); !errors.Is(err, ErrSessionDenied) ||
		store.stages != count {
		t.Fatal("invalid subject reached grant stage")
	}
	store.allow = false
	if value, err := grant.Issue(context.Background(), subject); !errors.Is(err, ErrSessionDenied) ||
		value.secret != "" {
		t.Fatal("denied grant returned a secret")
	}
	store.allow = true
	store.err = errors.New("ambiguous commit with private DSN")
	if value, err := grant.Issue(context.Background(), subject); !errors.Is(err, ErrSessionUnavailable) ||
		value.secret != "" || strings.Contains(err.Error(), "DSN") {
		t.Fatal("ambiguous grant stage leaked proof or DB details")
	}
	store.err = nil
	grant.random = bytes.NewReader([]byte{1, 2, 3})
	if value, err := grant.Issue(context.Background(), subject); !errors.Is(err, ErrSessionUnavailable) ||
		value.secret != "" {
		t.Fatal("partial CSPRNG produced grant")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if value, err := grant.Issue(ctx, subject); !errors.Is(err, ErrSessionUnavailable) || value.secret != "" {
		t.Fatal("cancelled login minted grant")
	}
	var nilAuthority *AuthenticationGrantAuthority
	if value, err := nilAuthority.Issue(context.Background(), subject); !errors.Is(err, ErrSessionUnavailable) ||
		value.secret != "" {
		t.Fatal("nil grant issuer accepted")
	}
}
