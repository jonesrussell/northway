package sqlite

import (
	"errors"
	"fmt"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/query"
	"os"
	"testing"
	"time"
)

func TestPostgresReviewedRegisterDisabledReplayAndCombinedSourceCap(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("local PostgreSQL only")
	}
	s, _ := fresh(t)
	for range 2 {
		n, e := s.InstallReviewedPublicRegister(t.Context())
		must(t, e)
		if n != 88 {
			t.Fatal(n)
		}
	}
	for sql, want := range map[string]int64{
		"SELECT count(*) FROM public_sources": 88, "SELECT count(*) FROM public_poll_sources": 88,
		"SELECT count(*) FROM public_sources WHERE onboarding=1":                                                         10,
		"SELECT count(*) FROM public_sources WHERE enabled=1 OR rights_basis!='not_assessed' OR review_state!='pending'": 0,
		"SELECT count(*) FROM tenants":                    0,
		"SELECT count(*) FROM public_register_exclusions": 8,
	} {
		if pgCount(t, s, sql) != want {
			t.Fatal(sql, pgCount(t, s, sql), want)
		}
	}
	if _, e := s.ClaimPublicPoll(t.Context()); !errors.Is(e, ingest.ErrIdle) {
		t.Fatal("register activated feeds", e)
	}
	for n := 0; n < 12; n++ {
		_, e := s.ConfigurePublicSource(t.Context(), publicDefinition(100+n))
		must(t, e)
	}
	if _, e := s.ConfigurePublicSource(t.Context(), publicDefinition(999)); !errors.Is(e, ingest.ErrBudget) {
		t.Fatal("100 source cap bypass", e)
	}
	if pgCount(t, s, "SELECT count(*) FROM public_poll_sources") != 100 {
		t.Fatal("cap failure partially installed")
	}
	seed(t, s, tenantA)
	e := s.ConfigurePoll(t.Context(), operator(tenantA), ingest.Policy{SourceID: sourceID, URL: "https://example.invalid/feed", Approved: true, Enabled: true, Interval: 24 * time.Hour, MaxBytes: ingest.MaxResponseBytes})
	if !errors.Is(e, ingest.ErrBudget) {
		t.Fatal("private source bypassed shared cap", e)
	}
}

func TestPostgresSyntheticTenSourceCanaryAtZeroWorkspaces(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("local PostgreSQL only")
	}
	s, _ := fresh(t)
	now := queryEpoch
	s.clock = func() time.Time { return now }
	for n := 0; n < 10; n++ {
		_, e := s.ConfigurePublicSource(t.Context(), publicDefinition(n))
		must(t, e)
	}
	f := &publicSyntheticFetcher{}
	for range 10 {
		_, e := s.RunPublicPollOnce(t.Context(), f)
		must(t, e)
		now = now.Add(time.Second)
	}
	if f.calls.Load() != 10 || pgCount(t, s, "SELECT count(*) FROM public_articles") != 10 || pgCount(t, s, "SELECT count(*) FROM public_article_versions") != 10 || pgCount(t, s, "SELECT count(*) FROM tenants") != 0 {
		t.Fatal("zero workspace synthetic canary failed")
	}
	if _, e := s.ClaimPublicPoll(t.Context()); !errors.Is(e, ingest.ErrIdle) {
		t.Fatal("canary polled twice", e)
	}
}

func TestPostgresSharedCorpusChangeCacheAndCompletionFence(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("local PostgreSQL only")
	}
	s, _ := fresh(t)
	now := queryEpoch
	s.clock = func() time.Time { return now }
	src, e := s.ConfigurePublicSource(t.Context(), publicDefinition(1))
	must(t, e)
	f := &publicSyntheticFetcher{}
	_, e = s.RunPublicPollOnce(t.Context(), f)
	must(t, e)
	now = now.Add(time.Second)
	publicWorkspace(t, s, tenantA, src)
	a := runRetrieval(t, s, operator(tenantA), "public-initial-cache-key", recentContext(), 720, 5)
	cached := runRetrieval(t, s, operator(tenantA), "public-second-cache-key", recentContext(), 720, 5)
	if cached.ID != a.ID {
		t.Fatal("shared cache reuse failed")
	}
	now = now.Add(24 * time.Hour)
	c, e := s.ClaimPublicPoll(t.Context())
	must(t, e)
	pending, e := s.BeginQuery(t.Context(), operator(tenantA), "public-update-race-key", request(), policy())
	must(t, e)
	must(t, s.FinishPublicPoll(t.Context(), c.ID, ingest.Result{Status: 200, Bytes: 512, ETag: "synthetic-v2", NotBefore: now.Add(48 * time.Hour), Items: []ingest.Item{{OriginID: "shared-origin", URL: "https://publisher.invalid/article", Title: "Shared revised headline"}}}))
	if _, e = s.GetSnapshot(t.Context(), operator(tenantA), a.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("changed public corpus retained stale snapshot", e)
	}
	_, e = s.CompleteQuery(t.Context(), operator(tenantA), pending.WorkID, "deterministic_fallback", []query.Selection{}, query.Settlement{Known: true})
	if !errors.Is(e, query.ErrConflict) {
		t.Fatal("public update accepted stale completion", e)
	}
	if pgCount(t, s, "SELECT count(*) FROM public_article_versions") != 2 || pgCount(t, s, "SELECT count(*) FROM public_articles") != 1 {
		t.Fatal("shared version identity changed")
	}
	next := runRetrieval(t, s, operator(tenantA), "public-updated-cache-key", recentContext(), 720, 5)
	if next.ID == a.ID || len(next.Items) != 1 || next.Items[0].Title != "Shared revised headline" {
		t.Fatal("public cache not invalidated", next)
	}
	now = now.Add(24 * time.Hour)
	if _, e = s.ClaimPublicPoll(t.Context()); !errors.Is(e, ingest.ErrIdle) {
		t.Fatal("Retry-After/cache hold ignored", e)
	}
}

func TestPostgresSharedOnboardingAtomicRetryAndSuspension(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("local PostgreSQL only")
	}
	s, _ := fresh(t)
	def := publicDefinition(1)
	def.Onboarding = true
	src, e := s.ConfigurePublicSource(t.Context(), def)
	must(t, e)
	for range 2 {
		must(t, s.ensurePublicWorkspace(t.Context(), tenantA))
	}
	if pgCount(t, s, "SELECT count(*) FROM workspace_public_subscriptions") != 1 || pgCount(t, s, "SELECT count(*) FROM sources") != 0 || pgCount(t, s, "SELECT count(*) FROM poll_sources") != 0 {
		t.Fatal("onboarding copied corpus/acquisition")
	}
	// A deterministic database failure during membership creation rolls back the
	// newly created workspace, so a retry cannot inherit a half-created account.
	_, e = s.writer.ExecContext(t.Context(), `CREATE FUNCTION synthetic_membership_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic failure'; END $$; CREATE TRIGGER synthetic_failure BEFORE INSERT ON workspace_public_subscriptions FOR EACH ROW EXECUTE FUNCTION synthetic_membership_failure()`)
	must(t, e)
	if e = s.ensurePublicWorkspace(t.Context(), tenantB); e == nil {
		t.Fatal("failed membership created workspace")
	}
	if pgCount(t, s, fmt.Sprintf("SELECT count(*) FROM tenants WHERE id='%s'", tenantB)) != 0 {
		t.Fatal("partial workspace persisted")
	}
	_, e = s.writer.ExecContext(t.Context(), `DROP TRIGGER synthetic_failure ON workspace_public_subscriptions; DROP FUNCTION synthetic_membership_failure()`)
	must(t, e)
	must(t, s.ensurePublicWorkspace(t.Context(), tenantB))
	_, e = s.writer.ExecContext(t.Context(), `UPDATE customer_workspaces SET state='suspended' WHERE tenant_id=$1`, string(tenantA))
	must(t, e)
	if pgCount(t, s, "SELECT count(*) FROM workspace_public_subscriptions") != 1 || pgCount(t, s, "SELECT count(*) FROM public_sources") != 1 {
		t.Fatal("suspension damaged another subscription")
	}
	if e = s.SubscribePublicSource(t.Context(), operator(tenantA), PublicFeedID("world"), src, true); !errors.Is(e, ErrNotFound) {
		t.Fatal("suspended workspace subscribed", e)
	}
}
