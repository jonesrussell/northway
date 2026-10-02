package sqlite

import (
	"github.com/jonesrussell/northway/internal/ingest"
	"os"
	"testing"
	"time"
)

func TestPostgresPublicActivationAtomicReplayAndDenial(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("local PostgreSQL only")
	}
	s, _ := fresh(t)
	_, e := s.InstallReviewedPublicRegister(t.Context())
	must(t, e)
	if _, e = s.ActivatePublicCanary(t.Context(), ""); e == nil {
		t.Fatal("missing approval accepted")
	}
	for range 2 {
		n, e := s.ActivatePublicCanary(t.Context(), "synthetic-owner-approval")
		must(t, e)
		if n != 10 {
			t.Fatal(n)
		}
	}
	if pgCount(t, s, "SELECT count(*) FROM public_sources WHERE enabled=1") != 10 || pgCount(t, s, "SELECT count(*) FROM tenants") != 0 {
		t.Fatal("activation copied ownership")
	}
	healthy, e := s.PublicPollHealthy(t.Context())
	must(t, e)
	if healthy {
		t.Fatal("unfetched corpus healthy")
	}
	claim, e := s.ClaimPublicPoll(t.Context())
	must(t, e)
	must(t, s.FinishPublicPoll(t.Context(), claim.ID, ingest.Result{Status: 403, Failure: "http"}))
	if pgCount(t, s, "SELECT count(*) FROM public_sources WHERE enabled=1") != 9 {
		t.Fatal("denied source remained enabled")
	}
	if _, e = s.ActivatePublicCanary(t.Context(), "synthetic-owner-approval"); e == nil {
		t.Fatal("replay revived denied source")
	}
	now := s.queryTime()
	s.clock = func() time.Time { return now.Add(48 * time.Hour) }
	for range 9 {
		c, e := s.ClaimPublicPoll(t.Context())
		must(t, e)
		if c.SourceID == claim.SourceID {
			t.Fatal("denial retried")
		}
		must(t, s.FinishPublicPoll(t.Context(), c.ID, ingest.Result{Status: 200}))
	}
	if pgCount(t, s, "SELECT count(*) FROM public_poll_attempts") != 10 {
		t.Fatal("extra acquisition")
	}
}

func TestPostgresPublicActivationConflictHasNoPartialWrites(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("local PostgreSQL only")
	}
	s, _ := fresh(t)
	_, e := s.InstallReviewedPublicRegister(t.Context())
	must(t, e)
	_, e = s.writer.ExecContext(t.Context(), "UPDATE public_sources SET review_state='excluded' WHERE id=(SELECT id FROM public_sources WHERE onboarding=1 ORDER BY id DESC LIMIT 1)")
	must(t, e)
	if _, e = s.ActivatePublicCanary(t.Context(), "synthetic"); e == nil {
		t.Fatal("conflict ignored")
	}
	if pgCount(t, s, "SELECT count(*) FROM public_sources WHERE enabled=1") != 0 {
		t.Fatal("partial activation")
	}
}
