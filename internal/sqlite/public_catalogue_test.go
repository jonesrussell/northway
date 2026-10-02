package sqlite

import (
	"errors"
	"fmt"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/source"
	"os"
	"testing"
	"time"
)

func TestPostgresCompleteCatalogueUpgradeReplayAndZeroWorkspaceAcquisition(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("disposable PostgreSQL required")
	}
	s, _ := fresh(t)
	now := queryEpoch
	s.clock = func() time.Time { return now }
	_, e := s.InstallReviewedPublicRegister(t.Context())
	must(t, e)
	_, e = s.ActivatePublicCanary(t.Context(), "earlier-owner-record")
	must(t, e)
	n, e := s.ActivatePublicCatalogue(t.Context(), "complete-owner-record")
	must(t, e)
	if n != 88 || pgCount(t, s, "SELECT count(*) FROM public_sources WHERE enabled=1 AND onboarding=1") != 88 || pgCount(t, s, "SELECT count(DISTINCT topic) FROM public_sources") != 20 {
		t.Fatal("incomplete catalogue")
	}
	if pgCount(t, s, "SELECT count(*) FROM public_sources WHERE provenance LIKE '%activation=earlier-owner-record'") != 10 {
		t.Fatal("existing approval overwritten")
	}
	revisions := pgCount(t, s, "SELECT sum(policy_revision) FROM public_sources")
	_, e = s.ActivatePublicCatalogue(t.Context(), "repeated-owner-record")
	must(t, e)
	if pgCount(t, s, "SELECT sum(policy_revision) FROM public_sources") != revisions {
		t.Fatal("replay changed revisions")
	}
	for range 88 {
		claim, e := s.ClaimPublicPoll(t.Context())
		must(t, e)
		must(t, s.FinishPublicPoll(t.Context(), claim.ID, ingest.Result{Status: 200, Bytes: 128, Items: []ingest.Item{{OriginID: "synthetic", URL: "https://synthetic.invalid/article", Title: "Synthetic attributed title"}}}))
		now = now.Add(time.Minute)
	}
	if pgCount(t, s, "SELECT count(*) FROM public_articles") != 88 || pgCount(t, s, "SELECT count(*) FROM public_poll_attempts") != 88 || pgCount(t, s, "SELECT count(*) FROM tenants") != 0 {
		t.Fatal("acquisition duplicated or required workspace")
	}
	if _, e = s.ClaimPublicPoll(t.Context()); !errors.Is(e, ingest.ErrIdle) {
		t.Fatal("polled catalogue twice", e)
	}
	statuses, e := s.PublicCatalogueStatus(t.Context())
	must(t, e)
	if len(statuses) != 88 {
		t.Fatal("incomplete source outcome report")
	}
	for _, status := range statuses {
		if !status.Enabled || !status.Onboarding || status.LastStatus != 200 || status.LastSuccess == 0 || status.Articles != 1 {
			t.Fatal("inaccurate source outcome", status)
		}
	}

	for _, tenant := range []identity.TenantID{tenantA, tenantB} {
		must(t, s.ensurePublicWorkspace(t.Context(), tenant))
		must(t, s.ensurePublicWorkspace(t.Context(), tenant))
	}
	if pgCount(t, s, "SELECT count(*) FROM workspace_public_subscriptions") != 176 || pgCount(t, s, "SELECT count(*) FROM feeds") != 40 || pgCount(t, s, "SELECT count(*) FROM poll_sources") != 0 {
		t.Fatal("onboarding copied acquisition or omitted catalogue")
	}

	healthy, e := s.PublicPollHealthy(t.Context())
	must(t, e)
	if !healthy {
		t.Fatal("completed catalogue unhealthy")
	}
}

func TestPostgresCompleteCatalogueConflictAtomicityAndDeniedReplay(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("disposable PostgreSQL required")
	}
	s, _ := fresh(t)
	_, e := s.InstallReviewedPublicRegister(t.Context())
	must(t, e)
	_, e = s.writer.ExecContext(t.Context(), "UPDATE public_sources SET review_state='excluded' WHERE id=(SELECT id FROM public_sources WHERE onboarding=0 ORDER BY id DESC LIMIT 1)")
	must(t, e)
	if _, e = s.ActivatePublicCatalogue(t.Context(), "owner-record"); e == nil {
		t.Fatal("excluded source accepted")
	}
	if pgCount(t, s, "SELECT count(*) FROM public_sources WHERE enabled=1") != 0 {
		t.Fatal("partial activation")
	}
	_, e = s.writer.ExecContext(t.Context(), "UPDATE public_sources SET review_state='pending' WHERE review_state='excluded'")
	must(t, e)
	_, e = s.ActivatePublicCatalogue(t.Context(), "owner-record")
	must(t, e)
	c, e := s.ClaimPublicPoll(t.Context())
	must(t, e)
	must(t, s.FinishPublicPoll(t.Context(), c.ID, ingest.Result{Status: 403, Failure: "http"}))
	if _, e = s.ActivatePublicCatalogue(t.Context(), "owner-record"); e == nil {
		t.Fatal("denial revived")
	}
	if pgCount(t, s, "SELECT count(*) FROM public_sources WHERE enabled=1") != 87 {
		t.Fatal("denial changed unrelated sources")
	}
}

func TestPostgresCompleteCatalogueCapacityAtomicity(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("disposable PostgreSQL required")
	}
	s, _ := fresh(t)
	_, e := s.InstallReviewedPublicRegister(t.Context())
	must(t, e)
	seed(t, s, tenantA)
	for n := range 4 {
		id := fmt.Sprintf("00000100-0000-4000-8000-%012d", n)
		url := fmt.Sprintf("https://capacity%d.invalid/feed", n)
		must(t, s.CreateSource(t.Context(), operator(tenantA), source.Source{ID: id, URL: url, Title: "Synthetic capacity fixture"}))
		must(t, s.ConfigurePoll(t.Context(), operator(tenantA), ingest.Policy{SourceID: id, URL: url, Approved: true, Enabled: true, Interval: time.Hour, MaxBytes: ingest.MaxResponseBytes}))
	}
	if _, e = s.ActivatePublicCatalogue(t.Context(), "owner-record"); e == nil {
		t.Fatal("combined daily ceiling exceeded")
	}
	if pgCount(t, s, "SELECT count(*) FROM public_sources WHERE enabled=1") != 0 {
		t.Fatal("capacity failure partially activated")
	}
}
