package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

const (
	confirmEmailPath = "/v1/identity/verify-email"
	maxConfirmBody   = 256
)

var ErrConfirmationHandlerConfig = errors.New("identity email confirmation handler unavailable")

// EmailConfirmer owns single-use, purpose/contact-bound token consumption.
// It must be injected by the Identity facade, never from a business router.
type EmailConfirmer interface {
	Confirm(context.Context, string) error
}

// ConfirmationAdmission is deliberately separate from REGISTER/RESEND
// admission: a 256-bit bearer has no email or client-chosen subject ID.
// A production implementation must enforce per-peer, distributed budgets
// without storing raw IP or bearer. This PR does NOT supply that live gate.
type ConfirmationAdmission interface {
	Admit(context.Context, netip.Addr) (bool, error)
}

// EmailConfirmationHTTP is a manually constructed candidate, never mounted
// by public/private routers. Direct Go TLS and exact browser origin are
// required; trusted reverse-proxy termination is a separate release decision.
type EmailConfirmationHTTP struct {
	peer      *ClientIPResolver
	admission ConfirmationAdmission
	confirmer EmailConfirmer
	origin    string
	host      string
}

// NewEmailConfirmationHTTP has no default admission implementation or
// permissive constructor. Tests inject a deterministic fake gate; release
// requires a reviewed durable rate limiter and a trusted ingress policy.
func NewEmailConfirmationHTTP(peer *ClientIPResolver, admission ConfirmationAdmission, confirmer EmailConfirmer, origin string) (*EmailConfirmationHTTP, error) {
	u, err := url.Parse(origin)
	if err != nil || u == nil || u.Scheme != "https" || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Path != "" || u.RawPath != "" || u.Opaque != "" ||
		strings.HasSuffix(u.Host, ".") || u.String() != origin ||
		peer == nil || admission == nil || confirmer == nil {
		return nil, ErrConfirmationHandlerConfig
	}
	return &EmailConfirmationHTTP{
		peer: peer, admission: admission, confirmer: confirmer,
		origin: origin, host: u.Host,
	}, nil
}

func (h *EmailConfirmationHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.peer == nil || h.admission == nil || h.confirmer == nil ||
		r == nil || r.URL == nil {
		writeConfirmationProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	if r.URL.Path != confirmEmailPath {
		writeConfirmationProblem(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeConfirmationProblem(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	// There must be no token in query/URL, cookie, Authorization, forwarded
	// transport declaration or cross-site request. The fragment token from
	// the Mailgun link is visible only to first-party browser JS.
	if r.TLS == nil || r.Host != h.host || r.URL.RawQuery != "" ||
		r.URL.Fragment != "" || len(r.Header.Values("Origin")) != 1 ||
		r.Header.Get("Origin") != h.origin ||
		len(r.Header.Values("Cookie")) != 0 ||
		len(r.Header.Values("Authorization")) != 0 ||
		len(r.Header.Values("X-Forwarded-Proto")) != 0 ||
		len(r.Header.Values("Forwarded")) != 0 ||
		(len(r.Header.Values("Sec-Fetch-Site")) != 0 &&
			(len(r.Header.Values("Sec-Fetch-Site")) != 1 ||
				r.Header.Get("Sec-Fetch-Site") != "same-origin")) {
		writeConfirmationProblem(w, http.StatusForbidden, "REQUEST_ORIGIN_DENIED")
		return
	}
	types := r.Header.Values("Content-Type")
	if len(types) != 1 {
		writeConfirmationProblem(w, http.StatusUnsupportedMediaType, "JSON_REQUIRED")
		return
	}
	mediaType, params, err := mime.ParseMediaType(types[0])
	if err != nil || mediaType != "application/json" || len(params) > 1 ||
		(len(params) == 1 && !strings.EqualFold(params["charset"], "utf-8")) {
		writeConfirmationProblem(w, http.StatusUnsupportedMediaType, "JSON_REQUIRED")
		return
	}
	if r.Body == nil {
		writeConfirmationProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if r.ContentLength > maxConfirmBody {
		writeConfirmationProblem(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxConfirmBody)
	defer func() { _ = r.Body.Close() }()
	token, err := decodeConfirmationJSON(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeConfirmationProblem(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
		} else {
			writeConfirmationProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		}
		return
	}
	peer, err := h.peer.Resolve(r)
	if err != nil {
		writeConfirmationProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	allowed, err := h.admission.Admit(r.Context(), peer)
	if err != nil {
		writeConfirmationProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	if !allowed {
		writeConfirmationProblem(w, http.StatusTooManyRequests, "RATE_LIMITED")
		return
	}
	// Confirm rechecks purpose, token digest, current contact version,
	// status, revocation and expiry in the owning verifier transaction.
	// It never issues a login session. All failed proofs have one public
	// outcome, regardless of whether invalid, expired or already consumed.
	if err := h.confirmer.Confirm(r.Context(), token); err != nil {
		writeConfirmationProblem(w, http.StatusBadRequest, "INVALID_OR_EXPIRED_PROOF")
		return
	}
	setConfirmationHeaders(w)
	w.WriteHeader(http.StatusNoContent)
}

// decodeConfirmationJSON rejects unknown/duplicate/missing keys, multiple
// documents, nested values, and noncanonical 32-byte random bearer strings.
func decodeConfirmationJSON(body io.Reader) (string, error) {
	dec := json.NewDecoder(body)
	first, err := dec.Token()
	if err != nil || first != json.Delim('{') {
		return "", ErrConfirmationHandlerConfig
	}
	var token string
	seen := false
	for dec.More() {
		field, err := dec.Token()
		if err != nil || field != "token" || seen {
			return "", ErrConfirmationHandlerConfig
		}
		seen = true
		if err := dec.Decode(&token); err != nil {
			return "", ErrConfirmationHandlerConfig
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim('}') || !seen {
		return "", ErrConfirmationHandlerConfig
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return "", ErrConfirmationHandlerConfig
	}
	if len(token) != 43 {
		return "", ErrConfirmationHandlerConfig
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 ||
		base64.RawURLEncoding.EncodeToString(decoded) != token {
		return "", ErrConfirmationHandlerConfig
	}
	return token, nil
}

func setConfirmationHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
}

func writeConfirmationProblem(w http.ResponseWriter, status int, code string) {
	setConfirmationHeaders(w)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"type":"about:blank","title":"Request rejected","status":` +
		strconv.Itoa(status) + `,"code":"` + code + `"}` + "\n"))
}
