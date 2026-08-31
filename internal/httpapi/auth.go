package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
)

type Authenticator interface {
	Authenticate(context.Context, string) (identity.Principal, error)
}

// Require passes identity explicitly, never by a tenant field or context string
// key. It is a reusable boundary; product endpoints are added in their slices.
// TLS terminates at the trusted private reverse proxy, not at this wrapper.
func Require(auth Authenticator, scope identity.Scopes, next func(http.ResponseWriter, *http.Request, identity.Principal)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		values := r.Header.Values("Authorization")
		if len(values) != 1 {
			authProblem(w, http.StatusUnauthorized)
			return
		}
		scheme, raw, ok := strings.Cut(values[0], " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || len(raw) != 80 {
			authProblem(w, http.StatusUnauthorized)
			return
		}
		if auth == nil || next == nil || !scope.Valid() {
			authProblem(w, http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		principal, err := auth.Authenticate(ctx, raw)
		if ctx.Err() != nil {
			err = identity.ErrUnavailable
		}
		cancel()
		if err != nil {
			if errors.Is(err, identity.ErrUnauthorized) {
				authProblem(w, http.StatusUnauthorized)
			} else {
				authProblem(w, http.StatusServiceUnavailable)
			}
			return
		}
		if _, err := principal.Require(scope); err != nil {
			if errors.Is(err, identity.ErrUnauthorized) {
				authProblem(w, http.StatusUnauthorized)
			} else {
				authProblem(w, http.StatusForbidden)
			}
			return
		}
		next(w, r, principal)
	})
}

func authProblem(w http.ResponseWriter, status int) {
	code, message := "unavailable", "Service temporarily unavailable"
	switch status {
	case http.StatusUnauthorized:
		code, message = "unauthorized", "Valid bearer credentials required"
		w.Header().Set("WWW-Authenticate", "Bearer")
	case http.StatusForbidden:
		code, message = "forbidden", "Required scope is missing"
	default:
		w.Header().Set("Retry-After", "1")
	}
	var id [16]byte
	rand.Read(id[:])
	id[6] = (id[6] & 15) | 64
	id[8] = (id[8] & 63) | 128
	h := hex.EncodeToString(id[:])
	requestID := h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
		Retryable bool   `json:"retryable"`
	}{code, message, requestID, status == http.StatusServiceUnavailable})
}
