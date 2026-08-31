package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/feedback"
	"github.com/jonesrussell/northway/internal/httpapi"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"github.com/jonesrussell/northway/internal/sqlite"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type httpFixture struct {
	s                       *sqlite.Store
	one, two                identity.Principal
	key, other, write, read identity.Secret
	server                  *httptest.Server
	responses               []json.RawMessage
}

func apiFixture(t *testing.T) *httpFixture {
	t.Helper()
	s := testStore(t)
	one := fixtureTenant(t, s, tenantOne, "Tenant One World")
	two := fixtureTenant(t, s, tenantTwo, "Tenant Two World")
	for _, p := range []identity.Principal{one, two} {
		check(t, s.ConfigureFeedPreferences(t.Context(), p, corpusID, feed.Preferences{Categories: []string{"world"}, Sources: []feed.SourceRule{{SourceID: corpusID, PublisherGroup: "publisher", Categories: []string{"world"}}}, PublisherCap: 2}))
	}
	_, key := fixtureKey(t, s, one, identity.FeedsRead|identity.FeedbackWrite)
	_, other := fixtureKey(t, s, two, identity.FeedsRead|identity.FeedbackWrite)
	_, write := fixtureKey(t, s, one, identity.FeedbackWrite)
	_, read := fixtureKey(t, s, one, identity.FeedsRead)
	server := httptest.NewServer(httpapi.NewAPI(identity.NewService(s), query.NewService(s), feedback.NewService(s)))
	t.Cleanup(server.Close)
	return &httpFixture{s: s, one: one, two: two, key: key, other: other, write: write, read: read, server: server}
}
func queryBody() string {
	return `{"feed_id":"` + corpusID + `","context":{"intent":"Recent world news","technologies":[]},"max_age_hours":24,"limit":5}`
}
func eventBody(event, snapshot, action, reverse string) string {
	e := feedback.Event{EventID: event, SnapshotID: snapshot, ArticleID: corpusID, Action: action, ReversesEventID: reverse}
	b, _ := json.Marshal(e)
	return string(b)
}
func hid(n int) string { return fmt.Sprintf("%08x-1111-4111-8111-111111111111", n) }
func (f *httpFixture) request(t *testing.T, method, path, body string, key identity.Secret, idempotency string, modify func(*http.Request)) (int, []byte, http.Header) {
	t.Helper()
	r, err := http.NewRequest(method, f.server.URL+path, strings.NewReader(body))
	check(t, err)
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
	response, err := f.server.Client().Do(r)
	check(t, err)
	defer response.Body.Close()
	b, err := io.ReadAll(response.Body)
	check(t, err)
	if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("X-Content-Type-Options") != "nosniff" || identity.ValidateID(response.Header.Get("X-Request-ID")) != nil {
		t.Fatal("missing boundary headers", response.Header)
	}
	if response.StatusCode == 204 {
		if len(b) != 0 {
			t.Fatal("204 body")
		}
	} else {
		if response.Header.Get("Content-Type") != "application/json" {
			t.Fatal("non JSON response", response.StatusCode, string(b))
		}
		var obj map[string]any
		check(t, json.Unmarshal(b, &obj))
		if obj["request_id"] != response.Header.Get("X-Request-ID") {
			t.Fatal("request IDs disagree")
		}
		f.responses = append(f.responses, append([]byte{}, b...))
	}
	return response.StatusCode, b, response.Header
}
func httpSnapshot(t *testing.T, status int, b []byte) query.Response {
	t.Helper()
	if status != 200 {
		t.Fatalf("HTTP %d: %s", status, b)
	}
	var v query.Response
	check(t, json.Unmarshal(b, &v))
	return v
}
func expectProblem(t *testing.T, status int, b []byte, h http.Header, want int, code string, retry bool) {
	t.Helper()
	var v struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
	}
	check(t, json.Unmarshal(b, &v))
	if status != want || v.Code != code || v.Retryable != retry || (h.Get("Retry-After") == "1") != retry {
		t.Fatalf("status=%d body=%s retry=%q; want %d/%s/%v", status, b, h.Get("Retry-After"), want, code, retry)
	}
}
func TestHTTPContracts(t *testing.T) {
	f := apiFixture(t)
	status, b, _ := f.request(t, "POST", "/v1/feed-queries", queryBody(), f.key, "http-original-query", nil)
	original := httpSnapshot(t, status, b)
	if len(original.Items) != 1 || original.Items[0].Title != "Tenant One World" || original.Ranking.Mode != "deterministic_fallback" {
		t.Fatal(original)
	}
	status, b, _ = f.request(t, "POST", "/v1/feed-queries", queryBody(), f.key, "http-original-query", nil)
	replay := httpSnapshot(t, status, b)
	if original.SnapshotID != replay.SnapshotID || original.RequestID == replay.RequestID {
		t.Fatal("replay contract")
	}
	// Struct key ordering and integer encodings are canonical, not new work.
	canonical := strings.ReplaceAll(strings.ReplaceAll(queryBody(), `"limit":5`, `"limit":5.0`), `"max_age_hours":24`, `"max_age_hours":24e0`)
	status, b, _ = f.request(t, "POST", "/v1/feed-queries", canonical, f.key, "http-original-query", nil)
	if httpSnapshot(t, status, b).SnapshotID != original.SnapshotID {
		t.Fatal("canonical replay changed")
	}
	status, b, h := f.request(t, "POST", "/v1/feed-queries", strings.Replace(queryBody(), `"limit":5`, `"limit":4`, 1), f.key, "http-original-query", nil)
	expectProblem(t, status, b, h, 409, "conflict", false)
	status, b, _ = f.request(t, "GET", "/v1/snapshots/"+original.SnapshotID, "", f.key, "", nil)
	stored := httpSnapshot(t, status, b)
	if !reflect.DeepEqual(stored.Items, original.Items) || stored.GeneratedAt != original.GeneratedAt || stored.Ranking != original.Ranking {
		t.Fatal("GET changed evidence")
	}
	// Same corpus IDs belong to different tenants; snapshots and caches cannot leak.
	status, b, _ = f.request(t, "POST", "/v1/feed-queries", queryBody(), f.other, "http-original-query", nil)
	other := httpSnapshot(t, status, b)
	if other.SnapshotID == original.SnapshotID || other.Items[0].Title != "Tenant Two World" {
		t.Fatal("tenant leak")
	}
	status, b, h = f.request(t, "GET", "/v1/snapshots/"+original.SnapshotID, "", f.other, "", nil)
	expectProblem(t, status, b, h, 404, "not_found", false)
	// Feedback-only key has no read authority, but can record scoped events.
	status, b, h = f.request(t, "GET", "/v1/snapshots/"+original.SnapshotID, "", f.write, "", nil)
	expectProblem(t, status, b, h, 403, "forbidden", false)
	save := eventBody(hid(301), original.SnapshotID, "save", "")
	for range 2 {
		status, b, h = f.request(t, "POST", "/v1/feedback", save, f.write, "", nil)
		if status != 204 {
			t.Fatal(status, string(b))
		}
	}
	status, b, h = f.request(t, "POST", "/v1/feedback", save, f.read, "", nil)
	expectProblem(t, status, b, h, 403, "forbidden", false)
	status, b, h = f.request(t, "POST", "/v1/feedback", eventBody(hid(301), original.SnapshotID, "dismiss", ""), f.write, "", nil)
	expectProblem(t, status, b, h, 409, "conflict", false)
	status, b, h = f.request(t, "POST", "/v1/feedback", save, f.other, "", nil)
	expectProblem(t, status, b, h, 404, "not_found", false)
	// Identical event UUID is independently available in tenant two.
	status, b, _ = f.request(t, "POST", "/v1/feedback", eventBody(hid(301), other.SnapshotID, "dismiss", ""), f.other, "", nil)
	if status != 204 {
		t.Fatal(status, string(b))
	}
	undo := eventBody(hid(302), original.SnapshotID, "undo", hid(301))
	for range 2 {
		status, b, _ = f.request(t, "POST", "/v1/feedback", undo, f.write, "", nil)
		if status != 204 {
			t.Fatal(status, string(b))
		}
	}
	status, b, h = f.request(t, "POST", "/v1/feedback", eventBody(hid(303), original.SnapshotID, "undo", hid(301)), f.write, "", nil)
	expectProblem(t, status, b, h, 409, "conflict", false)
	status, b, h = f.request(t, "POST", "/v1/feedback", eventBody(hid(304), original.SnapshotID, "undo", hid(302)), f.write, "", nil)
	expectProblem(t, status, b, h, 409, "conflict", false)
	missing := strings.Replace(save, `"article_id":"`+corpusID+`"`, `"article_id":"`+privateID+`"`, 1)
	status, b, h = f.request(t, "POST", "/v1/feedback", missing, f.write, "", nil)
	expectProblem(t, status, b, h, 404, "not_found", false)
	status, b, h = f.request(t, "POST", "/v1/feed-queries", `{}`, f.key, "invalid-http-query", nil)
	expectProblem(t, status, b, h, 400, "invalid_request", false)
	var pending query.Request
	check(t, json.Unmarshal([]byte(queryBody()), &pending))
	claim, err := f.s.BeginQuery(t.Context(), f.one, "schema-pending-query", pending, query.Policy{RankerVersion: query.DeterministicVersion, Lease: time.Minute, CacheTTL: time.Minute})
	check(t, err)
	status, b, h = f.request(t, "POST", "/v1/feed-queries", queryBody(), f.key, "schema-pending-query", nil)
	expectProblem(t, status, b, h, 409, "in_progress", true)
	check(t, f.s.FailQuery(t.Context(), f.one, claim.WorkID))
	status, b, h = f.request(t, "POST", "/v1/feed-queries", queryBody(), f.key, "schema-pending-query", nil)
	expectProblem(t, status, b, h, 503, "unavailable", false)
	// Snapshot evidence survives corpus removal; current source rights still win.
	check(t, f.s.DeleteArticle(t.Context(), f.one, corpusID))
	status, b, _ = f.request(t, "GET", "/v1/snapshots/"+original.SnapshotID, "", f.key, "", nil)
	if !reflect.DeepEqual(httpSnapshot(t, status, b).Items, original.Items) {
		t.Fatal("GET re-read corpus")
	}
	check(t, f.s.SetSourceEnabled(t.Context(), f.one, corpusID, false))
	status, b, _ = f.request(t, "POST", "/v1/feed-queries", queryBody(), f.key, "http-original-query", nil)
	if len(httpSnapshot(t, status, b).Items) != 0 {
		t.Fatal("revoked replay leaked item")
	}
	status, b, h = f.request(t, "POST", "/v1/feedback", save, f.write, "", nil)
	expectProblem(t, status, b, h, 404, "not_found", false)
	check(t, f.s.SetFeedEnabled(t.Context(), f.one, corpusID, false))
	status, b, h = f.request(t, "GET", "/v1/snapshots/"+original.SnapshotID, "", f.key, "", nil)
	expectProblem(t, status, b, h, 404, "not_found", false)
	status, b, h = f.request(t, "POST", "/v1/feed-queries", queryBody(), identity.Secret{}, "http-original-query", nil)
	expectProblem(t, status, b, h, 401, "unauthorized", false)
	if path := os.Getenv("NORTHWAY_TEST_HTTP_RESPONSES"); path != "" {
		data, err := json.Marshal(f.responses)
		check(t, err)
		check(t, os.WriteFile(path, data, 0600))
	}
}

func TestHTTPStrictDecoding(t *testing.T) {
	f := apiFixture(t)
	valid := queryBody()
	cases := map[string]string{
		"unknown":                 strings.Replace(valid, `"limit":5`, `"limit":5,"tenant_id":"private"`, 1),
		"duplicate":               strings.Replace(valid, `"limit":5`, `"limit":5,"limit":4`, 1),
		"escaped duplicate":       strings.Replace(valid, `"limit":5`, `"limit":5,"l\u0069mit":4`, 1),
		"nested duplicate":        strings.Replace(valid, `"technologies":[]`, `"technologies":[],"intent":"different"`, 1),
		"nested unknown":          strings.Replace(valid, `"technologies":[]`, `"technologies":[{"name":"Go","unknown":true}]`, 1),
		"nested case alias":       strings.Replace(valid, `"intent"`, `"Intent"`, 1),
		"root case alias":         strings.Replace(valid, `"feed_id"`, `"Feed_ID"`, 1),
		"null context":            strings.Replace(valid, `{"intent":"Recent world news","technologies":[]}`, `null`, 1),
		"null array":              strings.Replace(valid, `"technologies":[]`, `"technologies":null`, 1),
		"null element":            strings.Replace(valid, `"technologies":[]`, `"technologies":[null]`, 1),
		"null optional":           strings.Replace(valid, `"technologies":[]`, `"technologies":[],"focus":null`, 1),
		"null version":            strings.Replace(valid, `"technologies":[]`, `"technologies":[{"name":"Go","version":null}]`, 1),
		"empty version":           strings.Replace(valid, `"technologies":[]`, `"technologies":[{"name":"Go","version":""}]`, 1),
		"missing array":           strings.Replace(valid, `,"technologies":[]`, "", 1),
		"missing required":        strings.Replace(valid, `,"limit":5`, "", 1),
		"fraction":                strings.Replace(valid, `"limit":5`, `"limit":1.000000000000000000000000001`, 1),
		"exponent allocation":     strings.Replace(valid, `"limit":5`, `"limit":1e999999999`, 1),
		"zero":                    strings.Replace(valid, `"limit":5`, `"limit":0`, 1),
		"too many":                strings.Replace(valid, `"limit":5`, `"limit":21`, 1),
		"string int":              strings.Replace(valid, `"limit":5`, `"limit":"5"`, 1),
		"empty intent":            strings.Replace(valid, "Recent world news", "", 1),
		"oversize":                strings.Replace(valid, "Recent world news", strings.Repeat("x", 32769), 1),
		"unpaired high surrogate": strings.Replace(valid, "Recent world news", `\ud800`, 1),
		"unpaired low surrogate":  strings.Replace(valid, "Recent world news", `\udc00`, 1),
		"invalid utf8":            strings.Replace(valid, "Recent world news", string([]byte{255}), 1),
		"trailing document":       valid + `{}`, "array root": `[]`, "null root": `null`, "truncated": `{`, "deep": strings.Repeat(`[`, 20) + strings.Repeat(`]`, 20),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			status, b, h := f.request(t, "POST", "/v1/feed-queries", body, f.key, "strict-rejected-query", nil)
			expectProblem(t, status, b, h, 400, "invalid_request", false)
		})
	}
	// Invalid requests must not consume the key.
	status, b, _ := f.request(t, "POST", "/v1/feed-queries", valid, f.key, "strict-rejected-query", nil)
	httpSnapshot(t, status, b)
	headers := map[string]func(*http.Request){
		"no content type":        func(r *http.Request) { r.Header.Del("Content-Type") },
		"wrong content type":     func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") },
		"duplicate content type": func(r *http.Request) { r.Header.Add("Content-Type", "application/json") },
		"encoding":               func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") },
		"charset":                func(r *http.Request) { r.Header.Set("Content-Type", "application/json; charset=latin1") },
		"missing key":            func(r *http.Request) { r.Header.Del("Idempotency-Key") },
		"duplicate key":          func(r *http.Request) { r.Header.Add("Idempotency-Key", "another-idempotency-key") },
		"short key":              func(r *http.Request) { r.Header.Set("Idempotency-Key", "short") },
	}
	for name, modify := range headers {
		t.Run(name, func(t *testing.T) {
			status, b, h := f.request(t, "POST", "/v1/feed-queries", valid, f.key, "strict-rejected-query", modify)
			expectProblem(t, status, b, h, 400, "invalid_request", false)
		})
	}
	for _, body := range []string{
		eventBody(hid(400), hid(500), "undo", ""),
		eventBody(hid(400), hid(500), "save", hid(300)),
		strings.Replace(eventBody(hid(400), hid(500), "save", ""), `"action":"save"`, `"action":"save","action":"dismiss"`, 1),
		strings.Replace(eventBody(hid(400), hid(500), "save", ""), `"action":"save"`, `"action":"save","reverses_event_id":null`, 1),
	} {
		status, b, h := f.request(t, "POST", "/v1/feedback", body, f.write, "", nil)
		expectProblem(t, status, b, h, 400, "invalid_request", false)
	}
	for _, target := range []struct{ method, path, body string }{{"GET", "/v1/feed-queries", ""}, {"POST", "/v1/snapshots/" + hid(99), ""}, {"GET", "/v1/snapshots/" + hid(99), "{}"}, {"POST", "/v1/feed-queries?tenant=other", valid}, {"POST", "/v1/../v1/feed-queries", valid}, {"GET", "/v1/snapshots/..", ""}} {
		status, b, h := f.request(t, target.method, target.path, target.body, f.key, "strict-rejected-query", nil)
		expectProblem(t, status, b, h, 400, "invalid_request", false)
	}
}

type shortCacheStore struct{ *sqlite.Store }

func (s shortCacheStore) BeginQuery(ctx context.Context, p identity.Principal, k string, r query.Request, policy query.Policy) (query.Claim, error) {
	policy.CacheTTL = time.Second
	return s.Store.BeginQuery(ctx, p, k, r, policy)
}
func TestHTTPInProgressTerminalAndExpiredCache(t *testing.T) {
	f := apiFixture(t)
	var r query.Request
	check(t, json.Unmarshal([]byte(queryBody()), &r))
	claim, err := f.s.BeginQuery(t.Context(), f.one, "pending-http-query", r, query.Policy{RankerVersion: query.DeterministicVersion, Lease: time.Minute, CacheTTL: time.Second})
	check(t, err)
	status, b, h := f.request(t, "POST", "/v1/feed-queries", queryBody(), f.key, "pending-http-query", nil)
	expectProblem(t, status, b, h, 409, "in_progress", true)
	check(t, f.s.FailQuery(t.Context(), f.one, claim.WorkID))
	status, b, h = f.request(t, "POST", "/v1/feed-queries", queryBody(), f.key, "pending-http-query", nil)
	expectProblem(t, status, b, h, 503, "unavailable", false)
	// Actual failure after claim is terminal in its first HTTP response too.
	check(t, f.s.CreateFeed(t.Context(), f.one, feed.Feed{ID: privateID, Title: "Not configured"}))
	for range 2 {
		status, b, h = f.request(t, "POST", "/v1/feed-queries", strings.Replace(queryBody(), corpusID, privateID, 1), f.key, "failure-after-claim", nil)
		expectProblem(t, status, b, h, 503, "unavailable", false)
	}
	short := httptest.NewServer(httpapi.NewAPI(identity.NewService(f.s), query.NewService(shortCacheStore{f.s}), feedback.NewService(f.s)))
	defer short.Close()
	f.server = short
	status, b, _ = f.request(t, "POST", "/v1/feed-queries", queryBody(), f.key, "short-cache-query", nil)
	snap := httpSnapshot(t, status, b)
	timer := time.NewTimer(time.Until(snap.ExpiresAt) + 20*time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-t.Context().Done():
		t.Fatal("cancelled")
	}
	status, b, _ = f.request(t, "GET", "/v1/snapshots/"+snap.SnapshotID, "", f.key, "", nil)
	get := httpSnapshot(t, status, b)
	if !reflect.DeepEqual(get.Items, snap.Items) || get.GeneratedAt != snap.GeneratedAt {
		t.Fatal("expired cache reranked GET")
	}
	status, b, _ = f.request(t, "POST", "/v1/feed-queries", queryBody(), f.key, "short-cache-query", nil)
	if httpSnapshot(t, status, b).SnapshotID != snap.SnapshotID {
		t.Fatal("expiry broke promised replay")
	}
	status, b, _ = f.request(t, "POST", "/v1/feed-queries", queryBody(), f.key, "short-cache-new-key", nil)
	if httpSnapshot(t, status, b).SnapshotID == snap.SnapshotID {
		t.Fatal("expired cache reused for new query")
	}
}

func TestHTTPConcurrentFeedbackAndTransientRedaction(t *testing.T) {
	f := apiFixture(t)
	status, b, _ := f.request(t, "POST", "/v1/feed-queries", queryBody(), f.key, "feedback-concurrent", nil)
	snap := httpSnapshot(t, status, b)
	// Exercise real concurrent HTTP writes without sharing the fixture response recorder.
	body := eventBody(hid(301), snap.SnapshotID, "less_like_this", "")
	var wg sync.WaitGroup
	codes := make(chan int, 8)
	for range 8 {
		wg.Go(func() {
			r, err := http.NewRequestWithContext(t.Context(), "POST", f.server.URL+"/v1/feedback", strings.NewReader(body))
			if err != nil {
				codes <- 0
				return
			}
			r.Header.Set("Authorization", "Bearer "+f.write.Reveal())
			r.Header.Set("Content-Type", "application/json")
			res, err := f.server.Client().Do(r)
			if err != nil {
				codes <- 0
				return
			}
			defer res.Body.Close()
			codes <- res.StatusCode
		})
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 204 {
			t.Fatal(code)
		}
	}
	broken := httptest.NewServer(httpapi.NewAPI(identity.NewService(f.s), failingQueries{}, feedback.NewService(f.s)))
	defer broken.Close()
	f.server = broken
	status, b, h := f.request(t, "POST", "/v1/feed-queries", queryBody(), f.key, "transient-query-key", nil)
	expectProblem(t, status, b, h, 503, "unavailable", true)
	if bytes.Contains(b, []byte("PRIVATE")) {
		t.Fatal("internal error leaked")
	}
}

type failingQueries struct{}

func (failingQueries) Query(context.Context, identity.Principal, string, query.Request) (query.Snapshot, error) {
	return query.Snapshot{}, errors.New("PRIVATE SQL credentials")
}
func (failingQueries) Get(context.Context, identity.Principal, string) (query.Snapshot, error) {
	return query.Snapshot{}, errors.New("PRIVATE SQL credentials")
}

// Pause outside the real read transaction, after candidate selection but before
// finalization, to reproduce a feedback revision arriving during a query.
type pausedRetrievalStore struct {
	*sqlite.Store
	once             sync.Once
	entered, release chan struct{}
	completed        chan error
}

func (s *pausedRetrievalStore) RetrieveCandidates(ctx context.Context, p identity.Principal, id string, r query.Request) (query.Corpus, error) {
	corpus, err := s.Store.RetrieveCandidates(ctx, p, id, r)
	if err != nil {
		return corpus, err
	}
	s.once.Do(func() {
		close(s.entered)
		select {
		case <-s.release:
		case <-ctx.Done():
		}
	})
	return corpus, ctx.Err()
}
func (s *pausedRetrievalStore) CompleteRetrieval(ctx context.Context, p identity.Principal, id string, r query.Retrieval) (query.Snapshot, error) {
	snapshot, err := s.Store.CompleteRetrieval(ctx, p, id, r)
	s.completed <- err
	return snapshot, err
}
func TestHTTPFeedbackDuringQueryHasExplicitRefreshRecovery(t *testing.T) {
	f := apiFixture(t)
	status, b, _ := f.request(t, "POST", "/v1/feed-queries", queryBody(), f.key, "before-interleaving", nil)
	old := httpSnapshot(t, status, b)
	paused := &pausedRetrievalStore{Store: f.s, entered: make(chan struct{}), release: make(chan struct{}), completed: make(chan error, 1)}
	server := httptest.NewServer(httpapi.NewAPI(identity.NewService(f.s), query.NewService(paused), feedback.NewService(f.s)))
	defer server.Close()
	f.server = server
	// Change context so the concurrent query cannot hit the warm cache.
	body := strings.Replace(queryBody(), "Recent world news", "Recent general world news", 1)
	type result struct {
		status int
		body   []byte
		header http.Header
		err    error
	}
	finished := make(chan result, 1)
	go func() {
		// No testing fatal calls or shared response recorder in the worker.
		req, err := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/v1/feed-queries", strings.NewReader(body))
		if err != nil {
			finished <- result{err: err}
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+f.key.Reveal())
		req.Header.Set("Idempotency-Key", "interleaved-query-key")
		response, err := server.Client().Do(req)
		if err != nil {
			finished <- result{err: err}
			return
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		finished <- result{response.StatusCode, data, response.Header, err}
	}()
	select {
	case <-paused.entered:
	case early := <-finished:
		t.Fatalf("query ended before candidate pause: status=%d err=%v", early.status, early.err)
	case <-time.After(3 * time.Second):
		t.Fatal("query did not reach candidates")
	}
	status, b, _ = f.request(t, "POST", "/v1/feedback", eventBody(hid(500), old.SnapshotID, "save", ""), f.write, "", nil)
	if status != 204 {
		t.Fatal(status, string(b))
	}
	close(paused.release)
	var failed result
	select {
	case failed = <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("query did not finish")
	}
	check(t, failed.err)
	select {
	case cause := <-paused.completed:
		if !errors.Is(cause, query.ErrConflict) || errors.Is(cause, context.DeadlineExceeded) {
			t.Fatalf("wanted revision conflict, got %v", cause)
		}
	default:
		t.Fatal("query did not reach real finalization")
	}
	expectProblem(t, failed.status, failed.body, failed.header, 503, "unavailable", false)
	status, b, h := f.request(t, "POST", "/v1/feed-queries", body, f.key, "interleaved-query-key", nil)
	expectProblem(t, status, b, h, 503, "unavailable", false)
	status, b, _ = f.request(t, "GET", "/v1/snapshots/"+old.SnapshotID, "", f.key, "", nil)
	stored := httpSnapshot(t, status, b)
	if !reflect.DeepEqual(stored.Items, old.Items) || stored.GeneratedAt != old.GeneratedAt || stored.Ranking != old.Ranking {
		t.Fatal("old snapshot changed")
	}
	// Represents an explicit user refresh, never an automatic retry/render loop.
	status, b, _ = f.request(t, "POST", "/v1/feed-queries", body, f.key, "deliberate-refresh-key", nil)
	refreshed := httpSnapshot(t, status, b)
	if refreshed.SnapshotID == old.SnapshotID || refreshed.FeedRevision != old.FeedRevision+1 {
		t.Fatal("refresh did not use current revision")
	}
}

func TestHTTPSnapshotHEADIsRejectedWithoutBody(t *testing.T) {
	f := apiFixture(t)
	req, err := http.NewRequestWithContext(t.Context(), "HEAD", f.server.URL+"/v1/snapshots/"+hid(999), nil)
	check(t, err)
	req.Header.Set("Authorization", "Bearer "+f.key.Reveal())
	response, err := f.server.Client().Do(req)
	check(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	check(t, err)
	if response.StatusCode != 400 || len(body) != 0 || response.Header.Get("Cache-Control") != "no-store" || identity.ValidateID(response.Header.Get("X-Request-ID")) != nil {
		t.Fatal(response.StatusCode, string(body), response.Header)
	}
}
