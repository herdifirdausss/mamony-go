package handler

import (
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

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
	email, password, err := decodeEmailPasswordRequestJSON(r.Body, true)
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
