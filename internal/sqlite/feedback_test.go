package sqlite

import (
	"context"
	"errors"
	"fmt"
	"github.com/jonesrussell/northway/internal/feedback"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"sync"
	"testing"
	"time"
)

func feedbackFixture(t *testing.T) (*Store, string, identity.Principal, query.Snapshot) {
	t.Helper()
	s, path, p, pref := retrievalFixture(t, "world")
	src := addRetrievalSource(t, s, p, &pref, 101, "publisher", "world")
	must(t, s.ConfigureFeedPreferences(t.Context(), p, feedID, pref))
	addRetrievalItem(t, s, p, 201, src, "World headline", "https://example.invalid/world", nil, queryEpoch.Add(-time.Hour))
	return s, path, p, runRetrieval(t, s, p, "feedback-query-original", recentContext(), 24, 5)
}
func feedbackRevision(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT revision FROM feeds WHERE tenant_id=? AND id=?", tenantA, feedID).Scan(&n))
	return n
}
func feedbackCount(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM feedback_events").Scan(&n))
	return n
}
func TestFeedbackReplayReversalAndCacheRevision(t *testing.T) {
	s, _, p, snap := feedbackFixture(t)
	ctx := t.Context()
	before := feedbackRevision(t, s)
	e := feedback.Event{EventID: rid(301), SnapshotID: snap.ID, ArticleID: rid(201), Action: "save"}
	const workers = 12
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() { errs <- s.RecordFeedback(ctx, p, e) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		must(t, err)
	}
	if feedbackCount(t, s) != 1 || feedbackRevision(t, s) != before+1 {
		t.Fatal("replay changed state more than once")
	}
	changed := e
	changed.Action = "dismiss"
	if !errors.Is(s.RecordFeedback(ctx, p, changed), feedback.ErrConflict) {
		t.Fatal("payload conflict accepted")
	}
	next := runRetrieval(t, s, p, "after-feedback-new-key", recentContext(), 24, 5)
	if next.ID == snap.ID || next.FeedRevision != snap.FeedRevision+1 {
		t.Fatal("feedback did not invalidate cache")
	}
	replay := runRetrieval(t, s, p, "feedback-query-original", recentContext(), 24, 5)
	if replay.ID != snap.ID {
		t.Fatal("feedback reranked replay")
	}
	undo := feedback.Event{EventID: rid(302), SnapshotID: snap.ID, ArticleID: rid(201), Action: "undo", ReversesEventID: e.EventID}
	other := undo
	other.EventID = rid(303)
	errs = make(chan error, 2)
	wg.Go(func() { errs <- s.RecordFeedback(ctx, p, undo) })
	wg.Go(func() { errs <- s.RecordFeedback(ctx, p, other) })
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, feedback.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 || feedbackCount(t, s) != 2 || feedbackRevision(t, s) != before+2 {
		t.Fatal("reversal was not atomic")
	}
	// Replay original remains a no-op after reversal; it must not reapply a save.
	must(t, s.RecordFeedback(ctx, p, e))
	if feedbackRevision(t, s) != before+2 {
		t.Fatal("replay reapplied reversed event")
	}
}
func TestFeedbackInvalidUnavailableMembershipAndRollback(t *testing.T) {
	for _, failure := range []string{"ABORT", "IGNORE"} {
		t.Run(failure, func(t *testing.T) {
			s, _, p, snap := feedbackFixture(t)
			before := feedbackRevision(t, s)
			e := feedback.Event{EventID: rid(301), SnapshotID: snap.ID, ArticleID: rid(201), Action: "dismiss"}
			_, err := s.writer.ExecContext(t.Context(), fmt.Sprintf("CREATE TRIGGER reject_feedback_revision BEFORE UPDATE OF revision ON feeds BEGIN SELECT RAISE(%s%s); END", failure, map[string]string{"ABORT": ", 'fixture'"}[failure]))
			must(t, err)
			if s.RecordFeedback(t.Context(), p, e) == nil {
				t.Fatal("failed revision accepted")
			}
			if feedbackCount(t, s) != 0 || feedbackRevision(t, s) != before {
				t.Fatal("partial event commit")
			}
			_, err = s.writer.ExecContext(t.Context(), "DROP TRIGGER reject_feedback_revision")
			must(t, err)
			must(t, s.RecordFeedback(t.Context(), p, e))
		})
	}
	s, _, p, snap := feedbackFixture(t)
	e := feedback.Event{EventID: rid(301), SnapshotID: snap.ID, ArticleID: rid(999), Action: "save"}
	if !errors.Is(s.RecordFeedback(t.Context(), p, e), ErrNotFound) {
		t.Fatal("invented membership accepted")
	}
	e.ArticleID = rid(201)
	e.Action = "undo"
	e.ReversesEventID = rid(999)
	if !errors.Is(s.RecordFeedback(t.Context(), p, e), ErrNotFound) {
		t.Fatal("unknown reversal accepted")
	}
	e.Action = "save"
	e.ReversesEventID = ""
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if s.RecordFeedback(ctx, p, e) == nil || feedbackCount(t, s) != 0 {
		t.Fatal("cancelled event persisted")
	}
	// An item can disappear from corpus while retained evidence is still valid.
	must(t, s.DeleteArticle(t.Context(), p, e.ArticleID))
	must(t, s.RecordFeedback(t.Context(), p, e))
	must(t, s.SetSourceEnabled(t.Context(), p, rid(101), false))
	if !errors.Is(s.RecordFeedback(t.Context(), p, e), ErrNotFound) {
		t.Fatal("revoked replay accepted")
	}
	must(t, s.SetSourceEnabled(t.Context(), p, rid(101), true))
	s.clock = func() time.Time { return queryEpoch.Add(queryRetention + time.Hour) }
	if !errors.Is(s.RecordFeedback(t.Context(), p, e), ErrNotFound) {
		t.Fatal("expired snapshot feedback accepted")
	}
}
