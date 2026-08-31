package sqlite

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jonesrussell/northway/internal/query"
)

func TestQueryUnrelatedCorpusChangeKeepsPaidResultUncached(t *testing.T) {
	s, _, p, sel := queryFixture(t)
	c := claim(t, s, p, "paid-overlapping-poll")
	must(t, s.StartProvider(t.Context(), p, c.WorkID))
	a := item()
	a.ID = "00000009-0000-4000-8000-000000000000"
	a.OriginID = "another"
	a.Title = "A newly arrived article"
	must(t, s.PutArticle(t.Context(), operator(tenantA), a))
	snap := complete(t, s, p, c, sel, query.Settlement{Known: true, ActualMicros: 11})
	budget(t, s, 11, 0)
	if claim(t, s, p, "paid-overlapping-poll").Snapshot.ID != snap.ID {
		t.Fatal("paid result not replayable")
	}
	if claim(t, s, p, "new-corpus-query-key").Snapshot != nil {
		t.Fatal("old result stamped with unseen corpus revision")
	}
}

func TestQueryIdenticalArticleWritePreservesCache(t *testing.T) {
	s, _, p, sel := queryFixture(t)
	c := claim(t, s, p, "unchanged-write-original")
	must(t, s.StartProvider(t.Context(), p, c.WorkID))
	snap := complete(t, s, p, c, sel, query.Settlement{Known: true, ActualMicros: 3})
	must(t, s.PutArticle(t.Context(), operator(tenantA), item()))
	if hit := claim(t, s, p, "identical-write-cache"); hit.Snapshot == nil || hit.Snapshot.ID != snap.ID {
		t.Fatal("identical article invalidated cache")
	}
	// Actual observation-time changes affect age filtering and must invalidate.
	a := item()
	a.ObservedAt = queryEpoch
	must(t, s.PutArticle(t.Context(), operator(tenantA), a))
	if claim(t, s, p, "changed-observation-key").Snapshot != nil {
		t.Fatal("changed age semantics failed to invalidate")
	}
}

func TestQueryWorstCaseEncodedSnapshotFits(t *testing.T) {
	s, _, p, _ := queryFixture(t)
	selections := make([]query.Selection, 0, 20)
	for i := range 20 {
		a := item()
		a.ID = fmt.Sprintf("00000009-0000-4000-8000-%012d", i)
		a.OriginID = fmt.Sprintf("sized-%d", i)
		// Each byte/rune is legal, but json.Marshal expands it sixfold.
		a.Title = strings.Repeat("<", 512)
		a.URL = "https://example.invalid/" + strings.Repeat("&", 2024)
		must(t, s.PutArticle(t.Context(), operator(tenantA), a))
		stored, err := s.GetArticle(t.Context(), p, a.ID)
		must(t, err)
		selections = append(selections, query.Selection{ArticleID: a.ID, ContentHash: stored.ContentHash, Explanation: strings.Repeat("\x01", 1000)})
	}
	r := request()
	r.Limit = 20
	c, err := s.BeginQuery(t.Context(), p, "max-encoded-snapshot", r, policy())
	must(t, err)
	must(t, s.StartProvider(t.Context(), p, c.WorkID))
	snap, err := s.CompleteQuery(t.Context(), p, c.WorkID, "ai", selections, query.Settlement{Known: true, ActualMicros: 21})
	must(t, err)
	budget(t, s, 21, 0)
	got, err := s.GetSnapshot(t.Context(), p, snap.ID)
	must(t, err)
	if len(got.Items) != 20 || got.Items[19].Explanation != selections[19].Explanation {
		t.Fatal("maximum snapshot did not round-trip")
	}
}

func TestQueryFallbackCacheSurvivesBudgetIncrease(t *testing.T) {
	s, _, p, sel := queryFixture(t)
	must(t, s.SetBudget(t.Context(), operator(tenantA), 0))
	c := claim(t, s, p, "fallback-before-funding")
	if c.ProviderAllowed {
		t.Fatal("unfunded provider permitted")
	}
	snap, err := s.CompleteQuery(t.Context(), p, c.WorkID, "deterministic_fallback", []query.Selection{sel}, query.Settlement{Known: true})
	must(t, err)
	must(t, s.SetBudget(t.Context(), operator(tenantA), 100))
	hit := claim(t, s, p, "funded-cache-reuse-key")
	if hit.Snapshot == nil || hit.Snapshot.ID != snap.ID || hit.Snapshot.Mode != "deterministic_fallback" || hit.ProviderAllowed {
		t.Fatal("funding silently forced paid refresh")
	}
	budget(t, s, 0, 0)
}
