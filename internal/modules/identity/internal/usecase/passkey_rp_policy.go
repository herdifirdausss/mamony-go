package usecase

import (
	"errors"
	"strings"
)

// ErrPasskeyRPConfiguration is intentionally nonspecific: no caller or
// untrusted input may choose an alternate relying-party origin at runtime.
var ErrPasskeyRPConfiguration = errors.New("passkey relying-party configuration denied")

// PasskeyRelyingParty pins one HTTPS origin to exactly one RP ID. It is only
// a configuration boundary for a FUTURE maintained WebAuthn verifier, not
// an authenticator enrollment, assertion, or authentication mechanism.
//
// The initial Mamony profile deliberately denies subdomain-origin,
// related-origin, localhost and port exceptions. A production operator
// must independently prove control of the configured DNS domain.
type PasskeyRelyingParty struct {
	id     string
	origin string
}

// NewPasskeyRelyingParty accepts only canonical lower-case ASCII DNS names
// with at least two labels and the single matching HTTPS origin. This avoids
// silently broadening RP scope to a parent domain, wildcard, URL path,
// userinfo, cross-origin iframe or custom port. The eventual WebAuthn library
// must ALSO verify clientDataJSON, challenge, type, RP ID hash, flags,
// origin and signed authenticator data; this value alone proves none of them.
func NewPasskeyRelyingParty(rpID, origin string) (PasskeyRelyingParty, error) {
	if !validPasskeyRPID(rpID) || origin != "https://"+rpID {
		return PasskeyRelyingParty{}, ErrPasskeyRPConfiguration
	}
	return PasskeyRelyingParty{id: rpID, origin: origin}, nil
}

func (r PasskeyRelyingParty) RPID() string   { return r.id }
func (r PasskeyRelyingParty) Origin() string { return r.origin }
func (r PasskeyRelyingParty) Configured() bool {
	return r.id != "" && r.origin == "https://"+r.id && validPasskeyRPID(r.id)
}

func validPasskeyRPID(id string) bool {
	if len(id) < 4 || len(id) > 253 || id != strings.ToLower(id) ||
		strings.HasSuffix(id, ".") || strings.HasPrefix(id, ".") {
		return false
	}
	labels := strings.Split(id, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 ||
			label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	// A wholly numeric final label is prohibited by this project policy.
	// DNS control/public-suffix ownership remains a separate deployment gate.
	last := labels[len(labels)-1]
	for i := 0; i < len(last); i++ {
		if last[i] < '0' || last[i] > '9' {
			return true
		}
	}
	return false
}
