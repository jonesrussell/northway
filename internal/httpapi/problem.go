package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jonesrussell/northway/internal/feedback"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"net/http"
)

func requestID(w http.ResponseWriter) string {
	if id := w.Header().Get("X-Request-ID"); id != "" {
		return id
	}
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	id := fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
	w.Header().Set("X-Request-ID", id)
	return id
}
func problem(w http.ResponseWriter, status int, code, message string, retryable bool) {
	id := requestID(w)
	w.Header().Set("Content-Type", "application/json")
	if retryable {
		w.Header().Set("Retry-After", "1")
	} else {
		w.Header().Del("Retry-After")
	}
	if status == 401 {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
		Retryable bool   `json:"retryable"`
	}{code, message, id, retryable})
}
func serviceProblem(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrUnauthorized):
		problem(w, 401, "unauthorized", "Valid bearer credentials required", false)
	case errors.Is(err, identity.ErrForbidden):
		problem(w, 403, "forbidden", "Required scope is missing", false)
	case errors.Is(err, query.ErrUnavailable):
		problem(w, 503, "unavailable", "Query unavailable; do not automatically replace its key", false)
	case errors.Is(err, identity.ErrNotFound):
		problem(w, 404, "not_found", "Object unavailable", false)
	case errors.Is(err, query.ErrInvalid), errors.Is(err, feedback.ErrInvalid):
		problem(w, 400, "invalid_request", "Invalid request", false)
	case errors.Is(err, query.ErrInProgress):
		problem(w, 409, "in_progress", "Query still in progress; retry with the same key", true)
	case errors.Is(err, query.ErrConflict), errors.Is(err, feedback.ErrConflict):
		problem(w, 409, "conflict", "Request conflicts with existing state", false)
	default:
		problem(w, 503, "unavailable", "Service temporarily unavailable", true)
	}
}
