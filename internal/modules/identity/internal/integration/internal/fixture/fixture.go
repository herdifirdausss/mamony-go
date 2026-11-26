//go:build integration

// Package fixture provides input guards and identifiers for Identity tests.
package fixture

import (
	"crypto/rand"
	"encoding/base64"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

// GuardedDB reads the URL for a disposable loopback database role.
func GuardedDB(t *testing.T, key, role string) string {
	t.Helper()
	raw := os.Getenv(key)
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") ||
		u.User == nil || u.User.Username() != role ||
		(u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") ||
		len(u.Query()) != 1 || u.Query().Get("sslmode") != "disable" ||
		!strings.HasPrefix(u.Path, "/mamony_test_") ||
		strings.Contains(strings.TrimPrefix(u.Path, "/"), "/") ||
		u.Fragment != "" {
		t.Fatalf("%s must be the disposable loopback role fixture", key)
	}
	if _, ok := u.User.Password(); !ok {
		t.Fatalf("%s lacks fixture password", key)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		t.Fatalf("%s lacks explicit loopback port", key)
	}
	return raw
}

// RandomID returns a synthetic identifier with the given prefix.
func RandomID(t *testing.T, prefix string) string {
	t.Helper()
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw[:])
}
