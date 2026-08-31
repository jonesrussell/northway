package sqlite

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	assets "github.com/jonesrussell/northway/db"
	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"github.com/jonesrussell/northway/internal/usage"
	modern "modernc.org/sqlite"
)

var queryEpoch = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

func queryFixture(t *testing.T) (*Store, string, identity.Principal, query.Selection) {
	t.Helper()
	s, path := fresh(t)
	s.clock = func() time.Time { return queryEpoch }
	seed(t, s, tenantA)
	must(t, s.PutArticle(t.Context(), operator(tenantA), item()))
	_, secret := newKey(t, s, tenantA, identity.FeedsRead)
	p, err := identity.NewService(s).Authenticate(t.Context(), secret.Reveal())
	must(t, err)
	a, err := s.GetArticle(t.Context(), p, itemID)
	must(t, err)
	must(t, s.SetBudget(t.Context(), operator(tenantA), 100))
	return s, path, p, query.Selection{ArticleID: a.ID, ContentHash: a.ContentHash, Explanation: "Synthetic PHP fixture"}
}
func request() query.Request {
	return query.Request{FeedID: feedID, Context: query.Context{Intent: "private-context-marker", Technologies: []query.Technology{{Name: "PHP"}}}, MaxAgeHours: 720, Limit: 5}
}
func policy() query.Policy {
	return query.Policy{RankerVersion: "fixture-v1", WorstCaseMicros: 40, Lease: time.Minute, CacheTTL: 10 * time.Minute}
}
func claim(t *testing.T, s *Store, p identity.Principal, key string) query.Claim {
	t.Helper()
	c, err := s.BeginQuery(t.Context(), p, key, request(), policy())
	must(t, err)
	return c
}
func budget(t *testing.T, s *Store, spent, held int64) {
	t.Helper()
	b, err := s.GetBudget(t.Context(), operator(tenantA))
	must(t, err)
	if b.SpentMicros != spent || b.HeldMicros != held {
		t.Fatalf("budget=%+v, want spent=%d held=%d", b, spent, held)
	}
}
func complete(t *testing.T, s *Store, p identity.Principal, c query.Claim, sel query.Selection, cost query.Settlement) query.Snapshot {
	t.Helper()
	snap, err := s.CompleteQuery(t.Context(), p, c.WorkID, "ai", []query.Selection{sel}, cost)
	must(t, err)
	return snap
}

func TestQueryConcurrentClaimReplayCacheAndRestart(t *testing.T) {
	s, path, p, sel := queryFixture(t)
	var wg sync.WaitGroup
	claims := make(chan query.Claim, 20)
	errs := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			c, err := s.BeginQuery(t.Context(), p, "one-concurrent-key", request(), policy())
			if err != nil {
				errs <- err
			} else {
				claims <- c
			}
		})
	}
	wg.Wait()
	close(claims)
	close(errs)
	if len(claims) != 1 || len(errs) != 19 {
		t.Fatalf("claims=%d errors=%d", len(claims), len(errs))
	}
	for err := range errs {
		if !errors.Is(err, query.ErrInProgress) {
			t.Fatal(err)
		}
	}
	c := <-claims
	if !c.ProviderAllowed || c.Snapshot != nil || c.WorkID == "" {
		t.Fatal("invalid winning claim")
	}
	budget(t, s, 0, 40)
	var calls atomic.Int64
	for range 12 {
		wg.Go(func() {
			if s.StartProvider(t.Context(), p, c.WorkID) == nil {
				calls.Add(1)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("multiple provider attempts authorized", calls.Load())
	}
	snap := complete(t, s, p, c, sel, query.Settlement{Known: true, ActualMicros: 17})
	budget(t, s, 17, 0)
	replay := claim(t, s, p, "one-concurrent-key")
	if replay.Snapshot == nil || replay.Snapshot.ID != snap.ID || replay.WorkID != "" || replay.ProviderAllowed {
		t.Fatal("replay duplicated work")
	}
	changed := request()
	changed.Limit++
	if _, err := s.BeginQuery(t.Context(), p, "one-concurrent-key", changed, policy()); !errors.Is(err, query.ErrConflict) {
		t.Fatal("missing changed-payload conflict", err)
	}
	hit := claim(t, s, p, "different-cache-key")
	if hit.Snapshot == nil || hit.Snapshot.ID != snap.ID {
		t.Fatal("cache miss")
	}
	budget(t, s, 17, 0)
	if _, err := s.CompleteQuery(t.Context(), p, c.WorkID, "ai", []query.Selection{sel}, query.Settlement{Known: true, ActualMicros: 17}); !errors.Is(err, query.ErrConflict) {
		t.Fatal("duplicate completion", err)
	}
	must(t, s.Close())
	s, err := Open(t.Context(), path)
	must(t, err)
	defer s.Close()
	s.clock = func() time.Time { return queryEpoch.Add(23 * time.Hour) }
	replay = claim(t, s, p, "one-concurrent-key")
	if replay.Snapshot.ID != snap.ID {
		t.Fatal("restart/expired-cache replay lost snapshot")
	}
	budget(t, s, 17, 0)
	// Ordinary context and opaque caller keys are not persisted, including WAL.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		data, err := os.ReadFile(path + suffix)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		must(t, err)
		for _, raw := range []string{"private-context-marker", "one-concurrent-key", "different-cache-key"} {
			if strings.Contains(string(data), raw) {
				t.Fatal("private input persisted")
			}
		}
	}
}

func TestQueryBudgetRaceFallbackAndOverflow(t *testing.T) {
	s, _, p, _ := queryFixture(t)
	var wg sync.WaitGroup
	claims := make(chan query.Claim, 10)
	errs := make(chan error, 10)
	for i := range 10 {
		wg.Go(func() {
			c, err := s.BeginQuery(t.Context(), p, fmt.Sprintf("budget-race-key-%02d", i), request(), policy())
			claims <- c
			errs <- err
		})
	}
	wg.Wait()
	close(claims)
	close(errs)
	for err := range errs {
		must(t, err)
	}
	allowed := 0
	for c := range claims {
		if c.ProviderAllowed {
			allowed++
		} else if err := s.StartProvider(t.Context(), p, c.WorkID); !errors.Is(err, query.ErrConflict) {
			t.Fatal("budgetless provider authorized", err)
		}
	}
	if allowed != 2 {
		t.Fatal("budget race admitted", allowed)
	}
	budget(t, s, 0, 80)
	if err := s.SetBudget(t.Context(), operator(tenantA), 79); !errors.Is(err, usage.ErrLimit) {
		t.Fatal("lowered limit below holds", err)
	}
	// Integer extremes must not promote accounting to REAL or wrap around.
	must(t, s.SetBudget(t.Context(), operator(tenantA), math.MaxInt64))
	pol := policy()
	pol.WorstCaseMicros = math.MaxInt64
	c, err := s.BeginQuery(t.Context(), p, "overflow-boundary-key", request(), pol)
	must(t, err)
	if c.ProviderAllowed {
		t.Fatal("overflow admitted")
	}
	must(t, s.FailQuery(t.Context(), p, c.WorkID))
	budget(t, s, 0, 80)
	seed(t, s, tenantB)
	b, err := s.BeginQuery(t.Context(), operator(tenantB), "no-budget-tenant-key", request(), policy())
	must(t, err)
	if b.ProviderAllowed {
		t.Fatal("unconfigured tenant got paid work")
	}
	_, err = s.CompleteQuery(t.Context(), operator(tenantB), b.WorkID, "deterministic_fallback", nil, query.Settlement{Known: true})
	must(t, err)
}

func TestQueryRecoveryNeverBlindlyRefundsOrRetries(t *testing.T) {
	s, path, p, sel := queryFixture(t)
	unstarted := claim(t, s, p, "unstarted-claim-key")
	started := claim(t, s, p, "started-claim-key-1")
	must(t, s.StartProvider(t.Context(), p, started.WorkID))
	budget(t, s, 0, 80)
	must(t, s.Close())
	s, err := Open(t.Context(), path)
	must(t, err)
	defer s.Close()
	s.clock = func() time.Time { return queryEpoch.Add(2 * time.Minute) }
	if _, err := s.BeginQuery(t.Context(), p, "started-claim-key-1", request(), policy()); !errors.Is(err, query.ErrUnavailable) {
		t.Fatal("expired replay may retry", err)
	}
	n, err := s.RecoverQueries(t.Context(), operator(tenantA))
	must(t, err)
	if n != 2 {
		t.Fatal("wrong recovery count", n)
	}
	budget(t, s, 0, 40)
	for _, id := range []string{unstarted.WorkID, started.WorkID} {
		if err := s.StartProvider(t.Context(), p, id); !errors.Is(err, query.ErrConflict) {
			t.Fatal("stale worker started", err)
		}
		if _, err := s.CompleteQuery(t.Context(), p, id, "ai", []query.Selection{sel}, query.Settlement{Known: true}); !errors.Is(err, query.ErrConflict) {
			t.Fatal("stale worker settled", err)
		}
	}
	n, err = s.RecoverQueries(t.Context(), operator(tenantA))
	must(t, err)
	if n != 0 {
		t.Fatal("recovery repeated")
	}
	if err := s.ReconcileQuery(t.Context(), p, started.WorkID, 10); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("service key reconciled spend", err)
	}
	must(t, s.ReconcileQuery(t.Context(), operator(tenantA), started.WorkID, 10))
	must(t, s.ReconcileQuery(t.Context(), operator(tenantA), started.WorkID, 10))
	if err := s.ReconcileQuery(t.Context(), operator(tenantA), started.WorkID, 11); !errors.Is(err, query.ErrConflict) {
		t.Fatal("contradictory reconciliation", err)
	}
	budget(t, s, 10, 0)
	if _, err := s.BeginQuery(t.Context(), p, "unstarted-claim-key", request(), policy()); !errors.Is(err, query.ErrUnavailable) {
		t.Fatal("failed key automatically retried", err)
	}
	// A timeout may still return a deterministic result while keeping the hold.
	c := claim(t, s, p, "fallback-after-timeout")
	must(t, s.StartProvider(t.Context(), p, c.WorkID))
	snap, err := s.CompleteQuery(t.Context(), p, c.WorkID, "deterministic_fallback", []query.Selection{sel}, query.Settlement{})
	must(t, err)
	budget(t, s, 10, 40)
	if claim(t, s, p, "fallback-after-timeout").Snapshot.ID != snap.ID {
		t.Fatal("uncertain replay duplicated work")
	}
	must(t, s.ReconcileQuery(t.Context(), operator(tenantA), c.WorkID, 23))
	budget(t, s, 33, 0)
}

func TestQueryCacheIsolationRevisionAndRevocation(t *testing.T) {
	s, _, p, sel := queryFixture(t)
	c := claim(t, s, p, "cache-scope-original")
	must(t, s.StartProvider(t.Context(), p, c.WorkID))
	snap := complete(t, s, p, c, sel, query.Settlement{Known: true, ActualMicros: 1})
	seed(t, s, tenantB)
	other, err := s.BeginQuery(t.Context(), operator(tenantB), "cache-scope-original", request(), policy())
	must(t, err)
	if other.Snapshot != nil {
		t.Fatal("cross-tenant cache hit")
	}
	if _, err := s.GetSnapshot(t.Context(), operator(tenantB), snap.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-tenant snapshot", err)
	}
	if err := s.StartProvider(t.Context(), operator(tenantB), c.WorkID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-tenant work", err)
	}
	if err := s.ReconcileQuery(t.Context(), operator(tenantB), c.WorkID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-tenant settlement", err)
	}
	for i, change := range []func(*query.Request, *query.Policy){
		func(r *query.Request, p *query.Policy) { r.Context.Intent = "different private context" },
		func(r *query.Request, p *query.Policy) { p.RankerVersion = "fixture-v2" },
		func(r *query.Request, p *query.Policy) { r.MaxAgeHours = 24 },
		func(r *query.Request, p *query.Policy) { r.Limit = 1 },
	} {
		r, pol := request(), policy()
		change(&r, &pol)
		got, err := s.BeginQuery(t.Context(), p, fmt.Sprintf("cache-dimension-%03d", i), r, pol)
		must(t, err)
		if got.Snapshot != nil {
			t.Fatal("cache dimension omitted", i)
		}
		must(t, s.FailQuery(t.Context(), p, got.WorkID))
	}
	otherFeed := "00000006-0000-4000-8000-000000000000"
	must(t, s.CreateFeed(t.Context(), operator(tenantA), feed.Feed{ID: otherFeed, Title: "Other"}))
	r := request()
	r.FeedID = otherFeed
	got, err := s.BeginQuery(t.Context(), p, "different-feed-key", r, policy())
	must(t, err)
	if got.Snapshot != nil {
		t.Fatal("feed cache collision")
	}
	must(t, s.FailQuery(t.Context(), p, got.WorkID))
	// Corpus changes invalidate new keys, but retained replay preserves evidence.
	a := item()
	a.Title = "Updated PHP fixture"
	must(t, s.PutArticle(t.Context(), operator(tenantA), a))
	got = claim(t, s, p, "updated-corpus-key")
	if got.Snapshot != nil {
		t.Fatal("stale corpus cache")
	}
	must(t, s.FailQuery(t.Context(), p, got.WorkID))
	old, err := s.GetSnapshot(t.Context(), p, snap.ID)
	must(t, err)
	if old.Items[0].Title != snap.Items[0].Title {
		t.Fatal("snapshot reranked/rewritten")
	}
	must(t, s.SetSourceEnabled(t.Context(), operator(tenantA), sourceID, false))
	old, err = s.GetSnapshot(t.Context(), p, snap.ID)
	must(t, err)
	if len(old.Items) != 0 || !old.Suppressed {
		t.Fatal("revoked source leaked")
	}
	replay := claim(t, s, p, "cache-scope-original")
	if len(replay.Snapshot.Items) != 0 || !replay.Snapshot.Suppressed {
		t.Fatal("replay bypassed entitlements")
	}
	got = claim(t, s, p, "changed-entitlement-key")
	if got.Snapshot != nil {
		t.Fatal("entitlement revision ignored")
	}
	must(t, s.FailQuery(t.Context(), p, got.WorkID))
	must(t, s.SetSourceEnabled(t.Context(), operator(tenantA), sourceID, true))
	must(t, s.DetachSource(t.Context(), operator(tenantA), feedID, sourceID))
	old, err = s.GetSnapshot(t.Context(), p, snap.ID)
	must(t, err)
	if len(old.Items) != 0 {
		t.Fatal("detached source leaked")
	}
	must(t, s.SetFeedEnabled(t.Context(), operator(tenantA), feedID, false))
	if _, err := s.GetSnapshot(t.Context(), p, snap.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("disabled feed visible", err)
	}
	if _, err := s.BeginQuery(t.Context(), p, "cache-scope-original", request(), policy()); !errors.Is(err, ErrNotFound) {
		t.Fatal("disabled feed replay", err)
	}
}

func TestQueryAtomicRollbackCancellationAndFailedCompletion(t *testing.T) {
	s, _, p, sel := queryFixture(t)
	_, err := s.writer.ExecContext(t.Context(), `CREATE TRIGGER reject_claim BEFORE INSERT ON query_work BEGIN SELECT RAISE(ABORT,'injected claim failure'); END`)
	must(t, err)
	if c, err := s.BeginQuery(t.Context(), p, "failed-transaction-key", request(), policy()); err == nil || c.WorkID != "" {
		t.Fatal("claim survived rollback")
	}
	budget(t, s, 0, 0)
	_, err = s.writer.ExecContext(t.Context(), `DROP TRIGGER reject_claim`)
	must(t, err)
	c := claim(t, s, p, "failed-transaction-key")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.StartProvider(ctx, p, c.WorkID); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	must(t, s.StartProvider(t.Context(), p, c.WorkID))
	_, err = s.writer.ExecContext(t.Context(), `CREATE TRIGGER reject_snapshot BEFORE INSERT ON query_snapshots BEGIN SELECT RAISE(ABORT,'injected snapshot failure'); END`)
	must(t, err)
	if snap, err := s.CompleteQuery(t.Context(), p, c.WorkID, "ai", []query.Selection{sel}, query.Settlement{Known: true, ActualMicros: 9}); err == nil || snap.ID != "" {
		t.Fatal("snapshot survived rollback")
	}
	budget(t, s, 0, 40)
	_, err = s.writer.ExecContext(t.Context(), `DROP TRIGGER reject_snapshot`)
	must(t, err)
	// Retrying local finalization is safe; starting another provider call is not.
	if err := s.StartProvider(t.Context(), p, c.WorkID); !errors.Is(err, query.ErrConflict) {
		t.Fatal("failed finalization retried provider", err)
	}
	complete(t, s, p, c, sel, query.Settlement{Known: true, ActualMicros: 9})
	budget(t, s, 9, 0)
	// An explicit failure after starting retains uncertainty and is idempotent.
	r := request()
	r.Context.Intent = "new failure"
	c, err = s.BeginQuery(t.Context(), p, "explicit-failure-key", r, policy())
	must(t, err)
	must(t, s.StartProvider(t.Context(), p, c.WorkID))
	must(t, s.FailQuery(t.Context(), p, c.WorkID))
	must(t, s.FailQuery(t.Context(), p, c.WorkID))
	budget(t, s, 9, 40)
}

func TestQueryFullRollbackPreservesReservation(t *testing.T) {
	s, _, p, sel := queryFixture(t)
	// An explanation spanning several pages forces an actual engine SQLITE_FULL.
	sel.Explanation = strings.Repeat("𐀀", 1000)
	c := claim(t, s, p, "disk-full-query-key")
	must(t, s.StartProvider(t.Context(), p, c.WorkID))
	var pages, maximum int
	must(t, s.writer.QueryRowContext(t.Context(), "PRAGMA page_count").Scan(&pages))
	must(t, s.writer.QueryRowContext(t.Context(), fmt.Sprintf("PRAGMA max_page_count=%d", pages)).Scan(&maximum))
	_, err := s.CompleteQuery(t.Context(), p, c.WorkID, "ai", []query.Selection{sel}, query.Settlement{Known: true, ActualMicros: 7})
	var e *modern.Error
	if !errors.As(err, &e) || e.Code()&255 != 13 {
		t.Fatalf("expected actual SQLITE_FULL, got %v", err)
	}
	budget(t, s, 0, 40)
	must(t, s.writer.QueryRowContext(t.Context(), "PRAGMA max_page_count=2147483646").Scan(&maximum))
	complete(t, s, p, c, sel, query.Settlement{Known: true, ActualMicros: 7})
	budget(t, s, 7, 0)
}

func TestQueryUpgradeFromSchemaThree(t *testing.T) {
	path := filepath.Join(privateDir(t), "upgrade.sqlite")
	file, abs, err := lockFile(path, true)
	must(t, err)
	db, err := openPool(abs, false)
	must(t, err)
	migrations, err := fs.Sub(assets.Migrations, "migrations")
	must(t, err)
	provider, err := provider(db, migrations)
	must(t, err)
	_, err = provider.UpTo(t.Context(), 3)
	must(t, err)
	q := sqlc.New(db)
	must(t, q.CreateTenant(t.Context(), sqlc.CreateTenantParams{ID: string(tenantA), CreatedAt: 1}))
	must(t, q.CreateFeed(t.Context(), sqlc.CreateFeedParams{TenantID: string(tenantA), ID: feedID, Title: "Existing"}))
	record, secret, err := identity.GenerateKey(operator(tenantA), identity.FeedsRead)
	must(t, err)
	must(t, q.CreateAPIKey(t.Context(), sqlc.CreateAPIKeyParams{TenantID: string(tenantA), ID: record.ID, Digest: record.Digest[:], Scopes: 1, CreatedAt: record.CreatedAt.UnixMicro()}))
	must(t, db.Close())
	must(t, file.Close())
	must(t, Migrate(t.Context(), path))
	s, err := Open(t.Context(), path)
	must(t, err)
	defer s.Close()
	p, err := identity.NewService(s).Authenticate(t.Context(), secret.Reveal())
	must(t, err)
	c := claim(t, s, p, "upgraded-query-key")
	if c.ProviderAllowed {
		t.Fatal("migration enabled paid work")
	}
	_, err = s.CompleteQuery(t.Context(), p, c.WorkID, "deterministic_fallback", nil, query.Settlement{Known: true})
	must(t, err)
}
