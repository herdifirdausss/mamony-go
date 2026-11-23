package usecase

import (
	"errors"
	"strings"
	"testing"
)

func TestPasskeyRelyingPartyPinsExactHTTPSOrigin(t *testing.T) {
	t.Parallel()
	p, err := NewPasskeyRelyingParty("login.example.test", "https://login.example.test")
	if err != nil || !p.Configured() {
		t.Fatalf("valid strict RP configuration rejected: %v", err)
	}
	if p.RPID() != "login.example.test" || p.Origin() != "https://login.example.test" {
		t.Fatal("RP ID or accepted origin was silently rewritten")
	}
	if (PasskeyRelyingParty{}).Configured() {
		t.Fatal("zero-value RP policy treated as configured")
	}
}

func TestPasskeyRelyingPartyRejectsUnsafeScope(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		rpID   string
		origin string
	}{
		{"empty RP ID", "", "https://login.example.test"},
		{"empty origin", "login.example.test", ""},
		{"bare hostname", "localhost", "https://localhost"},
		{"IPv4", "127.0.0.1", "https://127.0.0.1"},
		{"IPv6", "::1", "https://[::1]"},
		{"scheme downgrade", "login.example.test", "http://login.example.test"},
		{"origin sibling", "login.example.test", "https://other.example.test"},
		{"broader parent RP", "example.test", "https://login.example.test"},
		{"origin subdomain", "login.example.test", "https://deep.login.example.test"},
		{"explicit default port", "login.example.test", "https://login.example.test:443"},
		{"custom port", "login.example.test", "https://login.example.test:8443"},
		{"path", "login.example.test", "https://login.example.test/register"},
		{"trailing slash", "login.example.test", "https://login.example.test/"},
		{"query", "login.example.test", "https://login.example.test?mode=register"},
		{"fragment", "login.example.test", "https://login.example.test#continue"},
		{"userinfo", "login.example.test", "https://user@login.example.test"},
		{"wildcard origin", "login.example.test", "https://*.example.test"},
		{"wildcard RP", "*.example.test", "https://*.example.test"},
		{"malformed label", "login..test", "https://login..test"},
		{"leading hyphen", "-login.example.test", "https://-login.example.test"},
		{"trailing hyphen", "login-.example.test", "https://login-.example.test"},
		{"underscore", "login_web.example.test", "https://login_web.example.test"},
		{"encoded host", "login%2eexample.test", "https://login%2eexample.test"},
		{"mixed case ID", "Login.example.test", "https://Login.example.test"},
		{"mixed case origin", "login.example.test", "https://Login.example.test"},
		{"final dot", "login.example.test.", "https://login.example.test."},
		{"Unicode IDN", "bücher.example.test", "https://bücher.example.test"},
		{"numeric final label", "login.example.123", "https://login.example.123"},
		{"oversized label", strings.Repeat("a", 64) + ".example.test", "https://" + strings.Repeat("a", 64) + ".example.test"},
		{"oversized ID", strings.Repeat("a.", 127) + "example.test", "https://" + strings.Repeat("a.", 127) + "example.test"},
		{"leading whitespace", " login.example.test", "https:// login.example.test"},
		{"trailing whitespace", "login.example.test ", "https://login.example.test "},
		{"scheme relative", "login.example.test", "//login.example.test"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewPasskeyRelyingParty(tc.rpID, tc.origin)
			if !errors.Is(err, ErrPasskeyRPConfiguration) || got.Configured() {
				t.Fatalf("unsafe RP accepted: %q %q, err=%v", tc.rpID, tc.origin, err)
			}
		})
	}
}
