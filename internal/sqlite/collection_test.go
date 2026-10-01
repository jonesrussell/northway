package sqlite

import (
	"errors"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"sync"
	"testing"
	"time"
)

func collectionSetup(t *testing.T) (*Store, string, *time.Time) {
	s, path, now := pollSetup(t)
	must(t, s.ConfigurePoll(t.Context(), operator(tenantA), ingest.Policy{SourceID: sourceID, URL: "https://example.invalid/feed", Mode: "html", Approved: true, Enabled: true, PreviewAllowed: true, RobotsUntil: now.Add(24 * time.Hour), Interval: time.Hour, MaxBytes: 2048}))
	return s, path, now
}
func collectionResult() ingest.Result {
	return ingest.Result{Status: 200, Bytes: 500, ETag: "v1", Observations: []ingest.Observation{{Version: 1, AccountURL: "https://example.invalid/feed", EvidenceURL: "https://example.invalid/feed", OriginalURL: "https://example.invalid/feed", Kind: "profile", Title: "River", Display: "link", State: "available", Method: "html-metadata-v1"}}}
}
func TestCollectionReplayRevisionsRemovalAndRestart(t *testing.T) {
	s, path, now := collectionSetup(t)
	p := operator(tenantA)
	if _, e := s.ClaimPoll(t.Context(), p); !errors.Is(e, ingest.ErrIdle) {
		t.Fatal("news scheduler admitted HTML", e)
	}
	cl, e := s.ClaimCollection(t.Context(), p)
	must(t, e)
	must(t, s.FinishPoll(t.Context(), p, cl.ID, collectionResult()))
	b, e := s.CollectionBatch(t.Context(), p, 0)
	must(t, e)
	if len(b.Events) != 1 || b.Events[0].Observation.Revision != 1 {
		t.Fatal(b)
	}
	id := b.Events[0].Observation.ID
	replay, e := s.CollectionBatch(t.Context(), p, 0)
	must(t, e)
	if replay.Next != b.Next {
		t.Fatal("cursor replay changed")
	}
	must(t, s.Close())
	reopened, e := Open(t.Context(), path)
	must(t, e)
	defer reopened.Close()
	s = reopened
	s.clock = func() time.Time { return *now }
	*now = now.Add(time.Hour)
	must(t, s.ResetPollSchedule(t.Context(), p, sourceID))
	cl, e = s.ClaimCollection(t.Context(), p)
	must(t, e)
	must(t, s.FinishPoll(t.Context(), p, cl.ID, collectionResult()))
	unchanged, e := s.CollectionBatch(t.Context(), p, b.Next)
	must(t, e)
	if len(unchanged.Events) != 0 {
		t.Fatal("unchanged observation versioned")
	}
	*now = now.Add(time.Hour)
	must(t, s.ResetPollSchedule(t.Context(), p, sourceID))
	cl, e = s.ClaimCollection(t.Context(), p)
	must(t, e)
	must(t, s.FinishPoll(t.Context(), p, cl.ID, ingest.Result{Status: 404}))
	removed, e := s.CollectionBatch(t.Context(), p, b.Next)
	must(t, e)
	if len(removed.Events) != 1 || removed.Events[0].Observation.State != "removed" || removed.Events[0].Observation.ID != id || removed.Events[0].Observation.Revision != 2 {
		t.Fatal(removed)
	}
}
func TestCollectionAtomicRollbackTenantAndLease(t *testing.T) {
	s, _, now := collectionSetup(t)
	p := operator(tenantA)
	cl, e := s.ClaimCollection(t.Context(), p)
	must(t, e)
	invalid := collectionResult()
	invalid.Observations = append(invalid.Observations, invalid.Observations[0])
	if e = s.FinishPoll(t.Context(), p, cl.ID, invalid); !errors.Is(e, ingest.ErrInvalid) {
		t.Fatal(e)
	}
	b, e := s.CollectionBatch(t.Context(), p, 0)
	must(t, e)
	if len(b.Events) != 0 {
		t.Fatal("rollback escaped")
	}
	if e = s.FinishPoll(t.Context(), operator(tenantB), cl.ID, collectionResult()); e == nil {
		t.Fatal("wrong tenant settled")
	}
	*now = now.Add(ingest.LeaseDuration)
	if e = s.FinishPoll(t.Context(), p, cl.ID, collectionResult()); !errors.Is(e, ingest.ErrLease) {
		t.Fatal("stale lease accepted", e)
	}
	if _, e = s.CollectionBatch(t.Context(), identity.Principal{}, 0); e == nil {
		t.Fatal("zero authority exported")
	}
}
func TestCollectionConcurrentClaimsAndExpiredRobots(t *testing.T) {
	s, _, now := collectionSetup(t)
	p := operator(tenantA)
	var wg sync.WaitGroup
	var mu sync.Mutex
	n := 0
	for range 12 {
		wg.Go(func() {
			_, e := s.ClaimCollection(t.Context(), p)
			if e == nil {
				mu.Lock()
				n++
				mu.Unlock()
			} else if !errors.Is(e, ingest.ErrBusy) {
				t.Error(e)
			}
		})
	}
	wg.Wait()
	if n != 1 {
		t.Fatal(n)
	}
	*now = now.Add(25 * time.Hour)
	if _, e := s.ClaimCollection(t.Context(), p); !errors.Is(e, ingest.ErrIdle) {
		t.Fatal("expired robots admitted", e)
	}
}

func TestCollectionSeedManagementIsDisabledIdempotentAndTenantScoped(t *testing.T) {
	s, _, _ := pollSetup(t)
	p := operator(tenantA)
	id := "00000006-0000-4000-8000-000000000000"
	seed := ingest.CollectionSeed{ID: id, URL: "https://creator.example/profile", Title: "Creator candidate"}
	must(t, s.AddCollectionSeed(t.Context(), p, seed))
	must(t, s.AddCollectionSeed(t.Context(), p, seed))
	v, e := s.CollectionStatus(t.Context(), p)
	must(t, e)
	if v.Seeds != 1 || v.Enabled != 0 || v.Items != 0 {
		t.Fatal(v)
	}
	seed.URL = "https://other.example/profile"
	if e = s.AddCollectionSeed(t.Context(), p, seed); !errors.Is(e, ingest.ErrInvalid) {
		t.Fatal("changed seed accepted", e)
	}
	if _, e = s.ClaimCollection(t.Context(), p); !errors.Is(e, ingest.ErrIdle) {
		t.Fatal("seed enabled acquisition", e)
	}
	if _, e = s.CollectionStatus(t.Context(), operator(tenantB)); e == nil {
		t.Fatal("unprovisioned tenant exported")
	}
}

func TestCollectionRemovalAtVersionCapacity(t *testing.T) {
	s, _, now := collectionSetup(t)
	defer s.Close()
	p := operator(tenantA)
	cl, e := s.ClaimCollection(t.Context(), p)
	must(t, e)
	must(t, s.FinishPoll(t.Context(), p, cl.ID, collectionResult()))
	_, e = s.writer.ExecContext(t.Context(), `WITH RECURSIVE n(x) AS (SELECT 2 UNION ALL SELECT x+1 FROM n WHERE x<10000) INSERT INTO collection_events(tenant_id,source_id,item_id,revision,payload) SELECT tenant_id,source_id,'capacity-fixture',x,payload FROM collection_events,n WHERE sequence=1`)
	must(t, e)
	*now = now.Add(time.Hour)
	must(t, s.ResetPollSchedule(t.Context(), p, sourceID))
	cl, e = s.ClaimCollection(t.Context(), p)
	must(t, e)
	must(t, s.FinishPoll(t.Context(), p, cl.ID, ingest.Result{Status: 410}))
	b, e := s.CollectionBatch(t.Context(), p, 10000)
	must(t, e)
	if len(b.Events) != 1 || b.Events[0].Observation.State != "removed" {
		t.Fatal(b)
	}
}
