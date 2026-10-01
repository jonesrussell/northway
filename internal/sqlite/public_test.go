package sqlite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/feedback"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/query"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
)

type publicSyntheticFetcher struct{ calls atomic.Int64 }

func (f *publicSyntheticFetcher) Fetch(_ context.Context, c ingest.Claim) ingest.Result {
	f.calls.Add(1)
	return ingest.Result{Status: 200, Bytes: 256, ETag: "synthetic-v1", Items: []ingest.Item{{OriginID: "shared-origin", URL: "https://publisher.invalid/article", Title: "Shared world headline"}}}
}
func publicDefinition(n int) PublicDefinition {
	return PublicDefinition{URL: fmt.Sprintf("https://publisher%d.invalid/feed", n), Title: "Public publisher", Publisher: "Synthetic Publisher", Topic: "world", Language: "en", Provenance: "synthetic local fixture; no network", ReviewState: "approved", RightsBasis: "reviewed_metadata", Enabled: true, Interval: 24 * time.Hour, MaxBytes: ingest.MaxResponseBytes}
}
func publicWorkspace(t *testing.T, s *Store, tenant identity.TenantID, src string) {
	t.Helper()
	seed(t, s, tenant)
	must(t, s.queries(s.writer).EnsureCustomerWorkspace(t.Context(), sqlc.EnsureCustomerWorkspaceParams{TenantID: string(tenant), CreatedAt: s.queryTime().UnixMicro()}))
	must(t, s.SubscribePublicSource(t.Context(), operator(tenant), feedID, src, true))
	pref := feed.Preferences{Categories: []string{"world"}, PublisherCap: 2, Sources: []feed.SourceRule{{SourceID: src, PublisherGroup: "shared-publisher", Categories: []string{"world"}}}}
	must(t, s.ConfigureFeedPreferences(t.Context(), operator(tenant), feedID, pref))
}
func pgCount(t *testing.T, s *Store, q string) int64 {
	t.Helper()
	var n int64
	must(t, s.readers.QueryRowContext(t.Context(), q).Scan(&n))
	return n
}
func TestPostgresSharedZeroWorkspaceOneFetchTwoSubscriptions(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("local PostgreSQL only")
	}
	s, path := fresh(t)
	now := queryEpoch
	s.clock = func() time.Time { return now }
	src, e := s.ConfigurePublicSource(t.Context(), publicDefinition(1))
	must(t, e)
	if pgCount(t, s, "SELECT count(*) FROM tenants") != 0 {
		t.Fatal("public configuration manufactured tenant")
	}
	f := &publicSyntheticFetcher{}
	_, e = s.RunPublicPollOnce(t.Context(), f)
	must(t, e)
	if f.calls.Load() != 1 || pgCount(t, s, "SELECT count(*) FROM public_articles") != 1 || pgCount(t, s, "SELECT count(*) FROM public_article_versions") != 1 {
		t.Fatal("zero workspace acquisition failed")
	}
	now = now.Add(time.Second)
	publicWorkspace(t, s, tenantA, src)
	publicWorkspace(t, s, tenantB, src)
	// Private sources remain separate, and another tenant cannot select them.
	must(t, s.PutArticle(t.Context(), operator(tenantA), item()))
	if pgCount(t, s, "SELECT count(*) FROM public_poll_sources") != 1 || pgCount(t, s, "SELECT count(*) FROM poll_sources") != 0 {
		t.Fatal("subscriptions duplicated acquisition")
	}
	if _, e = s.RunPublicPollOnce(t.Context(), f); !errors.Is(e, ingest.ErrIdle) || f.calls.Load() != 1 {
		t.Fatal("subscription triggered another fetch", e)
	}
	a := runRetrieval(t, s, operator(tenantA), "public-tenant-a-query", recentContext(), 24, 5)
	b := runRetrieval(t, s, operator(tenantB), "public-tenant-b-query", recentContext(), 24, 5)
	if len(a.Items) != 1 || len(b.Items) != 1 || a.Items[0].ArticleID != b.Items[0].ArticleID || a.ID == b.ID {
		t.Fatal("shared retrieval/private snapshots", a, b)
	}
	if a.Items[0].ArticleID == itemID {
		t.Fatal("private item leaked")
	}
	if _, e = s.GetSnapshot(t.Context(), operator(tenantB), a.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("cross tenant snapshot", e)
	}
	event := feedback.Event{EventID: rid(881), SnapshotID: a.ID, ArticleID: a.Items[0].ArticleID, Action: "save"}
	must(t, s.RecordFeedback(t.Context(), operator(tenantA), event))
	must(t, s.RecordFeedback(t.Context(), operator(tenantA), event))
	if e = s.RecordFeedback(t.Context(), operator(tenantB), event); e == nil {
		t.Fatal("feedback crossed tenant")
	}
	if pgCount(t, s, "SELECT count(*) FROM feedback_events") != 1 {
		t.Fatal("feedback replay duplicated")
	}
	// Unsubscribe between admission and completion fences the cache write.
	pending, e := s.BeginQuery(t.Context(), operator(tenantA), "unsubscribe-race-query", request(), policy())
	must(t, e)
	must(t, s.SubscribePublicSource(t.Context(), operator(tenantA), feedID, src, false))
	_, e = s.CompleteQuery(t.Context(), operator(tenantA), pending.WorkID, "deterministic_fallback", []query.Selection{{ArticleID: a.Items[0].ArticleID, ContentHash: a.Items[0].ContentHash, Explanation: "shared evidence"}}, query.Settlement{Known: true})
	if !errors.Is(e, query.ErrConflict) {
		t.Fatal("unsubscribe completion race", e)
	}
	suppressed, e := s.GetSnapshot(t.Context(), operator(tenantA), a.ID)
	must(t, e)
	if len(suppressed.Items) != 0 || !suppressed.Suppressed {
		t.Fatal("unsubscribed snapshot visible")
	}
	must(t, s.Close())
	must(t, CustomerSupport(t.Context(), path, tenantA, "delete", io.Discard))
	s, e = Open(t.Context(), path)
	must(t, e)
	defer s.Close()
	s.clock = func() time.Time { return now }
	if pgCount(t, s, "SELECT count(*) FROM public_articles") != 1 || pgCount(t, s, "SELECT count(*) FROM workspace_public_subscriptions") != 1 || pgCount(t, s, "SELECT count(*) FROM articles") != 0 {
		t.Fatal("deletion destroyed shared data or retained private state")
	}
	kept, e := s.GetSnapshot(t.Context(), operator(tenantB), b.ID)
	must(t, e)
	if len(kept.Items) != 1 {
		t.Fatal("other tenant lost shared access")
	}
	def := publicDefinition(1)
	def.Enabled = false
	_, e = s.ConfigurePublicSource(t.Context(), def)
	must(t, e)
	if _, e = s.GetSnapshot(t.Context(), operator(tenantB), b.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("publisher policy revision did not invalidate snapshot", e)
	}
	now = now.Add(48 * time.Hour)
	if _, e = s.ClaimPublicPoll(t.Context()); !errors.Is(e, ingest.ErrIdle) {
		t.Fatal("disabled publisher acquired", e)
	}
}

func TestPostgresSharedCombinedAdmissionCrashAndPolicyFence(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("local PostgreSQL only")
	}
	s, path := fresh(t)
	now := queryEpoch
	s.clock = func() time.Time { return now }
	src, e := s.ConfigurePublicSource(t.Context(), publicDefinition(1))
	must(t, e)
	second, e := Open(t.Context(), path)
	must(t, e)
	defer second.Close()
	second.clock = s.clock
	var accepted atomic.Int64
	var wg sync.WaitGroup
	var claimed ingest.Claim
	var mu sync.Mutex
	for range 12 {
		wg.Go(func() {
			c, e := second.ClaimPublicPoll(t.Context())
			if e == nil {
				accepted.Add(1)
				mu.Lock()
				claimed = c
				mu.Unlock()
			} else if !errors.Is(e, ingest.ErrBusy) {
				t.Error(e)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatal("duplicate shared acquisition", accepted.Load())
	}
	seed(t, s, tenantA)
	must(t, s.ConfigurePoll(t.Context(), operator(tenantA), ingest.Policy{SourceID: sourceID, URL: "https://example.invalid/feed", Approved: true, Enabled: true, Interval: 24 * time.Hour, MaxBytes: ingest.MaxResponseBytes}))
	if _, e = s.ClaimPoll(t.Context(), operator(tenantA)); !errors.Is(e, ingest.ErrBusy) {
		t.Fatal("private claim bypassed public reservation", e)
	}
	now = now.Add(ingest.LeaseDuration)
	if e = s.FinishPublicPoll(t.Context(), claimed.ID, ingest.Result{Status: 200}); !errors.Is(e, ingest.ErrLease) {
		t.Fatal("stale worker accepted", e)
	}
	private, e := s.ClaimPoll(t.Context(), operator(tenantA))
	must(t, e)
	must(t, s.FinishPoll(t.Context(), operator(tenantA), private.ID, ingest.Result{Status: 503, Failure: "http"}))
	if pgCount(t, s, "SELECT charged_bytes FROM public_poll_attempts LIMIT 1") != ingest.MaxResponseBytes {
		t.Fatal("crash refunded bytes")
	}
	def := publicDefinition(2)
	other, e := s.ConfigurePublicSource(t.Context(), def)
	must(t, e)
	c, e := s.ClaimPublicPoll(t.Context())
	must(t, e)
	if c.SourceID != other {
		t.Fatal("fair due selection", c.SourceID, src)
	}
	def.Enabled = false
	_, e = s.ConfigurePublicSource(t.Context(), def)
	must(t, e)
	if e = s.FinishPublicPoll(t.Context(), c.ID, ingest.Result{Status: 200}); !errors.Is(e, ingest.ErrLease) {
		t.Fatal("policy revocation accepted stale finish", e)
	}
	// Shared response-byte charges consume the same ceiling as private charges.
	now = now.Add(48 * time.Hour)
	def.Enabled = true
	_, e = s.ConfigurePublicSource(t.Context(), def)
	must(t, e)
	_, e = s.writer.ExecContext(t.Context(), `INSERT INTO erased_acquisition_usage(charged_at,charged_bytes) VALUES($1,$2)`, now.UnixMicro(), ingest.DailyBytes)
	must(t, e)
	if _, e = s.ClaimPublicPoll(t.Context()); !errors.Is(e, ingest.ErrBudget) {
		t.Fatal("combined byte cap bypass", e)
	}
}
