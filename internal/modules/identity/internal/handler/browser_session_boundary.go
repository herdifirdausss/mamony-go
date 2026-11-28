package handler

import (
	"encoding/base64"
	"io"
	"net/http"
	"strconv"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

// Origin is compulsory for both mutating POSTs. A read-only GET may omit the
// Origin header as real browser navigations commonly do; if supplied it must
// match. Forwarding headers do NOT establish TLS, peer or browser identity.
func (h *BrowserSessionHTTP) trustedRequest(r *http.Request, mutation bool) bool {
	if r.TLS == nil || r.Host != h.host || r.URL.RawQuery != "" ||
		r.URL.Fragment != "" || r.URL.RawPath != "" ||
		len(r.Header.Values("Authorization")) != 0 ||
		len(r.Header.Values("Forwarded")) != 0 ||
		len(r.Header.Values("X-Forwarded-For")) != 0 ||
		len(r.Header.Values("X-Forwarded-Proto")) != 0 ||
		len(r.Header.Values("X-Forwarded-Host")) != 0 ||
		len(r.Header.Values("X-Real-Ip")) != 0 ||
		len(r.Header.Values("True-Client-Ip")) != 0 {
		return false
	}
	origins := r.Header.Values("Origin")
	if mutation {
		if len(origins) != 1 || origins[0] != h.origin {
			return false
		}
	} else if len(origins) > 1 || (len(origins) == 1 && origins[0] != h.origin) {
		return false
	}
	if fetch := r.Header.Values("Sec-Fetch-Site"); len(fetch) > 1 ||
		(len(fetch) == 1 && fetch[0] != "same-origin" && !(methodIsGet(r.Method) && fetch[0] == "none")) {
		return false
	}
	return true
}

func methodIsGet(method string) bool { return method == http.MethodGet }

func browserSessionBearer(r *http.Request) (string, error) {
	if r == nil || len(r.Header.Values("Cookie")) != 1 {
		return "", usecase.ErrSessionInvalid
	}
	var secret string
	found := 0
	for _, c := range r.Cookies() {
		if c.Name == browserCookieName {
			found++
			secret = c.Value
		}
	}
	if found != 1 || !canonicalSessionBearer(secret) {
		return "", usecase.ErrSessionInvalid
	}
	return secret, nil
}

func canonicalSessionBearer(secret string) bool {
	if len(secret) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(secret)
	return err == nil && len(decoded) == 32 &&
		base64.RawURLEncoding.EncodeToString(decoded) == secret
}

func browserReadBodyEmpty(r *http.Request) bool {
	if r == nil || r.ContentLength > 0 || len(r.TransferEncoding) != 0 {
		return false
	}
	if r.Body == nil || r.Body == http.NoBody {
		return true
	}
	defer func() { _ = r.Body.Close() }()
	var one [1]byte
	n, err := r.Body.Read(one[:])
	return n == 0 && err == io.EOF
}

func sessionCookie(value string, age int) *http.Cookie {
	return &http.Cookie{
		Name: browserCookieName, Value: value,
		Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode,
		MaxAge: age,
	}
}

func setBrowserSessionHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
}

func writeBrowserSessionProblem(w http.ResponseWriter, status int, code string) {
	setBrowserSessionHeaders(w)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, "{\"type\":\"about:blank\",\"title\":\"Request rejected\",\"status\":"+
		httpStatusDigits(status)+",\"code\":\""+code+"\"}\n")
}

// Keep error serialization identical to other Identity candidate handlers.
func httpStatusDigits(status int) string { return strconv.Itoa(status) }
