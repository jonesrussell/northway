package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/source"
)

func pollSetup(t *testing.T) (*Store, string, *time.Time) {
	t.Helper()
	s, path := fresh(t)
	seed(t, s, tenantA)
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	s.clock = func() time.Time { return now }
	must(t, s.ConfigurePoll(t.Context(), operator(tenantA), ingest.Policy{SourceID: sourceID, URL: "https://example.invalid/feed", Approved: true, Enabled: true, Interval: time.Hour, MaxBytes: 2048}))
	return s, path, &now
}
func pollResult() ingest.Result {
	return ingest.Result{Status: 200, Bytes: 500, ETag: `"v1"`, Items: []ingest.Item{{OriginID: "one", URL: "https://example.invalid/item", Title: "Community news"}}}
}

func TestPollAtomicVersions304AndUnchanged200(t *testing.T) {
	s, _, now := pollSetup(t)
	p := operator(tenantA)
	claim, err := s.ClaimPoll(t.Context(), p)
	must(t, err)
	must(t, s.FinishPoll(t.Context(), p, claim.ID, pollResult()))
	var revision int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT corpus_revision FROM tenants WHERE id=?", tenantA).Scan(&revision))
	*now = now.Add(time.Hour)
	claim, err = s.ClaimPoll(t.Context(), p)
	must(t, err)
	if claim.ETag != `"v1"` {
		t.Fatal("validator lost")
	}
	must(t, s.FinishPoll(t.Context(), p, claim.ID, pollResult()))
	var current, versions int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT corpus_revision FROM tenants WHERE id=?", tenantA).Scan(&current))
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM article_versions").Scan(&versions))
	if current != revision || versions != 1 {
		t.Fatal("unchanged 200 caused writes", current, revision, versions)
	}
	*now = now.Add(time.Hour)
	claim, err = s.ClaimPoll(t.Context(), p)
	must(t, err)
	must(t, s.FinishPoll(t.Context(), p, claim.ID, ingest.Result{Status: 304}))
	var success int64
	var etag string
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT last_success,etag FROM poll_sources").Scan(&success, &etag))
	if success != now.UnixMicro() || etag != `"v1"` {
		t.Fatal("304 did not preserve validator/advance check")
	}
	*now = now.Add(time.Hour)
	claim, err = s.ClaimPoll(t.Context(), p)
	must(t, err)
	changed := pollResult()
	changed.Items[0].URL = "https://example.invalid/new-location"
	changed.ETag = ""
	must(t, s.FinishPoll(t.Context(), p, claim.ID, changed))
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM article_versions").Scan(&versions))
	if versions != 2 {
		t.Fatal("URL-only update lost version")
	}
	got, err := s.GetArticle(t.Context(), p, itemIDFor(sourceID, "one"))
	must(t, err)
	if got.Body != "" || got.PublishedAt != nil || got.URL != changed.Items[0].URL {
		t.Fatal(got)
	}
	if len(search(t, s, tenantA, "Community")) != 1 {
		t.Fatal("FTS missing")
	}
}

func TestPollRollbackDoesNotAdvanceValidators(t *testing.T) {
	s, _, _ := pollSetup(t)
	p := operator(tenantA)
	claim, err := s.ClaimPoll(t.Context(), p)
	must(t, err)
	_, err = s.writer.ExecContext(t.Context(), `CREATE TRIGGER synthetic_failure BEFORE INSERT ON articles WHEN new.origin_id='two' BEGIN SELECT RAISE(ABORT,'synthetic commit failure'); END`)
	must(t, err)
	result := pollResult()
	result.Items = append(result.Items, ingest.Item{OriginID: "two", URL: "https://example.invalid/two", Title: "Second"})
	if err := s.FinishPoll(t.Context(), p, claim.ID, result); err == nil {
		t.Fatal("injected failure ignored")
	}
	var articles, versions int
	var etag, state string
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM articles").Scan(&articles))
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM article_versions").Scan(&versions))
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT etag FROM poll_sources").Scan(&etag))
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT state FROM poll_attempts").Scan(&state))
	if articles != 0 || versions != 0 || etag != "" || state != "pending" || len(search(t, s, tenantA, "Community")) != 0 {
		t.Fatal("partial commit")
	}
	_, err = s.writer.ExecContext(t.Context(), "DROP TRIGGER synthetic_failure")
	must(t, err)
	must(t, s.FinishPoll(t.Context(), p, claim.ID, result))
}

func TestPollCrashRestartAndLeaseFencing(t *testing.T) {
	s, path, now := pollSetup(t)
	p := operator(tenantA)
	old, err := s.ClaimPoll(t.Context(), p)
	must(t, err)
	must(t, s.Close())
	s, err = Open(t.Context(), path)
	must(t, err)
	defer s.Close()
	s.clock = func() time.Time { return *now }
	if _, err = s.ClaimPoll(t.Context(), p); !errors.Is(err, ingest.ErrBusy) {
		t.Fatal(err)
	}
	*now = now.Add(3 * time.Minute)
	if err = s.FinishPoll(t.Context(), p, old.ID, pollResult()); !errors.Is(err, ingest.ErrLease) {
		t.Fatal("stale worker accepted", err)
	}
	if _, err = s.ClaimPoll(t.Context(), p); !errors.Is(err, ingest.ErrIdle) {
		t.Fatal("restart caused early retry", err)
	}
	var charge int
	var state string
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT charged_bytes,state FROM poll_attempts").Scan(&charge, &state))
	if charge != 2048 || state != "abandoned" {
		t.Fatal("unknown work became free")
	}
	*now = now.Add(time.Hour)
	next, err := s.ClaimPoll(t.Context(), p)
	must(t, err)
	if next.ID == old.ID {
		t.Fatal("claim reused")
	}
	if err = s.FinishPoll(t.Context(), p, old.ID, pollResult()); !errors.Is(err, ingest.ErrLease) {
		t.Fatal(err)
	}
	must(t, s.FinishPoll(t.Context(), p, next.ID, pollResult()))
}

func TestPollAuthorizationAndReconfigurationFence(t *testing.T) {
	s, _, _ := pollSetup(t)
	seed(t, s, tenantB)
	for _, p := range []identity.Principal{{}, operator(tenantB)} {
		if _, err := s.ClaimPoll(t.Context(), p); err == nil {
			t.Fatal("cross tenant or empty scope claimed")
		}
	}
	a, err := s.ClaimPoll(t.Context(), operator(tenantA))
	must(t, err)
	if err = s.FinishPoll(t.Context(), operator(tenantB), a.ID, pollResult()); !errors.Is(err, ingest.ErrLease) {
		t.Fatal(err)
	}
	policy := ingest.Policy{SourceID: sourceID, URL: a.URL, Approved: true, Enabled: false, Interval: time.Hour, MaxBytes: 2048}
	must(t, s.ConfigurePoll(t.Context(), operator(tenantA), policy))
	if err = s.FinishPoll(t.Context(), operator(tenantA), a.ID, pollResult()); !errors.Is(err, ingest.ErrLease) {
		t.Fatal("revoked collection committed", err)
	}
}

func TestGlobalConcurrentClaimAndBudget(t *testing.T) {
	s, _, now := pollSetup(t)
	seed(t, s, tenantB)
	must(t, s.ConfigurePoll(t.Context(), operator(tenantB), ingest.Policy{SourceID: sourceID, URL: "https://example.invalid/feed", Approved: true, Enabled: true, Interval: time.Hour, MaxBytes: 2048}))
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, tenant := range []identity.TenantID{tenantA, tenantB} {
		wg.Go(func() { _, err := s.ClaimPoll(t.Context(), operator(tenant)); results <- err })
	}
	wg.Wait()
	close(results)
	successes, busy := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ingest.ErrBusy) {
			busy++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || busy != 1 {
		t.Fatal(successes, busy)
	}
	*now = now.Add(time.Hour)
	// Seed durable prior work rather than make hundreds of requests. Both tenant
	// scopes share the same reservation window and cannot reset it on restart.
	_, err := s.writer.ExecContext(t.Context(), "UPDATE poll_attempts SET state='done',charged_bytes=?,charged_at=?", ingest.DailyBytes/32, now.UnixMicro())
	must(t, err)
	for i := 0; i < 31; i++ {
		_, err = s.writer.ExecContext(t.Context(), `INSERT INTO poll_attempts VALUES(?,?,?,?,?,?,?,?,?)`, fmt.Sprintf("synthetic-%d", i), tenantA, sourceID, now.UnixMicro(), now.Add(time.Minute).UnixMicro(), now.UnixMicro(), ingest.MaxResponseBytes, ingest.MaxResponseBytes, "done")
		must(t, err)
	}
	if _, err = s.ClaimPoll(t.Context(), operator(tenantB)); !errors.Is(err, ingest.ErrBudget) {
		t.Fatal("global byte cap failed", err)
	}
	*now = now.Add(24*time.Hour + time.Second)
	if _, err = s.ClaimPoll(t.Context(), operator(tenantB)); err != nil {
		t.Fatal("rolling window did not release", err)
	}
}

func TestCursorSkipsHeldFeedsAndErrorsDoNotRetry(t *testing.T) {
	s, _, now := pollSetup(t)
	p := operator(tenantA)
	second := "00000006-0000-4000-8000-000000000000"
	must(t, s.CreateSource(t.Context(), p, source.Source{ID: second, URL: "https://example.invalid/second", Title: "Second"}))
	must(t, s.ConfigurePoll(t.Context(), p, ingest.Policy{SourceID: second, URL: "https://example.invalid/second", Approved: true, Enabled: true, Interval: time.Hour, MaxBytes: 2048}))
	first, err := s.ClaimPoll(t.Context(), p)
	must(t, err)
	must(t, s.FinishPoll(t.Context(), p, first.ID, ingest.Result{Status: 429, Failure: "http", NotBefore: now.Add(48 * time.Hour)}))
	next, err := s.ClaimPoll(t.Context(), p)
	must(t, err)
	if next.SourceID != second {
		t.Fatal("cursor ignored")
	}
	must(t, s.FinishPoll(t.Context(), p, next.ID, pollResult()))
	if _, err = s.ClaimPoll(t.Context(), p); !errors.Is(err, ingest.ErrIdle) {
		t.Fatal("automatic retry", err)
	}
	*now = now.Add(time.Hour)
	next, err = s.ClaimPoll(t.Context(), p)
	must(t, err)
	if next.SourceID != second {
		t.Fatal("publisher hold ignored")
	}
}

func TestInvalidPollResultAndDisabledDefaults(t *testing.T) {
	s, _ := fresh(t)
	seed(t, s, tenantA)
	if _, err := s.ClaimPoll(t.Context(), operator(tenantA)); !errors.Is(err, ingest.ErrIdle) {
		t.Fatal("source enabled collection implicitly", err)
	}
	s, _, _ = pollSetup(t)
	claim, err := s.ClaimPoll(t.Context(), operator(tenantA))
	must(t, err)
	for _, r := range []ingest.Result{{Status: 304}, {Status: 200, Bytes: 2049}, {Status: 200, ETag: "bad\r\nHeader: x"}, {Status: 200, Items: []ingest.Item{{OriginID: "x", URL: "https://example.invalid/x", Title: strings.Repeat("x", 513)}}}} {
		if err := s.FinishPoll(t.Context(), operator(tenantA), claim.ID, r); err == nil {
			t.Fatal("invalid result committed", r)
		}
	}
}

type cancelledFetcher struct{}

func (cancelledFetcher) Fetch(ctx context.Context, c ingest.Claim) ingest.Result {
	return ingest.Result{Failure: "transport"}
}
func TestServiceSettlesKnownFailure(t *testing.T) {
	s, _, _ := pollSetup(t)
	s.clock = nil
	_, err0 := s.writer.ExecContext(t.Context(), "UPDATE poll_sources SET next_at=0")
	must(t, err0)
	_, err := ingest.New(s, cancelledFetcher{}).RunOnce(t.Context(), operator(tenantA))
	if !errors.Is(err, ingest.ErrFetch) {
		t.Fatal(err)
	}
	var state string
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT state FROM poll_attempts").Scan(&state))
	if state != "done" {
		t.Fatal(state)
	}
}

func TestAttemptCapAndCursorSurviveBudgetDeferral(t *testing.T) {
	s, _, now := pollSetup(t)
	p := operator(tenantA)
	_, err := s.writer.ExecContext(t.Context(), `INSERT INTO poll_cursors VALUES(?,?)`, tenantA, "unchanged-cursor")
	must(t, err)
	tx, err := s.writer.BeginTx(t.Context(), nil)
	must(t, err)
	for i := 0; i < ingest.DailyAttempts; i++ {
		_, err = tx.ExecContext(t.Context(), `INSERT INTO poll_attempts VALUES(?,?,?,?,?,?,?,?,?)`, fmt.Sprintf("attempt-%d", i), tenantA, sourceID, now.UnixMicro(), now.Add(time.Minute).UnixMicro(), now.UnixMicro(), 0, 2048, "done")
		must(t, err)
	}
	must(t, tx.Commit())
	if _, err = s.ClaimPoll(t.Context(), p); !errors.Is(err, ingest.ErrBudget) {
		t.Fatal("zero-byte attempts bypassed cap", err)
	}
	var cursor string
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT source_id FROM poll_cursors").Scan(&cursor))
	if cursor != "unchanged-cursor" {
		t.Fatal("deferral advanced cursor")
	}
	*now = now.Add(24*time.Hour + time.Second)
	if _, err = s.ClaimPoll(t.Context(), p); err != nil {
		t.Fatal(err)
	}
}

type cancellingFetcher struct{ cancel context.CancelFunc }

func (f cancellingFetcher) Fetch(ctx context.Context, c ingest.Claim) ingest.Result {
	f.cancel()
	return ingest.Result{Failure: "transport"}
}
func TestCancellationStillSettlesConservativeBytes(t *testing.T) {
	s, _, _ := pollSetup(t)
	s.clock = nil
	_, err := s.writer.ExecContext(t.Context(), "UPDATE poll_sources SET next_at=0")
	must(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, err = ingest.New(s, cancellingFetcher{cancel}).RunOnce(ctx, operator(tenantA))
	if !errors.Is(err, ingest.ErrFetch) {
		t.Fatal(err)
	}
	var state string
	var charge int64
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT state,charged_bytes FROM poll_attempts").Scan(&state, &charge))
	if state != "done" || charge != 2048 {
		t.Fatal("cancellation lost accounting", state, charge)
	}
}

func TestPollDiskFullKeepsReservationAndValidators(t *testing.T) {
	s, _, _ := pollSetup(t)
	s.reservePages = 0
	p := operator(tenantA)
	claim, err := s.ClaimPoll(t.Context(), p)
	must(t, err)
	var pages, maximum int
	must(t, s.writer.QueryRowContext(t.Context(), "PRAGMA page_count").Scan(&pages))
	must(t, s.writer.QueryRowContext(t.Context(), fmt.Sprintf("PRAGMA max_page_count=%d", pages+1)).Scan(&maximum))
	r := pollResult()
	r.Items = nil
	for i := 0; i < 1000; i++ {
		r.Items = append(r.Items, ingest.Item{OriginID: fmt.Sprintf("id-%d", i), URL: fmt.Sprintf("https://example.invalid/%d", i), Title: strings.Repeat("Synthetic ", 45)})
	}
	if err = s.FinishPoll(t.Context(), p, claim.ID, r); err == nil {
		t.Fatal("disk-full not exercised")
	}
	var count int
	var state, etag string
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM articles").Scan(&count))
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT state FROM poll_attempts").Scan(&state))
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT etag FROM poll_sources").Scan(&etag))
	if count != 0 || state != "pending" || etag != "" {
		t.Fatal("disk-full partially committed")
	}
	must(t, s.writer.QueryRowContext(t.Context(), "PRAGMA max_page_count=2147483646").Scan(&maximum))
	must(t, s.FinishPoll(t.Context(), p, claim.ID, pollResult()))
}

func TestAPIKeysCannotOperateCollector(t *testing.T) {
	s, _, _ := pollSetup(t)
	_, secret := newKey(t, s, tenantA, identity.FeedsRead|identity.FeedbackWrite)
	principal, err := identity.NewService(s).Authenticate(t.Context(), secret.Reveal())
	must(t, err)
	if _, err = s.ClaimPoll(t.Context(), principal); err == nil {
		t.Fatal("API key claimed poll")
	}
	if err = s.ConfigurePoll(t.Context(), principal, ingest.Policy{SourceID: sourceID, URL: "https://example.invalid/feed", Approved: true, Enabled: true, Interval: time.Hour, MaxBytes: 2048}); err == nil {
		t.Fatal("API key approved source")
	}
	claim, err := s.ClaimPoll(t.Context(), operator(tenantA))
	must(t, err)
	if err = s.FinishPoll(t.Context(), principal, claim.ID, pollResult()); err == nil {
		t.Fatal("API key committed collector result")
	}
}

func TestLongSuccessfulHoldHasExplicitOperatorRecovery(t *testing.T) {
	s, _, now := pollSetup(t)
	p := operator(tenantA)
	claim, err := s.ClaimPoll(t.Context(), p)
	must(t, err)
	result := pollResult()
	result.NotBefore = now.Add(30 * 24 * time.Hour)
	must(t, s.FinishPoll(t.Context(), p, claim.ID, result))
	*now = now.Add(2 * time.Hour)
	policy := ingest.Policy{SourceID: sourceID, URL: claim.URL, Approved: true, Enabled: true, Interval: time.Hour, MaxBytes: 2048}
	must(t, s.ConfigurePoll(t.Context(), p, policy))
	if _, err = s.ClaimPoll(t.Context(), p); !errors.Is(err, ingest.ErrIdle) {
		t.Fatal("ordinary configuration shortened publisher hold", err)
	}
	_, secret := newKey(t, s, tenantA, identity.FeedsRead)
	key, err := identity.NewService(s).Authenticate(t.Context(), secret.Reveal())
	must(t, err)
	if err = s.ResetPollSchedule(t.Context(), key, sourceID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("API key reset hold")
	}
	must(t, s.ResetPollSchedule(t.Context(), p, sourceID))
	if _, err = s.ClaimPoll(t.Context(), p); err != nil {
		t.Fatal("operator recovery unavailable", err)
	}
}

func TestExistingNaturalKeyKeepsItsArticleID(t *testing.T) {
	s, _, _ := pollSetup(t)
	p := operator(tenantA)
	a := item()
	a.OriginID = "one"
	must(t, s.PutArticle(t.Context(), p, a))
	claim, err := s.ClaimPoll(t.Context(), p)
	must(t, err)
	must(t, s.FinishPoll(t.Context(), p, claim.ID, pollResult()))
	got, err := s.GetArticle(t.Context(), p, itemID)
	must(t, err)
	if got.Title != "Community news" || got.Body != "" {
		t.Fatal(got)
	}
	var count int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM articles").Scan(&count))
	if count != 1 {
		t.Fatal("natural key duplicated")
	}
}

type fullCorpusFetcher struct{}

func (fullCorpusFetcher) Fetch(context.Context, ingest.Claim) ingest.Result { return pollResult() }
func TestCorpusCapFailsAndSettlesWithoutPublishingItems(t *testing.T) {
	s, _, _ := pollSetup(t)
	s.clock = nil
	_, err := s.writer.ExecContext(t.Context(), "UPDATE poll_sources SET next_at=0")
	must(t, err)
	// Bulk synthetic prior corpus, with valid identity and actual FTS triggers.
	_, err = s.writer.ExecContext(t.Context(), `WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<5000)
INSERT INTO articles(tenant_id,id,source_id,origin_id,url,title,body,content_hash,observed_at)
SELECT ?,printf('%08x-0000-4000-8000-000000000000',x),?,printf('old-%d',x),'https://example.invalid/old','Prior item','',?,1 FROM n`, tenantA, sourceID, strings.Repeat("a", 64))
	must(t, err)
	result, err := ingest.New(s, fullCorpusFetcher{}).RunOnce(t.Context(), operator(tenantA))
	if !errors.Is(err, ingest.ErrCorpusFull) || len(result.Items) != 0 || result.Failure != "corpus_full" {
		t.Fatal(result, err)
	}
	var count int
	var state, category string
	var charged, reserved int64
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT charged_bytes,reserved_bytes FROM poll_attempts").Scan(&charged, &reserved))
	if charged != reserved || reserved != 2048 {
		t.Fatal("corpus failure refunded work", charged, reserved)
	}
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM articles").Scan(&count))
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT state FROM poll_attempts").Scan(&state))
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT last_error FROM poll_sources").Scan(&category))
	if count != 5000 || state != "done" || category != "corpus_full" {
		t.Fatal("corpus cap not rolled back/settled", count, state, category)
	}
}

func TestPolicyApprovalAndSourceCountGuard(t *testing.T) {
	s, _, _ := pollSetup(t)
	p := operator(tenantA)
	v := ingest.Policy{SourceID: sourceID, URL: "https://example.invalid/feed", Enabled: true, Interval: time.Hour, MaxBytes: 2048}
	if err := s.ConfigurePoll(t.Context(), p, v); !errors.Is(err, ingest.ErrInvalid) {
		t.Fatal("unapproved policy enabled", err)
	}
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("%08x-0000-4000-8000-000000000000", i+100)
		url := fmt.Sprintf("https://example.invalid/feed-%d", i)
		must(t, s.CreateSource(t.Context(), p, source.Source{ID: id, URL: url, Title: "Synthetic"}))
		err := s.ConfigurePoll(t.Context(), p, ingest.Policy{SourceID: id, URL: url, Interval: time.Hour, MaxBytes: 2048})
		if i == 99 {
			if !errors.Is(err, ingest.ErrBudget) {
				t.Fatal("101st policy allowed", err)
			}
		} else {
			must(t, err)
		}
	}
}

func TestResetPreservesSpacingPolicyAndCharges(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(fmt.Sprint("pending=", pending), func(t *testing.T) {
			s, _, now := pollSetup(t)
			p := operator(tenantA)
			started := *now
			claim, err := s.ClaimPoll(t.Context(), p)
			must(t, err)
			wantCharged := int64(2048)
			if !pending {
				must(t, s.FinishPoll(t.Context(), p, claim.ID, pollResult()))
				wantCharged = 500
			}
			*now = now.Add(time.Minute)
			must(t, s.ResetPollSchedule(t.Context(), p, sourceID))
			var approved, enabled int
			var next, charged int64
			var category string
			must(t, s.readers.QueryRowContext(t.Context(), "SELECT approved,enabled,next_at,last_error FROM poll_sources").Scan(&approved, &enabled, &next, &category))
			must(t, s.readers.QueryRowContext(t.Context(), "SELECT charged_bytes FROM poll_attempts WHERE id=?", claim.ID).Scan(&charged))
			if approved != 1 || enabled != 1 || charged != wantCharged || next != started.Add(time.Hour).UnixMicro() {
				t.Fatal(approved, enabled, charged, next)
			}
			if pending {
				if category != "reset" {
					t.Fatal("stale pending source", category)
				}
				if err = s.FinishPoll(t.Context(), p, claim.ID, pollResult()); !errors.Is(err, ingest.ErrLease) {
					t.Fatal("reset worker published", err)
				}
				if _, err = s.ClaimPoll(t.Context(), p); !errors.Is(err, ingest.ErrBusy) {
					t.Fatal("reservation released early", err)
				}
			}
			*now = started.Add(ingest.LeaseDuration + time.Second)
			if _, err = s.ClaimPoll(t.Context(), p); !errors.Is(err, ingest.ErrIdle) {
				t.Fatal("reset waived minimum interval", err)
			}
			must(t, s.readers.QueryRowContext(t.Context(), "SELECT charged_bytes FROM poll_attempts WHERE id=?", claim.ID).Scan(&charged))
			if charged != wantCharged {
				t.Fatal("recovery refunded work", charged)
			}
			*now = started.Add(time.Hour)
			_, err = s.ClaimPoll(t.Context(), p)
			must(t, err)
		})
	}
	s, _, _ := pollSetup(t)
	p := operator(tenantA)
	must(t, s.ConfigurePoll(t.Context(), p, ingest.Policy{SourceID: sourceID, URL: "https://example.invalid/feed", Interval: time.Hour, MaxBytes: 2048}))
	must(t, s.ResetPollSchedule(t.Context(), p, sourceID))
	var approved, enabled, attempts int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT approved,enabled FROM poll_sources").Scan(&approved, &enabled))
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM poll_attempts").Scan(&attempts))
	if approved != 0 || enabled != 0 || attempts != 0 {
		t.Fatal("reset changed disabled policy", approved, enabled, attempts)
	}
	if _, err := s.ClaimPoll(t.Context(), p); !errors.Is(err, ingest.ErrIdle) {
		t.Fatal("reset enabled source", err)
	}
}
