package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jonesrussell/northway/internal/article"
	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/query"
	"github.com/jonesrussell/northway/internal/source"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func rid(n int) string { return fmt.Sprintf("%08x-1111-4111-8111-111111111111", n) }
func retrievalFixture(t *testing.T, cats ...string) (*Store, string, identity.Principal, feed.Preferences) {
	t.Helper()
	s, path := fresh(t)
	s.clock = func() time.Time { return queryEpoch }
	must(t, s.CreateTenant(t.Context(), tenantA))
	p := operator(tenantA)
	must(t, s.CreateFeed(t.Context(), p, feed.Feed{ID: feedID, Title: "Synthetic personal feed"}))
	pref := feed.Preferences{Categories: cats, PublisherCap: 2, Exclude: []string{}}
	return s, path, p, pref
}
func addRetrievalSource(t *testing.T, s *Store, p identity.Principal, pref *feed.Preferences, n int, group string, cats ...string) string {
	t.Helper()
	id := rid(n)
	must(t, s.CreateSource(t.Context(), p, source.Source{ID: id, Title: "Synthetic " + group, URL: fmt.Sprintf("https://example.invalid/feed/%d", n)}))
	must(t, s.AttachSource(t.Context(), p, feedID, id))
	pref.Sources = append(pref.Sources, feed.SourceRule{SourceID: id, PublisherGroup: group, Categories: cats})
	return id
}
func addRetrievalItem(t *testing.T, s *Store, p identity.Principal, n int, src, title, url string, published *time.Time, observed time.Time) {
	t.Helper()
	must(t, s.PutArticle(t.Context(), p, article.Article{ID: rid(n), SourceID: src, OriginID: fmt.Sprint(n), Title: title, URL: url, PublishedAt: published, ObservedAt: observed, Body: "PRIVATE-BODY must never become evidence or a title-match"}))
}
func runRetrieval(t *testing.T, s *Store, p identity.Principal, key string, ctx query.Context, age, limit int) query.Snapshot {
	t.Helper()
	v, err := query.NewService(s).Query(t.Context(), p, key, query.Request{FeedID: feedID, Context: ctx, MaxAgeHours: age, Limit: limit})
	must(t, err)
	return v
}
func recentContext() query.Context {
	return query.Context{Intent: "Show recent news", Technologies: []query.Technology{}}
}
func itemIDs(s query.Snapshot) []string {
	out := []string{}
	for _, v := range s.Items {
		out = append(out, v.ArticleID)
	}
	return out
}
func containsWarning(warnings []string, s string) bool {
	for _, w := range warnings {
		if strings.Contains(w, s) {
			return true
		}
	}
	return false
}

func TestRetrievalAgeExclusionLiteralFTSAndUnknownDate(t *testing.T) {
	s, _, p, pref := retrievalFixture(t, "development")
	src := addRetrievalSource(t, s, p, &pref, 101, "publisher", "development")
	pref.UseContext = true
	pref.Exclude = []string{"tutorial"}
	must(t, s.ConfigureFeedPreferences(t.Context(), p, feedID, pref))
	recent, old, future := queryEpoch.Add(-time.Hour), queryEpoch.Add(-90*24*time.Hour), queryEpoch.Add(time.Hour)
	addRetrievalItem(t, s, p, 201, src, "PHP release", "https://example.invalid/eligible", &recent, old)
	addRetrievalItem(t, s, p, 202, src, "PHP undated", "https://example.invalid/undated", nil, recent)
	addRetrievalItem(t, s, p, 203, src, "PHP old article newly observed", "https://example.invalid/old", &old, recent)
	addRetrievalItem(t, s, p, 204, src, "PHP TUTORIAL", "https://example.invalid/tutorial", &recent, recent)
	addRetrievalItem(t, s, p, 205, src, "PHP future", "https://example.invalid/future", &future, recent)
	addRetrievalItem(t, s, p, 206, src, "PHP future observation", "https://example.invalid/future-observation", &recent, future)
	addRetrievalItem(t, s, p, 207, src, "Unrelated film news", "https://example.invalid/unrelated", &recent, recent)
	ctx := query.Context{Intent: "PHP OR (\"*) --", Technologies: []query.Technology{{Name: "PHP", Version: "8.5"}}}
	snap := runRetrieval(t, s, p, "age-exclusion-key-0001", ctx, 24, 5)
	if !reflect.DeepEqual(itemIDs(snap), []string{rid(201), rid(202)}) {
		t.Fatal(itemIDs(snap))
	}
	response, err := snap.Response(rid(900), queryEpoch)
	must(t, err)
	if response.Items[0].PublishedAt == nil || response.Items[1].PublishedAt != nil || response.Coverage.Status != "stale" {
		t.Fatal(response)
	}
	data, err := json.Marshal(response)
	must(t, err)
	if strings.Contains(string(data), "PRIVATE-BODY") || strings.Contains(string(data), "8.5") || strings.Contains(string(data), "article_excerpt") {
		t.Fatal("unsupported evidence", string(data))
	}
	if response.Items[0].Evidence[0].Basis != "source_metadata" || response.Items[0].Evidence[0].Text != "PHP release" {
		t.Fatal(response.Items)
	}
}

func mixedFixture(t *testing.T) (*Store, string, identity.Principal, feed.Preferences) {
	s, path, p, pref := retrievalFixture(t, "development", "entertainment", "canada", "first_nations", "world")
	cats := []string{"development", "entertainment", "canada", "canada", "first_nations", "first_nations", "world", "world"}
	groups := []string{"corus", "corus", "corus", "canadian", "indigenous", "nation", "corus", "global_other"}
	urls := []string{"https://EXAMPLE.invalid:443/shared", "https://example.invalid/e", "https://example.invalid/c", "https://example.invalid/c2", "https://example.invalid/shared", "https://example.invalid/n2", "https://example.invalid/w", "https://example.invalid/w2"}
	for i, cat := range cats {
		src := addRetrievalSource(t, s, p, &pref, 101+i, groups[i], cat)
		at := queryEpoch.Add(-time.Duration(i+1) * time.Minute)
		addRetrievalItem(t, s, p, 201+i, src, "Synthetic "+cat+" headline", urls[i], &at, at)
	}
	must(t, s.ConfigureFeedPreferences(t.Context(), p, feedID, pref))
	return s, path, p, pref
}
func TestMixedDigestConflictFallbackAndStableReplay(t *testing.T) {
	s, path, p, pref := mixedFixture(t)
	ctx := query.Context{Intent: "PHP project news", Technologies: []query.Technology{{Name: "PHP"}}}
	snap := runRetrieval(t, s, p, "mixed-selection-key-01", ctx, 24, 5)
	want := []string{rid(201), rid(202), rid(204), rid(206), rid(208)}
	if !reflect.DeepEqual(itemIDs(snap), want) {
		t.Fatal(itemIDs(snap))
	}
	again := runRetrieval(t, s, p, "mixed-selection-key-02", ctx, 24, 5)
	if again.ID != snap.ID {
		t.Fatal("cache miss")
	}
	// Replay survives content change/restart with original metadata and ordering.
	must(t, s.DeleteArticle(t.Context(), p, rid(201)))
	must(t, s.Close())
	s, err := Open(t.Context(), path)
	must(t, err)
	defer s.Close()
	s.clock = func() time.Time { return queryEpoch }
	again, err = query.NewService(s).Get(t.Context(), p, snap.ID)
	must(t, err)
	if !reflect.DeepEqual(itemIDs(again), want) || again.Items[0].Title != snap.Items[0].Title {
		t.Fatal(again)
	}
	// Removing a source from preferences revokes retained access even when its
	// feed_sources membership remains; the stored order is never reshuffled.
	pref.Sources = slices.Delete(pref.Sources, 5, 6)
	must(t, s.ConfigureFeedPreferences(t.Context(), p, feedID, pref))
	revoked, err := query.NewService(s).Get(t.Context(), p, snap.ID)
	must(t, err)
	response, err := revoked.Response(rid(900), queryEpoch)
	must(t, err)
	if !revoked.Suppressed || len(revoked.Items) != 4 || !containsWarning(response.Warnings, "category: first_nations.") {
		t.Fatal(response)
	}
	var attempts int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM poll_attempts").Scan(&attempts))
	if attempts != 0 {
		t.Fatal("retrieval initiated acquisition")
	}
}

func TestPreferenceOrderCapMissingCategoryAndInvalidation(t *testing.T) {
	s, _, p, pref := mixedFixture(t)
	before := runRetrieval(t, s, p, "preferences-first-key", recentContext(), 24, 5)
	pref.Categories = []string{"world", "first_nations", "canada", "entertainment", "development"}
	pref.PublisherCap = 1
	must(t, s.ConfigureFeedPreferences(t.Context(), p, feedID, pref))
	after := runRetrieval(t, s, p, "preferences-next-key-", recentContext(), 24, 5)
	if after.ID == before.ID || after.FeedRevision <= before.FeedRevision || after.Items[0].Category != "world" || len(after.Items) != 3 {
		t.Fatal(after)
	}
	response, err := after.Response(rid(900), queryEpoch)
	must(t, err)
	if !containsWarning(response.Warnings, "category: development.") || !containsWarning(response.Warnings, "category: entertainment.") {
		t.Fatal(response.Warnings)
	}
	short := runRetrieval(t, s, p, "preferences-limit-key", recentContext(), 24, 2)
	if len(short.Items) != 2 || short.Items[1].Category != "first_nations" {
		t.Fatal(short)
	}
}

func TestCoverageUsesPersistedPollsAndAgesWithoutWork(t *testing.T) {
	s, _, p, pref := retrievalFixture(t, "world")
	for i := 0; i < 2; i++ {
		addRetrievalSource(t, s, p, &pref, 101+i, fmt.Sprint("publisher", i), "world")
	}
	must(t, s.ConfigureFeedPreferences(t.Context(), p, feedID, pref))
	mark := func(n int) {
		must(t, s.ConfigurePoll(t.Context(), p, ingest.Policy{SourceID: rid(n), URL: fmt.Sprintf("https://example.invalid/feed/%d", n), Approved: true, Enabled: true, Interval: time.Hour, MaxBytes: 2048}))
		claim, err := s.ClaimPoll(t.Context(), p)
		must(t, err)
		must(t, s.FinishPoll(t.Context(), p, claim.ID, ingest.Result{Status: 200}))
	}
	mark(101)
	partial := runRetrieval(t, s, p, "coverage-partial-key", recentContext(), 24, 5)
	r, err := partial.Response(rid(900), queryEpoch)
	must(t, err)
	if r.Coverage.Status != "partial" || r.Coverage.Current != 1 || len(r.Items) != 0 {
		t.Fatal(r)
	}
	mark(102)
	// A new request after cache expiry observes new poll state even without new articles.
	later := queryEpoch.Add(2 * time.Minute)
	s.clock = func() time.Time { return later }
	complete := runRetrieval(t, s, p, "coverage-complete-key", recentContext(), 24, 5)
	r, err = complete.Response(rid(900), later)
	must(t, err)
	if r.Coverage.Status != "complete" || r.Coverage.Current != 2 || len(r.Items) != 0 {
		t.Fatal(r)
	}
	r, err = complete.Response(rid(900), queryEpoch.Add(3*time.Hour))
	must(t, err)
	if r.Coverage.Status != "stale" || !containsWarning(r.Warnings, "cache freshness") {
		t.Fatal(r)
	}
}

func TestRetrievalTenantScopeAndRequestFences(t *testing.T) {
	s, _, p, pref := retrievalFixture(t, "canada")
	src := addRetrievalSource(t, s, p, &pref, 101, "publisher", "canada")
	must(t, s.ConfigureFeedPreferences(t.Context(), p, feedID, pref))
	addRetrievalItem(t, s, p, 201, src, "Canada headline", "https://example.invalid/a", nil, queryEpoch)
	must(t, s.CreateTenant(t.Context(), tenantB))
	other := operator(tenantB)
	must(t, s.CreateFeed(t.Context(), other, feed.Feed{ID: feedID, Title: "Other feed"}))
	must(t, s.CreateSource(t.Context(), other, source.Source{ID: src, Title: "PRIVATE-TENANT", URL: "https://example.invalid/private"}))
	must(t, s.AttachSource(t.Context(), other, feedID, src))
	for i := 0; i < 60; i++ {
		addRetrievalItem(t, s, other, 301+i, src, "PRIVATE-TENANT Canada", "https://example.invalid/other", nil, queryEpoch)
	}
	snap := runRetrieval(t, s, p, "tenant-a-query-key-01", recentContext(), 24, 5)
	if len(snap.Items) != 1 || snap.Items[0].ArticleID != rid(201) {
		t.Fatal(snap)
	}
	if _, err := query.NewService(s).Get(t.Context(), other, snap.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	_, secret := newKey(t, s, tenantA, identity.FeedbackWrite)
	feedbackKey, err := identity.NewService(s).Authenticate(t.Context(), secret.Reveal())
	must(t, err)
	if _, err = query.NewService(s).Query(t.Context(), feedbackKey, "wrong-scope-query-key", query.Request{FeedID: feedID, Context: recentContext(), MaxAgeHours: 24, Limit: 5}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal(err)
	}
	if err = s.ConfigureFeedPreferences(t.Context(), feedbackKey, feedID, pref); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal(err)
	}
	req := query.Request{FeedID: feedID, Context: recentContext(), MaxAgeHours: 24, Limit: 4}
	claim, err := s.BeginQuery(t.Context(), p, "request-fence-key-01", req, query.Policy{RankerVersion: query.DeterministicVersion, Lease: time.Minute, CacheTTL: time.Minute})
	must(t, err)
	changed := req
	changed.Limit = 3
	if _, err = s.RetrieveCandidates(t.Context(), p, claim.WorkID, changed); !errors.Is(err, query.ErrConflict) {
		t.Fatal(err)
	}
	corpus, err := s.RetrieveCandidates(t.Context(), p, claim.WorkID, req)
	must(t, err)
	selection, err := query.Select(corpus, req.Limit)
	must(t, err)
	must(t, s.SetSourceEnabled(t.Context(), p, src, false))
	if _, err = s.CompleteRetrieval(t.Context(), p, claim.WorkID, selection); !errors.Is(err, query.ErrConflict) {
		t.Fatal("revoked work finalized", err)
	}
}

func TestRetrievalCandidateCapAndAtomicFailureCleanup(t *testing.T) {
	s, _, p, pref := retrievalFixture(t, "development")
	src := addRetrievalSource(t, s, p, &pref, 101, "publisher", "development")
	pref.Exclude = []string{"tutorial"}
	must(t, s.ConfigureFeedPreferences(t.Context(), p, feedID, pref))
	for i := 0; i < 60; i++ {
		addRetrievalItem(t, s, p, 201+i, src, "Release headline", fmt.Sprintf("https://example.invalid/%d", i), nil, queryEpoch.Add(-time.Hour))
	}
	for i := 0; i < 60; i++ {
		addRetrievalItem(t, s, p, 301+i, src, "Tutorial", fmt.Sprintf("https://example.invalid/tutorial/%d", i), nil, queryEpoch)
	}
	snap := runRetrieval(t, s, p, "bounded-candidates-key", recentContext(), 24, 5)
	if len(snap.Items) != 2 || snap.Items[0].ArticleID != rid(201) || !containsWarning(snap.Details.Warnings, "Candidate window capped") {
		t.Fatal(snap)
	}
	_, err := s.writer.ExecContext(t.Context(), `CREATE TRIGGER reject_details BEFORE UPDATE OF details ON query_snapshots BEGIN SELECT RAISE(ABORT,'synthetic atomic failure'); END`)
	must(t, err)
	req := query.Request{FeedID: feedID, Context: recentContext(), MaxAgeHours: 25, Limit: 5}
	if _, err = query.NewService(s).Query(t.Context(), p, "failed-snapshot-key-01", req); err == nil {
		t.Fatal("partial finalization accepted")
	}
	var snapshots, failed int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM query_snapshots").Scan(&snapshots))
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM query_work WHERE work_state='failed' AND spend_state='settled' AND reserved_micros=0 AND actual_micros=0").Scan(&failed))
	if snapshots != 1 || failed != 1 {
		t.Fatal("snapshot/claim atomicity", snapshots, failed)
	}
	if _, err = query.NewService(s).Query(t.Context(), p, "failed-snapshot-key-01", req); !errors.Is(err, query.ErrUnavailable) {
		t.Fatal("failed key retried", err)
	}
}

func TestRetrievalPrivateContextAndLegacySnapshot(t *testing.T) {
	s, path, p, pref := retrievalFixture(t, "development")
	src := addRetrievalSource(t, s, p, &pref, 101, "publisher", "development")
	pref.UseContext = true
	must(t, s.ConfigureFeedPreferences(t.Context(), p, feedID, pref))
	addRetrievalItem(t, s, p, 201, src, "PHP release", "https://example.invalid/a", nil, queryEpoch)
	private := "PRIVATE-CONTEXT-ONLY-55dc078347"
	snap := runRetrieval(t, s, p, "private-query-key-0001", query.Context{Intent: private, Technologies: []query.Technology{{Name: "PHP"}}}, 24, 5)
	if len(snap.Items) != 1 {
		t.Fatal(snap)
	}
	for _, file := range []string{path, path + "-wal"} {
		data, err := os.ReadFile(file)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if strings.Contains(string(data), private) {
			t.Fatal("raw context persisted")
		}
	}
	_, err := s.writer.ExecContext(t.Context(), "UPDATE query_snapshots SET details='' WHERE id=?", snap.ID)
	must(t, err)
	if _, err = query.NewService(s).Get(t.Context(), p, snap.ID); !errors.Is(err, query.ErrUnavailable) {
		t.Fatal("legacy evidence fabricated", err)
	}
}

// Runtime evaluation uses independently labelled authored metadata. It is not
// publisher-content evaluation or the owner's two-week relevance acceptance.
func TestLabelledRetrievalEvaluation(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "evaluation", "retrieval.json"))
	must(t, err)
	var suite struct {
		Cases []struct {
			Name, Category string
			Context        query.Context
			UseContext     bool
			Exclude        []string
			Items          []struct {
				Title    string
				Relevant bool
			}
			Expected []int
		}
	}
	must(t, json.Unmarshal(data, &suite))
	if len(suite.Cases) != 7 {
		t.Fatal("missing interest/context cases")
	}
	for _, tc := range suite.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			s, _, p, pref := retrievalFixture(t, tc.Category)
			pref.UseContext = tc.UseContext
			pref.Exclude = tc.Exclude
			for i, a := range tc.Items {
				src := addRetrievalSource(t, s, p, &pref, 101+i, fmt.Sprintf("publisher%d", i), tc.Category)
				at := queryEpoch.Add(-time.Duration(i) * time.Minute)
				addRetrievalItem(t, s, p, 201+i, src, a.Title, fmt.Sprintf("https://example.invalid/item/%d", i), &at, at)
			}
			must(t, s.ConfigureFeedPreferences(t.Context(), p, feedID, pref))
			snap := runRetrieval(t, s, p, "labelled-evaluation-key", tc.Context, 24, 5)
			want := []string{}
			for _, i := range tc.Expected {
				want = append(want, rid(201+i))
			}
			if !reflect.DeepEqual(itemIDs(snap), want) {
				t.Fatalf("got %v want %v", itemIDs(snap), want)
			}
			relevant := 0
			for _, i := range tc.Expected {
				if tc.Items[i].Relevant {
					relevant++
				}
			}
			if len(snap.Items) == 0 || relevant*100/len(snap.Items) < 80 {
				t.Fatal("relevance baseline below target")
			}
		})
	}
}

func TestRetrievalCancellationDoesNotProduceSnapshot(t *testing.T) {
	s, _, p, pref := retrievalFixture(t, "world")
	addRetrievalSource(t, s, p, &pref, 101, "publisher", "world")
	must(t, s.ConfigureFeedPreferences(t.Context(), p, feedID, pref))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := query.NewService(s).Query(ctx, p, "cancelled-query-key-01", query.Request{FeedID: feedID, Context: recentContext(), MaxAgeHours: 24, Limit: 5}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var n int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM query_snapshots").Scan(&n))
	if n != 0 {
		t.Fatal("cancelled snapshot")
	}
}

func TestRetrievalResponseContract(t *testing.T) {
	s, _, p, pref := retrievalFixture(t, "world")
	for i := 0; i < 20; i++ {
		src := addRetrievalSource(t, s, p, &pref, 101+i, fmt.Sprintf("publisher%d", i), "world")
		_, err := s.writer.ExecContext(t.Context(), "UPDATE sources SET title=? WHERE tenant_id=? AND id=?", strings.Repeat("<", 512), tenantA, src)
		must(t, err)
		addRetrievalItem(t, s, p, 201+i, src, strings.Repeat("<", 512), fmt.Sprintf("https://example.invalid/%02d", i)+strings.Repeat("&", 2000), nil, queryEpoch)
	}
	must(t, s.ConfigureFeedPreferences(t.Context(), p, feedID, pref))
	snap := runRetrieval(t, s, p, "maximum-response-key", recentContext(), 24, 20)
	loaded, err := query.NewService(s).Get(t.Context(), p, snap.ID)
	must(t, err)
	response, err := loaded.Response(rid(900), queryEpoch)
	must(t, err)
	encoded, err := json.Marshal(response)
	must(t, err)
	if len(response.Items) != 20 || len(encoded) > 1024*1024 || !containsWarning(response.Warnings, "Publication date is unknown") {
		t.Fatal("response bounds", len(response.Items), len(encoded))
	}
	if path := os.Getenv("NORTHWAY_TEST_RESPONSE"); path != "" {
		must(t, os.WriteFile(path, encoded, 0600))
	}
}
