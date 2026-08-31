package httpapi

import (
	"context"
	"encoding/json"
	"github.com/jonesrussell/northway/internal/feedback"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"net/http"
	"path"
	"time"
)

type Queries interface {
	Query(context.Context, identity.Principal, string, query.Request) (query.Snapshot, error)
	Get(context.Context, identity.Principal, string) (query.Snapshot, error)
}
type Feedback interface {
	Submit(context.Context, identity.Principal, feedback.Event) error
}

// NewAPI exposes only authenticated product routes. Health remains a separate
// private operational boundary. No source provisioning, collection or paid work.
func NewAPI(auth Authenticator, queries Queries, events Feedback) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /v1/feed-queries", Require(auth, identity.FeedsRead, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		keys := r.Header.Values("Idempotency-Key")
		if len(keys) != 1 || !query.ValidKey(keys[0]) {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		m, err := readObject(w, r)
		if err != nil || !queryShape(m) {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		var input query.Request
		if decodeObject(m, &input) != nil {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		if _, err = input.Digest(); err != nil {
			serviceProblem(w, err)
			return
		}
		if queries == nil {
			serviceProblem(w, identity.ErrUnavailable)
			return
		}
		snap, err := queries.Query(r.Context(), p, keys[0], input)
		snapshotResponse(w, snap, err)
	}))
	mux.Handle("GET /v1/snapshots/{snapshot_id}", Require(auth, identity.FeedsRead, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		if r.Method != "GET" || r.ContentLength != 0 || len(r.TransferEncoding) > 0 || r.URL.RawQuery != "" {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		id := r.PathValue("snapshot_id")
		if identity.ValidateID(id) != nil {
			serviceProblem(w, identity.ErrNotFound)
			return
		}
		if queries == nil {
			serviceProblem(w, identity.ErrUnavailable)
			return
		}
		snap, err := queries.Get(r.Context(), p, id)
		snapshotResponse(w, snap, err)
	}))
	// Catch unsupported methods with the same auth/scope boundary and JSON errors.
	mux.Handle("/v1/feed-queries", Require(auth, identity.FeedsRead, invalidMethod))
	mux.Handle("/v1/snapshots/{snapshot_id}", Require(auth, identity.FeedsRead, invalidMethod))
	mux.Handle("POST /v1/feedback", Require(auth, identity.FeedbackWrite, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		m, err := readObject(w, r)
		if err != nil || !feedbackShape(m) {
			serviceProblem(w, feedback.ErrInvalid)
			return
		}
		var event feedback.Event
		if decodeObject(m, &event) != nil || event.Validate() != nil {
			serviceProblem(w, feedback.ErrInvalid)
			return
		}
		if events == nil {
			serviceProblem(w, identity.ErrUnavailable)
			return
		}
		if err := events.Submit(r.Context(), p, event); err != nil {
			serviceProblem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.Handle("/v1/feedback", Require(auth, identity.FeedbackWrite, invalidMethod))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { serviceProblem(w, identity.ErrNotFound) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		requestID(w)
		// Avoid automatic ServeMux redirects exposing a different transport shape.
		if path.Clean(r.URL.Path) != r.URL.Path {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func invalidMethod(w http.ResponseWriter, r *http.Request, p identity.Principal) {
	problem(w, 400, "invalid_request", "Unsupported method for this endpoint", false)
}
func snapshotResponse(w http.ResponseWriter, snap query.Snapshot, err error) {
	if err != nil {
		serviceProblem(w, err)
		return
	}
	response, err := snap.Response(requestID(w), time.Now().UTC())
	if err != nil {
		serviceProblem(w, query.ErrUnavailable)
		return
	}
	// Encode completely before committing HTTP success; no partial JSON on errors.
	b, err := json.Marshal(response)
	if err != nil {
		serviceProblem(w, query.ErrUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(b, '\n'))
}
