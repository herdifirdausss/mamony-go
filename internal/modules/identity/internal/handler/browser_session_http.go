package handler

import (
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"strconv"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

const (
	browserLoginPath   = "/v1/identity/login"
	browserSessionPath = "/v1/identity/session"
	browserCSRFPath    = "/v1/identity/csrf"
	browserLogoutPath  = "/v1/identity/logout"
	browserCookieName  = "__Host-mamony_sid"
	browserLoginBytes  = 2048
	browserSessionMaxAge = 12 * 60 * 60
)

// BrowserSessionHTTP is a manually constructed, UNMOUNTED browser-only
// candidate. Its concrete SessionLogin dependency cannot bypass the durable
// pre-KDF admission/authentication path from ADR-0005/0006.
// CAUTION: This does not resolve the session DB role's direct-mint authority.
type BrowserSessionHTTP struct {
	peer    *ClientIPResolver
	login   *usecase.SessionLogin
	session *usecase.OpaqueSessionAuthority
	csrf    *sessionCSRF
	origin  string
	host    string
}

var ErrBrowserSessionHTTPConfig = errors.New("identity browser session HTTP unavailable")

func NewBrowserSessionHTTP(peer *ClientIPResolver, login *usecase.SessionLogin, session *usecase.OpaqueSessionAuthority, csrfKey []byte, origin string) (*BrowserSessionHTTP, error) {
	u, err := url.Parse(origin)
	if err != nil || u == nil || u.Scheme != "https" || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Path != "" || u.RawPath != "" || u.Opaque != "" ||
		strings.HasSuffix(u.Host, ".") || u.String() != origin ||
		peer == nil || len(peer.proxies) != 0 || login == nil || session == nil {
		return nil, ErrBrowserSessionHTTPConfig
	}
	csrf, err := newSessionCSRF(csrfKey, origin)
	if err != nil {
		return nil, ErrBrowserSessionHTTPConfig
	}
	return &BrowserSessionHTTP{peer: peer, login: login, session: session, csrf: csrf, origin: origin, host: u.Host}, nil
}

// ServeHTTP intentionally has no router registration, no environment-driven
// defaults and no authz for any financial capability. The HTTP edge's
// PublicHandler/PrivateHandler stay unchanged and do not reference this type.
func (h *BrowserSessionHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.peer == nil || h.login == nil || h.session == nil || h.csrf == nil ||
		r == nil || r.URL == nil {
		writeBrowserSessionProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	var method string
	switch r.URL.Path {
	case browserLoginPath, browserLogoutPath:
		method = http.MethodPost
	case browserSessionPath, browserCSRFPath:
		method = http.MethodGet
	default:
		writeBrowserSessionProblem(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		writeBrowserSessionProblem(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	if !h.trustedRequest(r, method == http.MethodPost) {
		writeBrowserSessionProblem(w, http.StatusForbidden, "REQUEST_ORIGIN_DENIED")
		return
	}
	switch r.URL.Path {
	case browserLoginPath:
		h.loginRequest(w, r)
	case browserSessionPath:
		h.sessionRequest(w, r)
	case browserCSRFPath:
		h.csrfRequest(w, r)
	case browserLogoutPath:
		h.logoutRequest(w, r)
	}
}

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

func (h *BrowserSessionHTTP) loginRequest(w http.ResponseWriter, r *http.Request) {
	// Reject ambient cookies, bearer headers and session fixation on login.
	if len(r.Header.Values("Cookie")) != 0 {
		writeBrowserSessionProblem(w, http.StatusForbidden, "REQUEST_ORIGIN_DENIED")
		return
	}
	values := r.Header.Values("Content-Type")
	if len(values) != 1 {
		writeBrowserSessionProblem(w, http.StatusUnsupportedMediaType, "JSON_REQUIRED")
		return
	}
	mt, params, err := mime.ParseMediaType(values[0])
	if err != nil || mt != "application/json" || len(params) > 1 ||
		(len(params) == 1 && !strings.EqualFold(params["charset"], "utf-8")) {
		writeBrowserSessionProblem(w, http.StatusUnsupportedMediaType, "JSON_REQUIRED")
		return
	}
	if r.Body == nil {
		writeBrowserSessionProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if r.ContentLength > browserLoginBytes {
		writeBrowserSessionProblem(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, browserLoginBytes)
	defer func() { _ = r.Body.Close() }()
	email, password, err := decodeEnrollmentJSON(r.Body, true)
	if err != nil {
		var overflow *http.MaxBytesError
		if errors.As(err, &overflow) {
			writeBrowserSessionProblem(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
		} else {
			writeBrowserSessionProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		}
		return
	}
	if _, err := domain.CanonicalEmail(email); err != nil {
		writeBrowserSessionProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	client, err := h.peer.Resolve(r)
	if err != nil {
		writeBrowserSessionProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	token, err := h.login.Authenticate(r.Context(), email, password, client)
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrPasswordAuthenticationDenied):
			writeBrowserSessionProblem(w, http.StatusUnauthorized, "INVALID_CREDENTIALS")
		case errors.Is(err, usecase.ErrPasswordLoginThrottled):
			writeBrowserSessionProblem(w, http.StatusTooManyRequests, "RATE_LIMITED")
		default:
			writeBrowserSessionProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		}
		return
	}
	secret := token.ForTrustedTransport()
	if !canonicalSessionBearer(secret) || r.Context().Err() != nil {
		// No bearer is ever exposed if issuance outcome is uncertain.
		writeBrowserSessionProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	setBrowserSessionHeaders(w)
	http.SetCookie(w, sessionCookie(secret, browserSessionMaxAge))
	w.WriteHeader(http.StatusNoContent)
}

func (h *BrowserSessionHTTP) sessionRequest(w http.ResponseWriter, r *http.Request) {
	if !browserReadBodyEmpty(r) {
		writeBrowserSessionProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	_, err := h.authenticatedCookie(r)
	if err != nil {
		h.sessionError(w, err)
		return
	}
	setBrowserSessionHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "{\"authenticated\":true}\n")
}

func (h *BrowserSessionHTTP) csrfRequest(w http.ResponseWriter, r *http.Request) {
	if !browserReadBodyEmpty(r) {
		writeBrowserSessionProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	secret, err := h.authenticatedCookie(r)
	if err != nil {
		h.sessionError(w, err)
		return
	}
	token := h.csrf.issue(secret)
	if token == "" {
		writeBrowserSessionProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	setBrowserSessionHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "{\"csrfToken\":\""+token+"\"}\n")
}

func (h *BrowserSessionHTTP) logoutRequest(w http.ResponseWriter, r *http.Request) {
	if !browserReadBodyEmpty(r) || len(r.Header.Values("Content-Type")) != 0 {
		writeBrowserSessionProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	secret, err := browserSessionBearer(r)
	if err != nil {
		writeBrowserSessionProblem(w, http.StatusUnauthorized, "INVALID_SESSION")
		return
	}
	csrfHeaders := r.Header.Values("X-Mamony-CSRF")
	if len(csrfHeaders) != 1 || !h.csrf.verify(secret, csrfHeaders[0]) {
		writeBrowserSessionProblem(w, http.StatusForbidden, "CSRF_DENIED")
		return
	}
	// CSRF validation before DB minimizes illegitimate lookup load.
	_, err = h.session.Resolve(r.Context(), secret)
	if err != nil {
		h.sessionError(w, err)
		return
	}
	if err := h.session.Revoke(r.Context(), secret); err != nil {
		writeBrowserSessionProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	setBrowserSessionHeaders(w)
	http.SetCookie(w, sessionCookie("", -1))
	w.WriteHeader(http.StatusNoContent)
}

// Invalid and revoked sessions never get subject data or distinct public
// responses; storage errors remain separately fail-closed as 503.
func (h *BrowserSessionHTTP) authenticatedCookie(r *http.Request) (string, error) {
	secret, err := browserSessionBearer(r)
	if err != nil {
		return "", usecase.ErrSessionInvalid
	}
	if _, err := h.session.Resolve(r.Context(), secret); err != nil {
		return "", err
	}
	return secret, nil
}

func (h *BrowserSessionHTTP) sessionError(w http.ResponseWriter, err error) {
	if errors.Is(err, usecase.ErrSessionInvalid) {
		writeBrowserSessionProblem(w, http.StatusUnauthorized, "INVALID_SESSION")
		return
	}
	writeBrowserSessionProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
}

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

// Static type assertion: this is a candidate handler, never registered.
var _ http.Handler = (*BrowserSessionHTTP)(nil)
