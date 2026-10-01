package sqlite

import (
	"context"
	"fmt"
	"github.com/jonesrussell/northway/internal/catalogue"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"sort"
	"time"
)

func catalogueProfile(m catalogue.Manifest) ([]PilotSource, []PilotFeed, error) {
	if err := m.Validate(); err != nil {
		return nil, nil, err
	}
	entries := append([]catalogue.Entry(nil), m.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].URL < entries[j].URL })
	var sources []PilotSource
	var feeds []PilotFeed
	topics := map[string]bool{}
	interval := time.Duration(m.IntervalSeconds) * time.Second
	for _, e := range entries {
		if !e.Selected {
			continue
		}
		fid := catalogue.ID("feed", e.Topic)
		cat := catalogue.Category(e.Topic)
		if !topics[e.Topic] {
			topics[e.Topic] = true
			feeds = append(feeds, PilotFeed{ID: fid, Title: "Curated " + e.Topic + " links", Categories: []string{cat}, PublisherCap: 2, UseContext: true})
		}
		sources = append(sources, PilotSource{ID: catalogue.ID("source", e.URL), URL: e.URL, Title: fmt.Sprintf("%s | %s", e.Title, e.Publisher), PublisherGroup: catalogue.PublisherGroup(e.Publisher), Categories: []string{cat}, FeedIDs: []string{fid}, Interval: interval, MaxBytes: m.MaxBytes, Disabled: !m.Enabled, Activate: m.Enabled})
	}
	// Deterministic spread across the first interval; retries preserve existing schedules.
	for i := range sources {
		sources[i].InitialDelay = time.Duration(i) * interval / time.Duration(len(sources))
	}
	return sources, feeds, nil
}

// ProvisionCuratedCatalogue is offline operator admission. The complete immutable
// register remains the external policy/rights record; corpus stores source attribution.
func (s *Store) ProvisionCuratedCatalogue(ctx context.Context, p identity.Principal, m catalogue.Manifest) error {
	if _, e := p.RequireOperator(); e != nil {
		return e
	}
	sources, feeds, e := catalogueProfile(m)
	if e != nil {
		return e
	}
	return s.ProvisionPilot(ctx, p, sources, feeds)
}
func (s *Store) ProvisionCustomerCuratedCatalogue(ctx context.Context, p identity.Principal, m catalogue.Manifest) error {
	tenant, e := p.RequireManagement()
	if e != nil {
		return e
	}
	if e = requireWorkspace(ctx, sqlc.New(s.readers), tenant); e != nil {
		return e
	}
	sources, feeds, e := catalogueProfile(m)
	if e != nil {
		return e
	}
	return s.provisionProfile(ctx, tenant, sources, feeds)
}

type CatalogueInventory struct {
	GlobalPollSources int64                      `json:"global_poll_sources"`
	TenantSources     []sqlc.CatalogueSourcesRow `json:"tenant_sources"`
}

func (s *Store) CatalogueInventory(ctx context.Context, p identity.Principal) (CatalogueInventory, error) {
	tenant, e := p.RequireOperator()
	if e != nil {
		return CatalogueInventory{}, e
	}
	q := sqlc.New(s.readers)
	n, e := q.PollSourceCount(ctx)
	if e != nil {
		return CatalogueInventory{}, e
	}
	rows, e := q.CatalogueSources(ctx, string(tenant))
	return CatalogueInventory{n, rows}, e
}
