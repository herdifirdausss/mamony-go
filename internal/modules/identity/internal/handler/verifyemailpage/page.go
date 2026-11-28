package verifyemailpage

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/handler/internal/browserpage"
)

// Page assets are embedded at build time, never fetched from a CDN.
// The only script and stylesheet are pinned to their own response CSP hashes.
//
//go:embed page.js page.css
var verificationPageAssets embed.FS

const verificationPagePath = "/verify-email"

var ErrConfig = errors.New("identity verification page unavailable")

// Handler renders an unmounted, static, first-party fragment
// consumer. It has NO verifier, registration or database authority and does
// not accept a bearer in an HTTP request URL, cookie, or request body.
// An email client navigation must never consume a proof automatically.
type Handler struct {
	host string
}

// New requires exactly the approved direct-TLS origin
// used by the candidate confirmation API and Mailgun's verification link.
// Reverse-proxy TLS offload remains a separate, unresolved deployment policy.
func New(origin string) (*Handler, error) {
	u, err := url.Parse(origin)
	if err != nil || u == nil || u.Scheme != "https" || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Path != "" || u.RawPath != "" || u.Opaque != "" ||
		u.Host != u.Hostname() || strings.HasSuffix(u.Host, ".") ||
		u.String() != origin {
		return nil, ErrConfig
	}
	return &Handler{host: u.Host}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	browserpage.SetSecurityHeaders(w)
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
		r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Fragment != "" ||
		len(r.Header.Values("Cookie")) != 0 ||
		len(r.Header.Values("Authorization")) != 0 ||
		len(r.Header.Values("Forwarded")) != 0 ||
		len(r.Header.Values("X-Forwarded-Proto")) != 0 {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	script, err := verificationPageAssets.ReadFile("page.js")
	if err != nil || len(script) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	css, err := verificationPageAssets.ReadFile("page.css")
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
