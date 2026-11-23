package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/domain"
	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

const (
	registerPath      = "/v1/identity/register"
	resendPath        = "/v1/identity/resend-verification"
	maxEnrollmentBody = 2048
)

var ErrEnrollmentHandlerConfig = errors.New("identity HTTP enrollment handler unavailable")

// EnrollmentWorkflow is an internal usecase, never an authorization authority.
type EnrollmentWorkflow interface {
	Register(context.Context, string, string) error
	Resend(context.Context, string) error
}

// EnrollmentHTTP is NOT mounted by the running public or private listener.
// TLS is required at this boundary. Offloading TLS to a proxy must be
// explicitly reviewed; X-Forwarded-Proto is never trusted as evidence.
type EnrollmentHTTP struct {
	admission *AnonymousBoundary
	workflow  EnrollmentWorkflow
	origin    string
	host      string
}

func NewEnrollmentHTTP(admission *AnonymousBoundary, workflow EnrollmentWorkflow, origin string) (*EnrollmentHTTP, error) {
	u, err := url.Parse(origin)
	if err != nil || u == nil || u.Scheme != "https" || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Path != "" || u.RawPath != "" || u.Opaque != "" ||
		strings.HasSuffix(u.Host, ".") || u.String() != origin ||
		admission == nil || workflow == nil {
		return nil, ErrEnrollmentHandlerConfig
	}
	return &EnrollmentHTTP{admission: admission, workflow: workflow, origin: origin, host: u.Host}, nil
}

func (h *EnrollmentHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.admission == nil || h.workflow == nil || r == nil || r.URL == nil {
		writeEnrollmentProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	var action usecase.AnonymousAction
	switch r.URL.Path {
	case registerPath:
		action = usecase.AnonymousRegister
	case resendPath:
		action = usecase.AnonymousResend
	default:
		writeEnrollmentProblem(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeEnrollmentProblem(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		return
	}
	if r.TLS == nil || r.Host != h.host || r.URL.RawQuery != "" ||
		r.URL.Fragment != "" || len(r.Header.Values("Origin")) != 1 ||
		r.Header.Get("Origin") != h.origin ||
		len(r.Header.Values("Cookie")) != 0 ||
		len(r.Header.Values("Authorization")) != 0 ||
		(len(r.Header.Values("Sec-Fetch-Site")) != 0 &&
			(len(r.Header.Values("Sec-Fetch-Site")) != 1 ||
				r.Header.Get("Sec-Fetch-Site") != "same-origin")) {
		writeEnrollmentProblem(w, http.StatusForbidden, "REQUEST_ORIGIN_DENIED")
		return
	}
	types := r.Header.Values("Content-Type")
	if len(types) != 1 {
		writeEnrollmentProblem(w, http.StatusUnsupportedMediaType, "JSON_REQUIRED")
		return
	}
	mediaType, params, err := mime.ParseMediaType(types[0])
	if err != nil || mediaType != "application/json" || len(params) > 1 ||
		(len(params) == 1 && !strings.EqualFold(params["charset"], "utf-8")) {
		writeEnrollmentProblem(w, http.StatusUnsupportedMediaType, "JSON_REQUIRED")
		return
	}
	if r.Body == nil {
		writeEnrollmentProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if r.ContentLength > maxEnrollmentBody {
		writeEnrollmentProblem(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxEnrollmentBody)
	defer func() { _ = r.Body.Close() }()
	email, password, err := decodeEnrollmentJSON(r.Body, action == usecase.AnonymousRegister)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeEnrollmentProblem(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
		} else {
			writeEnrollmentProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		}
		return
	}
	if _, err := domain.CanonicalEmail(email); err != nil {
		writeEnrollmentProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	allowed, err := h.admission.Admit(r, action, email)
	if err != nil {
		writeEnrollmentProblem(w, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	if allowed {
		if action == usecase.AnonymousRegister {
			err = h.workflow.Register(r.Context(), email, password)
			// Input validity does not reveal account existence. Passwords
			// rejected by policy have the same 400 for existing/new aliases.
			if errors.Is(err, domain.ErrInvalidPassword) || errors.Is(err, domain.ErrInvalidEmail) {
				writeEnrollmentProblem(w, http.StatusBadRequest, "INVALID_REQUEST")
				return
			}
		} else {
			err = h.workflow.Resend(r.Context(), email)
		}
		// Runtime outcomes collapse into the same receipt for missing,
		// duplicate, blocked, throttled and unavailable accounts. Real
		// rollout needs privacy-safe outage alerting and reconciliation.
		_ = err
	}
	WriteOpaqueAdmissionReceipt(w)
}

// decodeEnrollmentJSON rejects duplicate keys, unknown keys, wrong value
// types, nested JSON and multiple top-level documents.
func decodeEnrollmentJSON(body io.Reader, registration bool) (string, string, error) {
	dec := json.NewDecoder(body)
	first, err := dec.Token()
	if err != nil || first != json.Delim('{') {
		return "", "", ErrEnrollmentHandlerConfig
	}
	seen := make(map[string]bool, 2)
	var email, password string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return "", "", ErrEnrollmentHandlerConfig
		}
		name, ok := tok.(string)
		if !ok || seen[name] || (name != "email" && (!registration || name != "password")) {
			return "", "", ErrEnrollmentHandlerConfig
		}
		seen[name] = true
		var value string
		if err := dec.Decode(&value); err != nil {
			return "", "", ErrEnrollmentHandlerConfig
		}
		switch name {
		case "email":
			if len(value) == 0 || len(value) > 254 {
				return "", "", ErrEnrollmentHandlerConfig
			}
			email = value
		case "password":
			if len(value) == 0 || len(value) > 512 {
				return "", "", ErrEnrollmentHandlerConfig
			}
			password = value
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim('}') || !seen["email"] ||
		(registration && !seen["password"]) {
		return "", "", ErrEnrollmentHandlerConfig
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return "", "", ErrEnrollmentHandlerConfig
	}
	return email, password, nil
}

func writeEnrollmentProblem(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"type":"about:blank","title":"Request rejected","status":` +
		strconv.Itoa(status) + `,"code":"` + code + `"}` + "\n"))
}
