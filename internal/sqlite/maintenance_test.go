package sqlite

import (
	"errors"
	"testing"
	"time"

	"github.com/jonesrussell/northway/internal/article"
	"github.com/jonesrussell/northway/internal/feedback"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/query"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
)

func TestMaintenancePrunesEligibleTenantDataAndPreservesEvidence(t *testing.T) {
	s, _, p, selection := queryFixture(t)
	op := operator(tenantA)

	done := claim(t, s, p, "maintenance-complete-key")
	must(t, s.StartProvider(t.Context(), p, done.WorkID))
	snapshot := complete(t, s, p, done, selection, query.Settlement{Known: true, ActualMicros: 1})
	uncertainRequest := request()
	uncertainRequest.Context.Intent = "different bounded maintenance fixture"
	uncertain, err := s.BeginQuery(t.Context(), p, "maintenance-uncertain-key", uncertainRequest, policy())
	must(t, err)
	must(t, s.StartProvider(t.Context(), p, uncertain.WorkID))
	must(t, s.FailQuery(t.Context(), p, uncertain.WorkID))

	must(t, s.ConfigurePoll(t.Context(), op, ingest.Policy{SourceID: sourceID, URL: "https://example.invalid/feed", Approved: true, Enabled: true, Interval: time.Hour, MaxBytes: 2048}))
	poll, err := s.ClaimPoll(t.Context(), op)
	must(t, err)
	must(t, s.FinishPoll(t.Context(), op, poll.ID, pollResult()))

	seed(t, s, tenantB)
	other := item()
	other.ID = "00000006-0000-4000-8000-000000000000"
	other.OriginID = "other-old"
	other.ObservedAt = queryEpoch.Add(-100 * 24 * time.Hour)
	must(t, s.PutArticle(t.Context(), operator(tenantB), other))

	old := article.Article{ID: "00000007-0000-4000-8000-000000000000", SourceID: sourceID, OriginID: "expired", URL: "https://example.invalid/expired", Title: "Expired", ObservedAt: queryEpoch.Add(-100 * 24 * time.Hour)}
	must(t, s.PutArticle(t.Context(), op, old))

	now := queryEpoch.Add(corpusRetention + 24*time.Hour)
	s.clock = func() time.Time { return now }
	current := item()
	current.Title = "Current version"
	current.ObservedAt = now
	must(t, s.PutArticle(t.Context(), op, current))

	report, err := s.Maintain(t.Context(), op)
	must(t, err)
	if report.QueryWork != 1 || report.Snapshots != 1 || report.ArticleVersions < 1 || report.Articles < 1 || report.PollAttempts != 1 || report.Unreconciled != 1 || report.WALBusy {
		t.Fatalf("unexpected report: %+v", report)
	}
	if _, err := s.GetSnapshot(t.Context(), p, snapshot.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired snapshot retained: %v", err)
	}
	var count int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM query_work WHERE tenant_id=? AND id=?", tenantA, uncertain.WorkID).Scan(&count))
	if count != 1 {
		t.Fatal("unreconciled provider evidence was deleted")
	}
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM articles WHERE tenant_id=? AND id=?", tenantB, other.ID).Scan(&count))
	if count != 1 {
		t.Fatal("maintenance crossed tenant boundary")
	}
	if len(search(t, s, tenantA, "Expired")) != 0 || len(search(t, s, tenantA, "Current")) != 1 {
		t.Fatal("corpus or FTS retention is inconsistent")
	}
	var versions int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM article_versions WHERE tenant_id=? AND article_id=?", tenantA, itemID).Scan(&versions))
	if versions != 1 {
		t.Fatalf("current article versions=%d, want 1", versions)
	}
	reused, err := s.BeginQuery(t.Context(), p, "maintenance-complete-key", request(), policy())
	must(t, err)
	if reused.WorkID == "" || reused.Snapshot != nil {
		t.Fatal("expired and physically removed key was not reusable")
	}

	report, err = s.Maintain(t.Context(), op)
	must(t, err)
	if report.QueryWork != 0 || report.Snapshots != 0 || report.ArticleVersions != 0 || report.Articles != 0 || report.PollAttempts != 0 || report.Unreconciled != 1 {
		t.Fatalf("maintenance was not idempotent: %+v", report)
	}
	if _, err := s.Maintain(t.Context(), identity.Principal{}); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("unauthorized maintenance: %v", err)
	}
}

func TestPollHealthUsesPersistedFreshnessFailureAndLeaseState(t *testing.T) {
	s, _, now := pollSetup(t)
	op := operator(tenantA)
	healthy, err := s.PollHealthy(t.Context(), op)
	must(t, err)
	if healthy {
		t.Fatal("never-polled source reported healthy")
	}
	claim, err := s.ClaimPoll(t.Context(), op)
	must(t, err)
	must(t, s.FinishPoll(t.Context(), op, claim.ID, pollResult()))
	healthy, err = s.PollHealthy(t.Context(), op)
	must(t, err)
	if !healthy {
		t.Fatal("fresh successful source reported unhealthy")
	}
	successAt := *now
	must(t, s.ConfigurePoll(t.Context(), op, ingest.Policy{SourceID: sourceID, URL: "https://example.invalid/feed", Approved: true, Enabled: true, Interval: 7 * 24 * time.Hour, MaxBytes: 2048}))
	*now = successAt.Add(25 * time.Hour)
	report, err := s.Maintain(t.Context(), op)
	must(t, err)
	if report.PollAttempts != 1 {
		t.Fatalf("closed attempt cleanup=%d", report.PollAttempts)
	}
	healthy, err = s.PollHealthy(t.Context(), op)
	must(t, err)
	if !healthy {
		t.Fatal("deleting closed attempt broke persisted source health")
	}
	*now = successAt.Add(14*24*time.Hour + time.Microsecond)
	healthy, err = s.PollHealthy(t.Context(), op)
	must(t, err)
	if healthy {
		t.Fatal("stale source reported healthy")
	}
	claim, err = s.ClaimPoll(t.Context(), op)
	must(t, err)
	must(t, s.FinishPoll(t.Context(), op, claim.ID, ingest.Result{Status: 503, Failure: "http"}))
	healthy, err = s.PollHealthy(t.Context(), op)
	must(t, err)
	if healthy {
		t.Fatal("failed source reported healthy")
	}
	if _, err := s.PollHealthy(t.Context(), identity.Principal{}); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("unauthorized health check: %v", err)
	}
}

func TestMaintenancePreservesSnapshotForActiveSave(t *testing.T) {
	s, _, p, selection := queryFixture(t)
	c := claim(t, s, p, "maintenance-saved-snapshot")
	must(t, s.StartProvider(t.Context(), p, c.WorkID))
	snapshot := complete(t, s, p, c, selection, query.Settlement{Known: true})
	must(t, s.RecordFeedback(t.Context(), operator(tenantA), feedback.Event{EventID: "00000009-0000-4000-8000-000000000000", SnapshotID: snapshot.ID, ArticleID: itemID, Action: "save"}))
	s.reservePages = storageMaxPages
	if _, err := s.GetSnapshot(t.Context(), p, snapshot.ID); err != nil {
		t.Fatalf("snapshot read was blocked by write reserve: %v", err)
	}
	s.clock = func() time.Time { return queryEpoch.Add(queryRetention + time.Hour) }
	report, err := s.Maintain(t.Context(), operator(tenantA))
	must(t, err)
	if report.QueryWork != 1 || report.Snapshots != 0 {
		t.Fatalf("saved snapshot report: %+v", report)
	}
	var count int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM query_snapshots WHERE tenant_id=? AND id=?", tenantA, snapshot.ID).Scan(&count))
	if count != 1 {
		t.Fatal("active save lost its immutable snapshot evidence")
	}
}

func TestStorageReserveRejectsAdmissionButAllowsMaintenance(t *testing.T) {
	s, _ := fresh(t)
	seed(t, s, tenantA)
	op := operator(tenantA)
	must(t, s.ConfigurePoll(t.Context(), op, ingest.Policy{SourceID: sourceID, URL: "https://example.invalid/feed", Approved: true, Enabled: true, Interval: time.Hour, MaxBytes: 2048}))
	s.reservePages = storageMaxPages
	if err := s.PutArticle(t.Context(), op, item()); !errors.Is(err, ErrStoragePressure) {
		t.Fatalf("ordinary write crossed reserve: %v", err)
	}
	if _, err := s.ClaimPoll(t.Context(), op); !errors.Is(err, ingest.ErrCorpusFull) {
		t.Fatalf("poll did not surface storage pressure: %v", err)
	}
	if _, err := s.Maintain(t.Context(), op); err != nil {
		t.Fatalf("maintenance could not use reserve: %v", err)
	}
}

func TestStorageReserveAllowsExpiredQueryRecovery(t *testing.T) {
	s, _, p, _ := queryFixture(t)
	c := claim(t, s, p, "reserve-recovery-key")
	s.clock = func() time.Time { return queryEpoch.Add(2 * time.Minute) }
	s.reservePages = storageMaxPages
	recovered, err := s.RecoverQueries(t.Context(), operator(tenantA))
	must(t, err)
	if recovered != 1 {
		t.Fatalf("recovered=%d", recovered)
	}
	if _, err := s.BeginQuery(t.Context(), p, "new-pressure-key", request(), policy()); !errors.Is(err, ErrStoragePressure) {
		t.Fatalf("ordinary query crossed reserve: %v", err)
	}
	if c.WorkID == "" {
		t.Fatal("fixture did not create work")
	}
}

func TestCheckpointSerializesWithInProcessWriter(t *testing.T) {
	s, _ := fresh(t)
	entered, release := make(chan struct{}), make(chan struct{})
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- s.writeOperational(t.Context(), func(*sqlc.Queries) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	checkpointDone := make(chan error, 1)
	go func() {
		var busy, logPages, checkpointed int
		checkpointDone <- s.checkpoint(t.Context(), &busy, &logPages, &checkpointed)
	}()
	select {
	case err := <-checkpointDone:
		t.Fatalf("checkpoint bypassed writer gate: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	must(t, <-writeDone)
	must(t, <-checkpointDone)
}
