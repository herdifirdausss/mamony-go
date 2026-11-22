package handler

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

const browserRotatePath = "/v1/identity/change-password"
const browserChangePasswordBytes = 4096

// BrowserPasswordChangeHTTP is an explicitly constructed, UNMOUNTED first-party
// candidate. It shares the exact origin, TLS, trusted-peer and session-bound
// CSRF authority of BrowserSessionHTTP; no second independent browser secret
// or credential-verification path can be configured through this handler.
//
// On a certain commit, the v14 epoch invalidates all existing sessions.
// This handler clears the browser cookie but never issues a replacement.
type BrowserPasswordChangeHTTP struct {
	browser *BrowserSessionHTTP
	change  *usecase.AuthenticatedPasswordChange
}

func NewBrowserPasswordChangeHTTP(browser *BrowserSessionHTTP, change *usecase.AuthenticatedPasswordChange) (*BrowserPasswordChangeHTTP, error) {
	if browser == nil || browser.peer == nil || browser.login == nil ||
		browser.session == nil || browser.csrf == nil || change == nil {
		return nil, ErrBrowserSessionHTTPConfig
	}
	return &BrowserPasswordChangeHTTP{browser: browser, change: change}, nil
}

func (h *BrowserPasswordChangeHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.browser == nil || h.change == nil ||
		r == nil || r.URL == nil {
		writeBrowserSessionProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	if r.URL.Path != browserRotatePath {
		writeBrowserSessionProblem(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeBrowserSessionProblem(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	if !h.browser.trustedRequest(r, true) {
		writeBrowserSessionProblem(w, http.StatusForbidden, "REQUEST_ORIGIN_DENIED")
		return
	}
	secret, err := browserSessionBearer(r)
	if err != nil {
		writeBrowserSessionProblem(w, http.StatusUnauthorized, "INVALID_SESSION")
		return
	}
	csrf := r.Header.Values("X-Mamony-CSRF")
	if len(csrf) != 1 || !h.browser.csrf.verify(secret, csrf[0]) {
		writeBrowserSessionProblem(w, http.StatusForbidden, "CSRF_DENIED")
		return
	}
	types := r.Header.Values("Content-Type")
	if len(types) != 1 {
		writeBrowserSessionProblem(w, http.StatusUnsupportedMediaType, "JSON_REQUIRED")
		return
	}
	mediaType, params, err := mime.ParseMediaType(types[0])
	if err != nil || mediaType != "application/json" || len(params) > 1 ||
		(len(params) == 1 && !strings.EqualFold(params["charset"], "utf-8")) {
		writeBrowserSessionProblem(w, http.StatusUnsupportedMediaType, "JSON_REQUIRED")
		return
	}
	if r.Body == nil {
		writeBrowserSessionProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if r.ContentLength > browserChangePasswordBytes {
		writeBrowserSessionProblem(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, browserChangePasswordBytes)
	defer func() { _ = r.Body.Close() }()
	email, current, next, err := decodePasswordChangeJSON(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeBrowserSessionProblem(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
		} else {
			writeBrowserSessionProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		}
		return
	}
	canonical, err := domain.CanonicalEmail(email)
	if err != nil || canonical != email {
		writeBrowserSessionProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	peer, err := h.browser.peer.Resolve(r)
	if err != nil {
		writeBrowserSessionProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	err = h.change.Change(r.Context(), secret, email, current, next, peer)
	if err != nil {
		if errors.Is(err, usecase.ErrPasswordChangeDenied) {
			// Collapse invalid session, wrong old password, account mismatch,
			// exhausted admission and denied new-password policy.
			writeBrowserSessionProblem(w, http.StatusUnauthorized, "PASSWORD_CHANGE_DENIED")
			return
		}
		// A lost commit acknowledgement may mean the password did change.
		// Never claim success, retry or clear the cookie on ambiguity.
		writeBrowserSessionProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	if r.Context().Err() != nil {
		writeBrowserSessionProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	setBrowserSessionHeaders(w)
	http.SetCookie(w, sessionCookie("", -1))
	w.WriteHeader(http.StatusNoContent)
}

// decodePasswordChangeJSON refuses all duplicate or unknown keys, nested or
// non-string values, trailing documents and unbounded credential fields.
func decodePasswordChangeJSON(body io.Reader) (string, string, string, error) {
	dec := json.NewDecoder(body)
	first, err := dec.Token()
	if err != nil {
		return "", "", "", err
	}
	if first != json.Delim('{') {
		return "", "", "", ErrBrowserSessionHTTPConfig
	}
	seen := make(map[string]bool, 3)
	values := make(map[string]string, 3)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return "", "", "", err
		}
		key, ok := token.(string)
		if !ok || seen[key] || (key != "email" && key != "currentPassword" && key != "nextPassword") {
			return "", "", "", ErrBrowserSessionHTTPConfig
		}
		seen[key] = true
		var value string
		if err := dec.Decode(&value); err != nil {
			// Preserve *http.MaxBytesError when the client streams a body
			// without Content-Length; the HTTP boundary must respond 413.
			return "", "", "", err
		}
		if len(value) == 0 ||
			(key == "email" && len(value) > 254) ||
			(key != "email" && len(value) > 512) {
			return "", "", "", ErrBrowserSessionHTTPConfig
		}
		values[key] = value
	}
	end, err := dec.Token()
	if err != nil {
		return "", "", "", err
	}
	if end != json.Delim('}') || !seen["email"] ||
		!seen["currentPassword"] || !seen["nextPassword"] {
		return "", "", "", ErrBrowserSessionHTTPConfig
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return "", "", "", err
		}
		return "", "", "", ErrBrowserSessionHTTPConfig
	}
	return values["email"], values["currentPassword"], values["nextPassword"], nil
}

var _ http.Handler = (*BrowserPasswordChangeHTTP)(nil)
