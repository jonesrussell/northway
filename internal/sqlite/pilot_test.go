package sqlite

import (
	"errors"
	"testing"
	"time"
)

func TestProvisionPilotIsAtomicAndIdempotent(t *testing.T) {
	s, _ := fresh(t)
	p := operator(tenantA)
	must(t, s.CreateTenant(t.Context(), tenantA))
	feeds := []PilotFeed{
		{ID: "10000000-0000-4000-8000-000000000001", Title: "Development", Categories: []string{"development"}, PublisherCap: 2},
		{ID: "10000000-0000-4000-8000-000000000002", Title: "Mixed", Categories: []string{"development"}, PublisherCap: 2},
	}
	sources := []PilotSource{{
		ID: "20000000-0000-4000-8000-000000000001", URL: "https://example.invalid/feed", Title: "Example",
		FeedIDs: []string{feeds[0].ID, feeds[1].ID}, Interval: 4 * time.Hour, MaxBytes: 2048, PublisherGroup: "example", Categories: []string{"development"},
	}}
	must(t, s.ProvisionPilot(t.Context(), p, sources, feeds))
	var revision int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT revision FROM feeds WHERE tenant_id=? AND id=?", string(tenantA), feeds[0].ID).Scan(&revision))
	if err := s.ProvisionPilot(t.Context(), p, sources, feeds); err != nil {
		t.Fatalf("idempotent provision: %v", err)
	}
	var after int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT revision FROM feeds WHERE tenant_id=? AND id=?", string(tenantA), feeds[0].ID).Scan(&after))
	if after != revision {
		t.Fatalf("idempotent provision changed feed revision: %d -> %d", revision, after)
	}
	must(t, s.DetachSource(t.Context(), p, feeds[0].ID, sources[0].ID))
	if err := s.ProvisionPilot(t.Context(), p, sources, feeds); !errors.Is(err, ErrPilotConflict) {
		t.Fatalf("detached membership silently restored: %v", err)
	}
	claim, err := s.ClaimPoll(t.Context(), p)
	must(t, err)
	if claim.SourceID != sources[0].ID || claim.URL != sources[0].URL || claim.MaxBytes != 2048 {
		t.Fatalf("unexpected claim: %#v", claim)
	}

	bad := append([]PilotFeed(nil), feeds...)
	bad[0].Title = "Changed"
	if err := s.ProvisionPilot(t.Context(), p, sources, bad); err == nil {
		t.Fatal("conflicting reviewed feed accepted")
	}
	if _, err := s.GetFeed(t.Context(), p, feeds[0].ID); err != nil {
		t.Fatalf("original feed lost: %v", err)
	}
}
