package httpapi

import (
	"context"
	"encoding/json"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"net/http"
	"strings"
	"time"
)

type CustomerStore interface {
	EnsureWorkspace(context.Context, identity.Principal) error
	IssueCustomerKey(context.Context, identity.Principal, identity.Scopes) (identity.KeyMetadata, identity.Secret, error)
	ListCustomerKeys(context.Context, identity.Principal) ([]identity.KeyMetadata, error)
	RevokeCustomerKey(context.Context, identity.Principal, string) error
	ListCustomerFeeds(context.Context, identity.Principal) ([]identity.FeedMetadata, error)
	TakeRequestBudget(context.Context, identity.Principal, time.Time) (bool, error)
}

type customerAuthenticator struct {
	Authenticator
	assertions *identity.AssertionVerifier
	store      CustomerStore
}

func (a *customerAuthenticator) AuthenticateRequest(ctx context.Context, raw string, r *http.Request) (identity.Principal, error) {
	var p identity.Principal
	var err error
	// Fixed credential classes, never fallback after assertion verification fails.
	if strings.HasPrefix(raw, "nw1_") {
		p, err = a.Authenticator.Authenticate(ctx, raw)
	} else {
		p, err = a.assertions.Verify(ctx, raw, r.Method+" "+r.URL.EscapedPath(), time.Now().UTC())
	}
	if err != nil {
		return p, err
	}
	ok, err := a.store.TakeRequestBudget(ctx, p, time.Now().UTC())
	if err != nil {
		return identity.Principal{}, identity.ErrUnavailable
	}
	if !ok {
		return identity.Principal{}, identity.ErrRateLimited
	}
	return p, nil
}

// NewCustomerAPI is disabled unless an explicit first-party verifier is supplied.
// The same business services serve browser assertions and external limited keys.
func NewCustomerAPI(auth Authenticator, assertions *identity.AssertionVerifier, store CustomerStore, queries Queries, events Feedback) http.Handler {
	if assertions == nil || store == nil {
		return NewAPI(auth, queries, events)
	}
	a := &customerAuthenticator{auth, assertions, store}
	data := NewAPI(a, queries, events)
	mux := http.NewServeMux()
	managed := func(next func(http.ResponseWriter, *http.Request, identity.Principal)) http.Handler {
		return Require(a, identity.FeedsRead, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
			if _, err := p.RequireManagement(); err != nil {
				serviceProblem(w, err)
				return
			}
			if r.URL.RawQuery != "" {
				serviceProblem(w, query.ErrInvalid)
				return
			}
			next(w, r, p)
		})
	}
	empty := func(r *http.Request) bool { return r.ContentLength == 0 && len(r.TransferEncoding) == 0 }
	mux.Handle("PUT /v1/workspace", managed(func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		if !empty(r) {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		if err := store.EnsureWorkspace(r.Context(), p); err != nil {
			serviceProblem(w, err)
			return
		}
		customerJSON(w, 200, map[string]any{"tenant_id": p.TenantID()})
	}))
	mux.Handle("GET /v1/keys", managed(func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		if !empty(r) {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		keys, err := store.ListCustomerKeys(r.Context(), p)
		if err != nil {
			serviceProblem(w, err)
			return
		}
		customerJSON(w, 200, map[string]any{"keys": keys})
	}))
	mux.Handle("POST /v1/keys", managed(func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		m, err := readObject(w, r)
		if err != nil || len(m) != 1 || m["scopes"] == nil {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		var input struct {
			Scopes string `json:"scopes"`
		}
		if decodeObject(m, &input) != nil {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		scopes, err := identity.ParseScopes(input.Scopes)
		if err != nil {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		metadata, secret, err := store.IssueCustomerKey(r.Context(), p, scopes)
		if err != nil {
			serviceProblem(w, err)
			return
		}
		customerJSON(w, 201, struct {
			Key    identity.KeyMetadata `json:"key"`
			Secret string               `json:"secret"`
		}{metadata, secret.Reveal()})
	}))
	mux.Handle("DELETE /v1/keys/{id}", managed(func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		if !empty(r) {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		if err := store.RevokeCustomerKey(r.Context(), p, r.PathValue("id")); err != nil {
			serviceProblem(w, err)
			return
		}
		w.WriteHeader(204)
	}))
	mux.Handle("GET /v1/feeds", Require(a, identity.FeedsRead, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		if !empty(r) || r.URL.RawQuery != "" {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		feeds, err := store.ListCustomerFeeds(r.Context(), p)
		if err != nil {
			serviceProblem(w, err)
			return
		}
		customerJSON(w, 200, map[string]any{"feeds": feeds})
	}))
	mux.Handle("/", data)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Reject alternate path encodings rather than ServeMux redirects.
		if strings.Contains(r.URL.EscapedPath(), "%") || strings.Contains(r.URL.Path, "//") || strings.Contains(r.URL.Path, "/.") {
			serviceProblem(w, query.ErrInvalid)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func customerJSON(w http.ResponseWriter, status int, value any) {
	b, err := json.Marshal(value)
	if err != nil {
		serviceProblem(w, identity.ErrUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n'))
}
