package app_test

import (
	"encoding/json"
	"errors"
	"github.com/jonesrussell/northway/internal/feedback"
	"github.com/jonesrussell/northway/internal/httpapi"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/query"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func collectionGrant(t *testing.T, f *httpFixture, p identity.Principal, scopes identity.CollectionScopes) (identity.AgentGrant, identity.Secret) {
	t.Helper()
	now := time.Now().UTC().Add(-time.Second)
	g, key, e := identity.GenerateAgentGrant(p, scopes, "local adapter qualification", now, now.Add(time.Hour))
	check(t, e)
	check(t, f.s.CreateAgentGrant(t.Context(), p, g))
	return g, key
}
func collectionFixture(t *testing.T) *httpFixture {
	f := apiFixture(t)
	f.server.Close()
	f.server = httptest.NewServer(httpapi.WithCollectionAPI(httpapi.NewAPI(identity.NewService(f.s), query.NewService(f.s), feedback.NewService(f.s)), identity.NewAgentService(f.s), f.s))
	t.Cleanup(f.server.Close)
	return f
}
func TestCollectionAgentAdapterSeedScopeTenantAndRevocation(t *testing.T) {
	f := collectionFixture(t)
	g, key := collectionGrant(t, f, f.one, identity.CollectionSeed|identity.CollectionStatus|identity.CollectionObservationsRead)
	_, other := collectionGrant(t, f, f.two, identity.CollectionStatus)
	id := hid(900)
	body := `{"id":"` + id + `","url":"https://fixture.example/creator","title":"River"}`
	for i := 0; i < 2; i++ {
		code, b, _ := collectionRequest(f, t, "POST", "/v1/collection/seeds", body, key, "", nil)
		if code != 200 {
			t.Fatal(code, string(b))
		}
		var r map[string]any
		check(t, json.Unmarshal(b, &r))
		if r["acquisition_changed"] != false {
			t.Fatal("seed enabled")
		}
	}
	owned, e := f.s.CollectionStatus(t.Context(), f.one)
	check(t, e)
	if owned.Enabled != 0 || owned.Seeds != 1 {
		t.Fatal("admission enabled collection", owned)
	}
	code, b, _ := collectionRequest(f, t, "GET", "/v1/collection/status", "", other, "", nil)
	if code != 200 {
		t.Fatal(code, string(b))
	}
	var status ingest.CollectionStatus
	check(t, json.Unmarshal(b, &status))
	if status.Seeds != 0 {
		t.Fatal("tenant leak", status)
	}
	for _, entry := range []struct {
		path, body string
		key        identity.Secret
		want       int
	}{
		{"/v1/collection/status", "", f.key, 401}, {"/v1/collection/seeds", body, other, 403},
		{"/v1/collection/status?tenant_id=" + string(tenantTwo), "", key, 400},
	} {
		method := "GET"
		if entry.body != "" {
			method = "POST"
		}
		code, b, _ = collectionRequest(f, t, method, entry.path, entry.body, entry.key, "", nil)
		if code != entry.want {
			t.Fatal(entry.path, code, string(b))
		}
	}
	p, e := identity.NewAgentService(f.s).Authenticate(t.Context(), key.Reveal())
	check(t, e)
	if _, e = p.RequireManagement(); !errors.Is(e, identity.ErrForbidden) {
		t.Fatal("agent became first party", e)
	}
	if _, e = p.RequireOperator(); !errors.Is(e, identity.ErrForbidden) {
		t.Fatal("agent became operator", e)
	}
	if _, e = p.Require(identity.FeedsRead); !errors.Is(e, identity.ErrForbidden) {
		t.Fatal("agent became feed key", e)
	}
	if e = f.s.RevokeAgentGrant(t.Context(), f.two, g.ID); !errors.Is(e, identity.ErrNotFound) {
		t.Fatal("cross tenant revoke", e)
	}
	check(t, f.s.RevokeAgentGrant(t.Context(), f.one, g.ID))
	code, _, _ = collectionRequest(f, t, "GET", "/v1/collection/status", "", key, "", nil)
	if code != 401 {
		t.Fatal("revoked grant accepted", code)
	}
	// Feed-only keys still work on the existing feed path.
	code, b, _ = collectionRequest(f, t, "POST", "/v1/feed-queries", queryBody(), f.key, "collection-compatibility", nil)
	if code != 200 {
		t.Fatal("feed compatibility", code, string(b))
	}
}
func TestCollectionAgentObservationReplayAndNoAcquisitionAuthority(t *testing.T) {
	f := collectionFixture(t)
	_, key := collectionGrant(t, f, f.one, identity.CollectionObservationsRead)
	id := hid(901)
	url := "https://fixture.example/creator"
	check(t, f.s.AddCollectionSeed(t.Context(), f.one, ingest.CollectionSeed{ID: id, URL: url, Title: "River"}))
	now := time.Now().UTC()
	check(t, f.s.ConfigurePoll(t.Context(), f.one, ingest.Policy{SourceID: id, URL: url, Mode: "html", Approved: true, Enabled: true, RobotsUntil: now.Add(time.Hour), Interval: time.Hour, MaxBytes: 2048}))
	cl, e := f.s.ClaimCollection(t.Context(), f.one)
	check(t, e)
	check(t, f.s.FinishPoll(t.Context(), f.one, cl.ID, ingest.Result{Status: 200, Bytes: 200, Observations: []ingest.Observation{{Version: 1, AccountURL: url, EvidenceURL: url, OriginalURL: url, Kind: "profile", Title: "River", State: "available", Display: "link", Method: "html-metadata-v1"}}}))
	code, b, _ := collectionRequest(f, t, "GET", "/v1/collection/observations?after=0", "", key, "", nil)
	if code != 200 {
		t.Fatal(code, string(b))
	}
	var batch ingest.Batch
	check(t, json.Unmarshal(b, &batch))
	if len(batch.Events) != 1 {
		t.Fatal(batch)
	}
	_, replay, _ := collectionRequest(f, t, "GET", "/v1/collection/observations?after=0", "", key, "", nil)
	if string(b) != string(replay) {
		t.Fatal("unstable replay")
	}
	p, e := identity.NewAgentService(f.s).Authenticate(t.Context(), key.Reveal())
	check(t, e)
	if _, e = f.s.ClaimCollection(t.Context(), p); !errors.Is(e, identity.ErrForbidden) {
		t.Fatal("agent acquired live work", e)
	}
	for _, path := range []string{"/v1/collection/observations?after=-1", "/v1/collection/observations?after=0&after=1", "/v1/collection/observations?after=0&tenant_id=" + string(tenantTwo)} {
		code, _, _ = collectionRequest(f, t, "GET", path, "", key, "", nil)
		if code != 400 {
			t.Fatal(path, code)
		}
	}
}
func TestCollectionAgentExpiryAndGrantSeparation(t *testing.T) {
	f := collectionFixture(t)
	_, key := collectionGrant(t, f, f.one, identity.CollectionStatus)
	if _, e := identity.NewService(f.s).Authenticate(t.Context(), key.Reveal()); !errors.Is(e, identity.ErrUnauthorized) {
		t.Fatal("agent credential accepted by feed auth", e)
	}
	if _, e := identity.NewAgentService(f.s).Authenticate(t.Context(), f.key.Reveal()); !errors.Is(e, identity.ErrUnauthorized) {
		t.Fatal("feed credential accepted by agent auth", e)
	}
	// Test credentials exist only in this ephemeral fixture. Expiry cannot be minted remotely.
	_, _, e := identity.GenerateAgentGrant(f.one, identity.CollectionStatus, "fixture", time.Now(), time.Now().Add(25*time.Hour))
	if !errors.Is(e, identity.ErrForbidden) {
		t.Fatal("unbounded grant", e)
	}
}

func collectionRequest(f *httpFixture, t *testing.T, method, path, body string, key identity.Secret, idempotency string, modify func(*http.Request)) (int, []byte, http.Header) {
	t.Helper()
	r, e := http.NewRequest(method, f.server.URL+path, strings.NewReader(body))
	check(t, e)
	r.Header.Set("Authorization", "Bearer "+key.Reveal())
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if idempotency != "" {
		r.Header.Set("Idempotency-Key", idempotency)
	}
	if modify != nil {
		modify(r)
	}
	response, e := f.server.Client().Do(r)
	check(t, e)
	defer response.Body.Close()
	b, e := io.ReadAll(response.Body)
	check(t, e)
	if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("X-Content-Type-Options") != "nosniff" || identity.ValidateID(response.Header.Get("X-Request-ID")) != nil {
		t.Fatal("boundary headers", response.Header)
	}
	return response.StatusCode, b, response.Header
}

func TestCollectionAgentSharedRequestBudget(t *testing.T) {
	f := collectionFixture(t)
	_, key := collectionGrant(t, f, f.one, identity.CollectionStatus)
	// This cap is shared with feed/browser requests rather than a separate agent allowance.
	for i := 0; i < 60; i++ {
		allowed, e := f.s.TakeRequestBudget(t.Context(), f.one, time.Now().UTC())
		check(t, e)
		if !allowed {
			t.Fatal(i)
		}
	}
	code, b, _ := collectionRequest(f, t, "GET", "/v1/collection/status", "", key, "", nil)
	if code != 429 {
		t.Fatal("agent escaped shared budget", code, string(b))
	}
}
