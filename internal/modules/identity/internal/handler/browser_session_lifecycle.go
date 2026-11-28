package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

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
