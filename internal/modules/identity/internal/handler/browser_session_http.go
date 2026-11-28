package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

const (
	browserLoginPath     = "/v1/identity/login"
	browserSessionPath   = "/v1/identity/session"
	browserCSRFPath      = "/v1/identity/csrf"
	browserLogoutPath    = "/v1/identity/logout"
	browserCookieName    = "__Host-mamony_sid"
	browserLoginBytes    = 2048
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

// Static type assertion: this is a candidate handler, never registered.
var _ http.Handler = (*BrowserSessionHTTP)(nil)
