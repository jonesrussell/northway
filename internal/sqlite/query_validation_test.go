package sqlite

import (
	"errors"
	"testing"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
)

func TestQueryScopeGuardsEveryEntryPoint(t *testing.T) {
	s, _, p, sel := queryFixture(t)
	c := claim(t, s, p, "scope-guard-query-key")
	_, secret := newKey(t, s, tenantA, identity.FeedbackWrite)
	feedback, err := identity.NewService(s).Authenticate(t.Context(), secret.Reveal())
	must(t, err)
	for _, bad := range []identity.Principal{{}, feedback} {
		checks := []func() error{
			func() error {
				_, err := s.BeginQuery(t.Context(), bad, "scope-guard-query-key", request(), policy())
				return err
			},
			func() error { return s.StartProvider(t.Context(), bad, c.WorkID) },
			func() error {
				_, err := s.CompleteQuery(t.Context(), bad, c.WorkID, "ai", []query.Selection{sel}, query.Settlement{Known: true})
				return err
			},
			func() error { _, err := s.GetSnapshot(t.Context(), bad, c.WorkID); return err },
			func() error { return s.FailQuery(t.Context(), bad, c.WorkID) },
			func() error { _, err := s.RecoverQueries(t.Context(), bad); return err },
			func() error { return s.ReconcileQuery(t.Context(), bad, c.WorkID, 0) },
			func() error { return s.SetBudget(t.Context(), bad, 1000) },
			func() error { _, err := s.GetBudget(t.Context(), bad); return err },
			func() error { return s.SetSourceEnabled(t.Context(), bad, sourceID, false) },
			func() error { return s.SetFeedEnabled(t.Context(), bad, feedID, false) },
			func() error { return s.DetachSource(t.Context(), bad, feedID, sourceID) },
		}
		for i, check := range checks {
			if err := check(); !errors.Is(err, identity.ErrUnauthorized) && !errors.Is(err, identity.ErrForbidden) {
				t.Fatalf("entry %d missing auth: %v", i, err)
			}
		}
	}
	if err := s.SetBudget(t.Context(), p, 1000); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("reader can increase spend", err)
	}
	if _, err := s.RecoverQueries(t.Context(), p); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("reader can recover other work", err)
	}
	budget(t, s, 0, 40)
}

func TestQueryCandidateValidationAndRevisionFencing(t *testing.T) {
	s, _, p, sel := queryFixture(t)
	c := claim(t, s, p, "candidate-validation-key")
	if _, err := s.CompleteQuery(t.Context(), p, c.WorkID, "ai", []query.Selection{sel}, query.Settlement{Known: true}); !errors.Is(err, query.ErrConflict) {
		t.Fatal("AI completion without provider start", err)
	}
	must(t, s.StartProvider(t.Context(), p, c.WorkID))
	badHash := sel
	badHash.ContentHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	badID := sel
	badID.ArticleID = "00000009-0000-4000-8000-000000000000"
	for _, selections := range [][]query.Selection{{sel, sel}, {badHash}, {badID}} {
		if _, err := s.CompleteQuery(t.Context(), p, c.WorkID, "ai", selections, query.Settlement{Known: true}); err == nil {
			t.Fatal("invalid selection accepted")
		}
	}
	for _, cost := range []query.Settlement{{Known: true, ActualMicros: -1}, {Known: true, ActualMicros: 41}, {ActualMicros: 1}} {
		if _, err := s.CompleteQuery(t.Context(), p, c.WorkID, "ai", []query.Selection{sel}, cost); err == nil {
			t.Fatal("invalid usage accepted")
		}
	}
	budget(t, s, 0, 40)
	must(t, s.SetSourceEnabled(t.Context(), operator(tenantA), sourceID, false))
	if _, err := s.CompleteQuery(t.Context(), p, c.WorkID, "ai", []query.Selection{sel}, query.Settlement{Known: true}); err == nil {
		t.Fatal("revoked selection accepted")
	}
	if _, err := s.GetArticle(t.Context(), p, itemID); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked source through article read", err)
	}
	rows, err := s.Search(t.Context(), p, feedID, "PHP", time.Unix(0, 0), 5)
	must(t, err)
	if len(rows) != 0 {
		t.Fatal("revoked source through FTS")
	}
	must(t, s.FailQuery(t.Context(), p, c.WorkID))
	budget(t, s, 0, 40)
	must(t, s.SetSourceEnabled(t.Context(), operator(tenantA), sourceID, true))
	c = claim(t, s, p, "revision-fencing-key")
	// A feed/preference revision, even without a corpus change, fences work.
	_, err = s.writer.ExecContext(t.Context(), "UPDATE feeds SET revision=revision+1 WHERE tenant_id=? AND id=?", tenantA, feedID)
	must(t, err)
	if err := s.StartProvider(t.Context(), p, c.WorkID); !errors.Is(err, query.ErrConflict) {
		t.Fatal("stale revision authorized inference", err)
	}
	must(t, s.FailQuery(t.Context(), p, c.WorkID))
	budget(t, s, 0, 40)
}

func TestQueryCacheExpiryRetentionAndPreferenceRevision(t *testing.T) {
	s, _, p, sel := queryFixture(t)
	c := claim(t, s, p, "retention-origin-key")
	must(t, s.StartProvider(t.Context(), p, c.WorkID))
	snap := complete(t, s, p, c, sel, query.Settlement{Known: true})
	s.clock = func() time.Time { return queryEpoch.Add(9 * time.Minute) }
	if claim(t, s, p, "retention-cache-key").Snapshot.ID != snap.ID {
		t.Fatal("cache hit missing")
	}
	s.clock = func() time.Time { return queryEpoch.Add(10 * time.Minute) }
	miss := claim(t, s, p, "expired-cache-new-key")
	if miss.Snapshot != nil {
		t.Fatal("expired cache used")
	}
	must(t, s.FailQuery(t.Context(), p, miss.WorkID))
	s.clock = func() time.Time { return queryEpoch.Add(24*time.Hour + 5*time.Minute) }
	if claim(t, s, p, "retention-cache-key").Snapshot.ID != snap.ID {
		t.Fatal("cache replay did not extend retention")
	}
	if _, err := s.BeginQuery(t.Context(), p, "retention-origin-key", request(), policy()); !errors.Is(err, query.ErrUnavailable) {
		t.Fatal("expired key restarted", err)
	}
	s.clock = func() time.Time { return queryEpoch.Add(24*time.Hour + 10*time.Minute) }
	if _, err := s.GetSnapshot(t.Context(), p, snap.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("snapshot read beyond retention", err)
	}
	// Separate fresh snapshot demonstrates explicit preference revision scoping.
	c = claim(t, s, p, "preference-origin-key")
	must(t, s.StartProvider(t.Context(), p, c.WorkID))
	complete(t, s, p, c, sel, query.Settlement{Known: true})
	_, err := s.writer.ExecContext(t.Context(), "UPDATE feeds SET revision=revision+1 WHERE tenant_id=? AND id=?", tenantA, feedID)
	must(t, err)
	if claim(t, s, p, "preference-changed-key").Snapshot != nil {
		t.Fatal("preference revision not in cache")
	}
}
