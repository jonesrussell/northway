package sqlite

import (
	"errors"
	"github.com/jonesrussell/northway/internal/ingest"
	"os"
	"testing"
	"time"
)

func TestPostgresPublicCanonicalIdentityReplayAndHostHold(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("local PostgreSQL only")
	}
	s, _ := fresh(t)
	now := queryEpoch
	s.clock = func() time.Time { return now }
	def := publicDefinition(1)
	def.URL = "https://Publisher.invalid:443/feed"
	src, e := s.ConfigurePublicSource(t.Context(), def)
	must(t, e)
	if src != PublicSourceID("https://publisher.invalid/feed") {
		t.Fatal("canonical identity split")
	}
	c, e := s.ClaimPublicPoll(t.Context())
	must(t, e)
	_, e = s.ConfigurePublicSource(t.Context(), def)
	must(t, e)
	// Identical provisioning must neither fence an active worker nor delay a due
	// source. Failed bytes remain reserved, with a publisher-wide Retry-After hold.
	must(t, s.FinishPublicPoll(t.Context(), c.ID, ingest.Result{Status: 429, Failure: "http", NotBefore: now.Add(48 * time.Hour)}))
	def.URL = "https://publisher.invalid/another-feed"
	_, e = s.ConfigurePublicSource(t.Context(), def)
	must(t, e)
	if _, e = s.ClaimPublicPoll(t.Context()); !errors.Is(e, ingest.ErrIdle) {
		t.Fatal("same-host hold ignored", e)
	}
	if pgCount(t, s, "SELECT charged_bytes FROM public_poll_attempts LIMIT 1") != ingest.MaxResponseBytes {
		t.Fatal("failure refunded reservation")
	}
	now = now.Add(48 * time.Hour)
	if _, e = s.ClaimPublicPoll(t.Context()); e != nil {
		t.Fatal("host hold never released", e)
	}
}

func TestPostgresPublicAttemptsSharePrivateCeiling(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("local PostgreSQL only")
	}
	s, _ := fresh(t)
	now := queryEpoch
	s.clock = func() time.Time { return now }
	_, e := s.ConfigurePublicSource(t.Context(), publicDefinition(1))
	must(t, e)
	_, e = s.writer.ExecContext(t.Context(), `INSERT INTO erased_acquisition_usage(charged_at,charged_bytes) SELECT $1::bigint,0 FROM generate_series(1,180)`, now.UnixMicro())
	must(t, e)
	if _, e = s.ClaimPublicPoll(t.Context()); !errors.Is(e, ingest.ErrBudget) {
		t.Fatal("public bypassed private attempt cap", e)
	}
	now = now.Add(24*time.Hour + time.Second)
	if _, e = s.ClaimPublicPoll(t.Context()); e != nil {
		t.Fatal("rolling attempts did not release", e)
	}
}
