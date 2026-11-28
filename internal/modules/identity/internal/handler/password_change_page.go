package handler

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/handler/internal/browserpage"
)

//go:embed password_change_page.js password_change_page.css
var passwordChangePageAssets embed.FS

const passwordChangePagePath = "/account/change-password"

var ErrPasswordChangePageConfig = errors.New("identity password-change page unavailable")

// PasswordChangePage is an explicitly constructed, UNMOUNTED candidate.
// It shares the existing browser session's exact direct-TLS origin boundary.
// Rendering this page does not itself authenticate, mint a bearer, reissue a
// session, read a password, or perform an account mutation.
type PasswordChangePage struct {
	browser *BrowserSessionHTTP
}

func NewPasswordChangePage(browser *BrowserSessionHTTP) (*PasswordChangePage, error) {
	if browser == nil || browser.peer == nil || browser.login == nil ||
		browser.session == nil || browser.csrf == nil || browser.origin == "" {
		return nil, ErrPasswordChangePageConfig
	}
	return &PasswordChangePage{browser: browser}, nil
}

func (h *PasswordChangePage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// No cache, referrer, frames, third-party fetch or persisted state.
	browserpage.SetSecurityHeaders(w)
	if h == nil || h.browser == nil || r == nil || r.URL == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if r.URL.Path != passwordChangePagePath {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.ForceQuery || !h.browser.trustedRequest(r, false) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	script, err := passwordChangePageAssets.ReadFile("password_change_page.js")
	if err != nil || len(script) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	style, err := passwordChangePageAssets.ReadFile("password_change_page.css")
	if err != nil || len(style) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	scriptHash := sha256.Sum256(script)
	styleHash := sha256.Sum256(style)
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; base-uri 'none'; object-src 'none'; "+
			"script-src 'sha256-"+base64.StdEncoding.EncodeToString(scriptHash[:])+"'; "+
			"style-src 'sha256-"+base64.StdEncoding.EncodeToString(styleHash[:])+"'; "+
			"connect-src 'self'; img-src 'none'; font-src 'none'; "+
			"form-action 'none'; frame-ancestors 'none'; worker-src 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\">" +
		"<meta name=\"referrer\" content=\"no-referrer\">" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
		"<title>Change password | Mamony</title><script>"))
	_, _ = w.Write(script)
	_, _ = w.Write([]byte("</script><style>"))
	_, _ = w.Write(style)
	_, _ = w.Write([]byte("</style></head><body><main class=\"panel\" aria-labelledby=\"heading\">" +
		"<p class=\"eyebrow\">Mamony · Account security</p>" +
		"<h1 id=\"heading\">Change password</h1>" +
		"<p id=\"status\" role=\"status\" aria-live=\"polite\">Checking your session securely…</p>" +
		"<form id=\"change-form\" hidden autocomplete=\"off\">" +
		"<label for=\"email\">Account email</label>" +
		"<input id=\"email\" name=\"username\" type=\"email\" autocomplete=\"username\" maxlength=\"254\" required>" +
		"<label for=\"current\">Current password</label>" +
		"<input id=\"current\" type=\"password\" autocomplete=\"current-password\" maxlength=\"512\" required>" +
		"<label for=\"replacement\">New password</label>" +
		"<input id=\"replacement\" type=\"password\" autocomplete=\"new-password\" maxlength=\"512\" required>" +
		"<label for=\"confirmation\">Confirm new password</label>" +
		"<input id=\"confirmation\" type=\"password\" autocomplete=\"new-password\" maxlength=\"512\" required>" +
		"<button id=\"change\" type=\"submit\" disabled>Change password</button></form>" +
		"<p class=\"footnote\">Changing your password signs out all sessions. " +
		"If a network error occurs, the password may already have changed. " +
		"This page will never retry a change automatically.</p></main></body></html>"))
}

var _ http.Handler = (*PasswordChangePage)(nil)
