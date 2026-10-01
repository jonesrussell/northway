package sqlite

import (
	"errors"
	"github.com/jonesrussell/northway/internal/catalogue"
	"github.com/jonesrussell/northway/internal/ingest"
	"testing"
	"time"
)

func curated(n int) catalogue.Manifest {
	m := catalogue.Manifest{SchemaVersion: 1, Profile: "curated-v1", ResearchSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", IntervalSeconds: 86400, MaxBytes: 2048}
	for i := 0; i < n; i++ {
		u := "https://" + catalogue.ID("test", string(rune('a'+i))) + ".example/feed"
		m.Entries = append(m.Entries, catalogue.Entry{Candidate: catalogue.Candidate{URL: u, Title: u, Publisher: u, Topic: "science", Region: "Global", Language: "en", Rights: "not_assessed", Notes: "No rehosting rights assumed"}, Selected: true})
	}
	return m
}
func TestCuratedDisabledActivationIdempotenceAndTenantIsolation(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.CreateTenant(t.Context(), tenantA))
	must(t, s.CreateTenant(t.Context(), tenantB))
	p := operator(tenantA)
	m := curated(3)
	must(t, s.ProvisionCuratedCatalogue(t.Context(), p, m))
	must(t, s.ProvisionCuratedCatalogue(t.Context(), p, m))
	if _, e := s.ClaimPoll(t.Context(), p); !errors.Is(e, ingest.ErrIdle) {
		t.Fatalf("disabled source fetched: %v", e)
	}
	inv, e := s.CatalogueInventory(t.Context(), operator(tenantB))
	must(t, e)
	if len(inv.TenantSources) != 0 || inv.GlobalPollSources != 3 {
		t.Fatalf("%+v", inv)
	}
	m.Enabled = true
	m.ApprovalRecord = "review-fixture"
	for i := range m.Entries {
		m.Entries[i].MetadataBasis = "Explicit metadata-only fixture approval"
	}
	must(t, s.ProvisionCuratedCatalogue(t.Context(), p, m))
	must(t, s.ProvisionCuratedCatalogue(t.Context(), p, m))
	inv, e = s.CatalogueInventory(t.Context(), p)
	must(t, e)
	seen := map[int64]bool{}
	for _, r := range inv.TenantSources {
		if r.Approved != int64(1) || r.PollEnabled != int64(1) {
			t.Fatalf("%+v", r)
		}
		seen[r.NextAt] = true
	}
	if len(seen) != 3 {
		t.Fatal("first polls not staggered")
	}
	if e = s.ProvisionCuratedCatalogue(t.Context(), operator(tenantB), curated(1)); e != nil {
		t.Fatal(e)
	}
	// Activated policies cannot silently be reverted by an older disabled register.
	m.Enabled = false
	if e = s.ProvisionCuratedCatalogue(t.Context(), p, m); !errors.Is(e, ErrPilotConflict) {
		t.Fatalf("stale policy adopted: %v", e)
	}
}
func TestCuratedCapacityRollsBackAndDuplicateURLRejected(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.CreateTenant(t.Context(), tenantA))
	p := operator(tenantA)
	m := curated(100)
	must(t, s.ProvisionCuratedCatalogue(t.Context(), p, m))
	extra := curated(1)
	extra.Entries[0].URL = "https://extra.example/feed"
	extra.Entries[0].Topic = "arts"
	if e := s.ProvisionCuratedCatalogue(t.Context(), p, extra); !errors.Is(e, ingest.ErrBudget) {
		t.Fatalf("cap exceeded: %v", e)
	}
	inv, e := s.CatalogueInventory(t.Context(), p)
	must(t, e)
	if inv.GlobalPollSources != 100 || len(inv.TenantSources) != 100 {
		t.Fatalf("partial writes: %+v", inv)
	}
	// Shared provisioning rejects another source ID for the same URL, transactionally.
	source := PilotSource{ID: catalogue.ID("other", "duplicate"), URL: m.Entries[0].URL, Title: "Duplicate", PublisherGroup: "other", Categories: []string{"world"}, FeedIDs: []string{catalogue.ID("other", "feed")}, Interval: 24 * time.Hour, MaxBytes: 2048}
	e = s.ProvisionPilot(t.Context(), p, []PilotSource{source}, []PilotFeed{{ID: source.FeedIDs[0], Title: "Duplicate", Categories: []string{"world"}, PublisherCap: 2}})
	if !errors.Is(e, ErrPilotConflict) {
		t.Fatalf("URL duplicate accepted: %v", e)
	}
}

func TestCuratedCannotAdoptHTMLPolicy(t *testing.T) {
	for _, enabled := range []int{0, 1} {
		t.Run(string(rune('0'+enabled)), func(t *testing.T) {
			s, _ := fresh(t)
			must(t, s.CreateTenant(t.Context(), tenantA))
			p := operator(tenantA)
			m := curated(1)
			must(t, s.ProvisionCuratedCatalogue(t.Context(), p, m))
			_, e := s.writer.ExecContext(t.Context(), "UPDATE poll_sources SET mode='html',preview_allowed=1,robots_until=123,approved=?,enabled=?", enabled, enabled)
			must(t, e)
			m.Enabled = true
			m.ApprovalRecord = "review-fixture"
			m.Entries[0].MetadataBasis = "metadata-only"
			if e = s.ProvisionCuratedCatalogue(t.Context(), p, m); !errors.Is(e, ErrPilotConflict) {
				t.Fatalf("HTML policy adopted: %v", e)
			}
			var mode string
			var approved, preview, robots int
			must(t, s.readers.QueryRowContext(t.Context(), "SELECT mode,approved,preview_allowed,robots_until FROM poll_sources").Scan(&mode, &approved, &preview, &robots))
			if mode != "html" || approved != enabled || preview != 1 || robots != 123 {
				t.Fatal("HTML policy changed despite rejection")
			}
		})
	}
}
