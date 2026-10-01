package httpapi

import (
	"context"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/query"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

type CollectionStore interface {
	AddCollectionSeed(context.Context, identity.Principal, ingest.CollectionSeed) error
	CollectionStatus(context.Context, identity.Principal) (ingest.CollectionStatus, error)
	CollectionBatch(context.Context, identity.Principal, int64) (ingest.Batch, error)
	TakeRequestBudget(context.Context, identity.Principal, time.Time) (bool, error)
}

// WithCollectionAPI accepts only distinct agent grants for collection routes.
// Existing feed/customer handlers retain their credential classes unchanged.
func WithCollectionAPI(fallback http.Handler, auth Authenticator, store CollectionStore) http.Handler {
	mux := http.NewServeMux()
	require := func(scope identity.CollectionScopes, next func(http.ResponseWriter, *http.Request, identity.Principal)) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID(w)
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			values := r.Header.Values("Authorization")
			if len(values) != 1 {
				authProblem(w, 401)
				return
			}
			scheme, raw, ok := strings.Cut(values[0], " ")
			if !ok || !strings.EqualFold(scheme, "Bearer") || len(raw) != 81 {
				authProblem(w, 401)
				return
			}
			if auth == nil || store == nil {
				authProblem(w, 503)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			p, e := auth.Authenticate(ctx, raw)
			if e != nil {
				serviceProblem(w, e)
				return
			}
			if _, e = p.RequireCollection(scope); e != nil {
				serviceProblem(w, e)
				return
			}
			allowed, e := store.TakeRequestBudget(ctx, p, time.Now().UTC())
			if e != nil {
				serviceProblem(w, e)
				return
			}
			if !allowed {
				serviceProblem(w, identity.ErrRateLimited)
				return
			}
			next(w, r, p)
		})
	}
	empty := func(r *http.Request) bool { return r.ContentLength == 0 && len(r.TransferEncoding) == 0 }
	mux.Handle("POST /v1/collection/seeds", require(identity.CollectionSeed, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		if r.URL.RawQuery != "" {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		m, e := readObject(w, r)
		if e != nil || len(m) != 3 || m["id"] == nil || m["url"] == nil || m["title"] == nil {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		var seed ingest.CollectionSeed
		if decodeObject(m, &seed) != nil {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		if e = store.AddCollectionSeed(r.Context(), p, seed); e != nil {
			serviceProblem(w, e)
			return
		}
		customerJSON(w, 200, map[string]any{"id": seed.ID, "acquisition_changed": false})
	}))
	mux.Handle("GET /v1/collection/status", require(identity.CollectionStatus, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		if !empty(r) || r.URL.RawQuery != "" {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		status, e := store.CollectionStatus(r.Context(), p)
		if e != nil {
			serviceProblem(w, e)
			return
		}
		customerJSON(w, 200, status)
	}))
	mux.Handle("GET /v1/collection/observations", require(identity.CollectionObservationsRead, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		q, parseErr := url.ParseQuery(r.URL.RawQuery)
		values := q["after"]
		if parseErr != nil || !empty(r) || len(q) != 1 || len(values) != 1 {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		after, e := strconv.ParseInt(values[0], 10, 64)
		if e != nil || after < 0 {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		batch, e := store.CollectionBatch(r.Context(), p, after)
		if e != nil {
			serviceProblem(w, e)
			return
		}
		customerJSON(w, 200, batch)
	}))
	mux.Handle("/v1/collection/", require(identity.CollectionStatus, invalidMethod))
	mux.Handle("/", fallback)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID(w)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if path.Clean(r.URL.Path) != r.URL.Path {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
