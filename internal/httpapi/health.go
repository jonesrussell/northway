package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// NewHandler exposes process health and fail-closed dependency readiness.
// A missing check is not readiness. Dependency errors never reach the caller.
func NewHandler(checkReady func(context.Context) error, collectionStatus func(context.Context) string) http.Handler {
	mux := http.NewServeMux()
	status := collectionStatus
	if status == nil {
		status = func(context.Context) string { return "disabled" }
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, "alive")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if checkReady == nil {
			respond(w, http.StatusServiceUnavailable, "not_ready")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 250*time.Millisecond)
		defer cancel()
		if err := checkReady(ctx); err != nil || ctx.Err() != nil {
			respond(w, http.StatusServiceUnavailable, "not_ready")
			return
		}
		respond(w, http.StatusOK, "ready")
	})
	mux.HandleFunc("GET /statusz", func(w http.ResponseWriter, r *http.Request) {
		value := status(r.Context())
		switch value {
		case "disabled", "idle", "polling", "degraded":
		default:
			value = "degraded"
		}
		respond(w, http.StatusOK, value)
	})
	return mux
}

func respond(w http.ResponseWriter, status int, value string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	// Responses are fixed small values; the server owns connection write errors.
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
	}{value})
}
