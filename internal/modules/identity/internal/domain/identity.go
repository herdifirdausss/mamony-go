// Package domain owns early identity invariants. Nothing here confers a session or financial access.
package domain

import (
	"errors"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var (
	ErrInvalidEmail    = errors.New("invalid enrollment email")
	ErrInvalidPassword = errors.New("password does not satisfy enrollment policy")
)

// CanonicalEmail deliberately supports only basic ASCII mailbox syntax in
// the first synthetic browser profile. EAI/IDN and provider-specific alias
// transformations require a separate reviewed policy. Do not trim user input.
func CanonicalEmail(input string) (string, error) {
	if len(input) == 0 || len(input) > 254 || !utf8.ValidString(input) || strings.TrimSpace(input) != input {
		return "", ErrInvalidEmail
	}
	if strings.IndexFunc(input, func(r rune) bool { return r <= 32 || r >= 127 }) >= 0 {
		return "", ErrInvalidEmail
	}
	at := strings.LastIndexByte(input, '@')
	if at <= 0 || at != strings.IndexByte(input, '@') || at >= len(input)-1 {
		return "", ErrInvalidEmail
	}
	local, host := input[:at], input[at+1:]
	if len(local) > 64 || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") || strings.ContainsAny(local, "()<>[]:;\\\" ,") {
		return "", ErrInvalidEmail
	}
	if !strings.Contains(host, ".") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return "", ErrInvalidEmail
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrInvalidEmail
		}
		for _, c := range label {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
				return "", ErrInvalidEmail
			}
		}
	}
	for _, c := range local {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("!#$%&'*+-/=?^_`{|}~.", c)) {
			return "", ErrInvalidEmail
		}
	}
	// This is a declared application alias policy, not a claim that every
	// email provider treats local-part case as insignificant.
	return strings.ToLower(input), nil
}

// PasswordPolicy has no open/default-allow blocklist. Production enrollment
// MUST provide a reviewed common/breached/contextual rejector before enabling
// routes. The first slice intentionally has no public route.
type PasswordPolicy struct {
	IsBlocked func(normalizedPassword, canonicalEmail string) bool
}

func (p PasswordPolicy) NormalizeAndValidate(input, canonicalEmail string) (string, error) {
	if p.IsBlocked == nil || !utf8.ValidString(input) {
		return "", ErrInvalidPassword
	}
	normalized := norm.NFC.String(input)
	n := utf8.RuneCountInString(normalized)
	if n < 15 || n > 128 || len(normalized) > 512 || p.IsBlocked(normalized, canonicalEmail) {
		return "", ErrInvalidPassword
	}
	return normalized, nil
}

// NormalizeForVerification shares enrollment semantics without imposing a
// new password policy on previously issued credentials. KDF verifier versions
// MUST preserve normalization compatibility across future migrations.
func NormalizeForVerification(input string) (string, error) {
	if !utf8.ValidString(input) || len(input) > 2048 {
		return "", ErrInvalidPassword
	}
	normalized := norm.NFC.String(input)
	if len(normalized) > 512 {
		return "", ErrInvalidPassword
	}
	return normalized, nil
}

// PasswordVerifier is persistence data; it does not authorize login or
// external access on its own. Stored KDF parameters must be allowlisted before use.
type PasswordVerifier struct {
	Version     uint16
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	Salt        []byte
	Digest      []byte
}
