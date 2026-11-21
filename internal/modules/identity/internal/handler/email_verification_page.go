package handler

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Page assets are embedded at build time, never fetched from a CDN.
// The only script and stylesheet are pinned to their own response CSP hashes.
//go:embed email_verification_page.js email_verification_page.css
var verificationPageAssets embed.FS

const verificationPagePath = "/verify-email"

var ErrVerificationPageConfig = errors.New("identity verification page unavailable")

// EmailVerificationPage renders an unmounted, static, first-party fragment
// consumer. It has NO verifier, registration or database authority and does
// not accept a bearer in an HTTP request URL, cookie, or request body.
// An email client navigation must never consume a proof automatically.
type EmailVerificationPage struct {
	host string
}

// NewEmailVerificationPage requires exactly the approved direct-TLS origin
// used by the candidate confirmation API and Mailgun's verification link.
// Reverse-proxy TLS offload remains a separate, unresolved deployment policy.
func NewEmailVerificationPage(origin string) (*EmailVerificationPage, error) {
	u, err := url.Parse(origin)
	if err != nil || u == nil || u.Scheme != "https" || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Path != "" || u.RawPath != "" || u.Opaque != "" ||
		strings.HasSuffix(u.Host, ".") || u.String() != origin {
		return nil, ErrVerificationPageConfig
	}
	return &EmailVerificationPage{host: u.Host}, nil
}

func (h *EmailVerificationPage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setVerificationPageHeaders(w)
	if h == nil || r == nil || r.URL == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if r.URL.Path != verificationPagePath {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// GET navigation from an email app typically carries no Origin; POST
	// confirmation, not the landing page, requires matching Origin.
	// Never accept credentials, request query/fragment or forwarded schemes.
	if r.TLS == nil || r.Host != h.host ||
		r.URL.RawQuery != "" || r.URL.Fragment != "" ||
		len(r.Header.Values("Cookie")) != 0 ||
		len(r.Header.Values("Authorization")) != 0 ||
		len(r.Header.Values("Forwarded")) != 0 ||
		len(r.Header.Values("X-Forwarded-Proto")) != 0 {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	script, err := verificationPageAssets.ReadFile("email_verification_page.js")
	if err != nil || len(script) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	css, err := verificationPageAssets.ReadFile("email_verification_page.css")
	if err != nil || len(css) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	// CSP hashes bind exactly the embedded executable bytes. The page has
	// no third-party code, images, fonts, worker or telemetry requests.
	scriptHash := sha256.Sum256(script)
	cssHash := sha256.Sum256(css)
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; base-uri 'none'; object-src 'none'; "+
			"script-src 'sha256-"+base64.StdEncoding.EncodeToString(scriptHash[:])+"'; "+
			"style-src 'sha256-"+base64.StdEncoding.EncodeToString(cssHash[:])+"'; "+
			"connect-src 'self'; img-src 'none'; font-src 'none'; "+
			"form-action 'none'; frame-ancestors 'none'; worker-src 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	// The bootstrap script is deliberately the FIRST resource/executable
	// element in HEAD, before title, style, or any other page content.
	// Its first operation captures and removes the URL fragment using
	// history.replaceState; it cannot silently POST proof on page load.
	_, _ = w.Write([]byte("<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\">" +
		"<meta name=\"referrer\" content=\"no-referrer\"><script>"))
	_, _ = w.Write(script)
	_, _ = w.Write([]byte("</script><style>"))
	_, _ = w.Write(css)
	_, _ = w.Write([]byte("</style><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
		"<title>Verify email | Mamony</title></head><body><main class=\"panel\" aria-labelledby=\"heading\">" +
		"<p class=\"eyebrow\">Mamony · Account security</p>" +
		"<h1 id=\"heading\">Verify your email</h1>" +
		"<p id=\"status\" role=\"status\" aria-live=\"polite\">Checking verification link…</p>" +
		"<button id=\"confirm\" type=\"button\" hidden>Verify my email</button>" +
		"<p class=\"footnote\">We never ask for your password on this page. " +
		"Verification does not sign you in.</p></main></body></html>"))
}

func setVerificationPageHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
}
