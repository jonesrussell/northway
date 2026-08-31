package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jonesrussell/northway/internal/feedback"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
)

// RecordFeedback authorizes with feedback:write alone. Snapshot membership,
// current access, event replay/reversal and revision change share one write
// transaction. No tenant or identity is accepted from the event payload.
func (s *Store) RecordFeedback(ctx context.Context, p identity.Principal, e feedback.Event) error {
	tenant, err := p.Require(identity.FeedbackWrite)
	if err != nil {
		return err
	}
	if err := e.Validate(); err != nil {
		return err
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		now := s.queryTime()
		if !validTimestamp(now) {
			return feedback.ErrInvalid
		}
		snap, err := loadSnapshot(ctx, q, string(tenant), e.SnapshotID, now)
		if err != nil {
			return err
		}
		member := false
		for _, item := range snap.Items {
			if item.ArticleID == e.ArticleID {
				member = true
				break
			}
		}
		if !member {
			return ErrNotFound
		}
		old, err := q.FeedbackEvent(ctx, sqlc.FeedbackEventParams{TenantID: string(tenant), ID: e.EventID})
		if err == nil {
			if old.SnapshotID == e.SnapshotID && old.ArticleID == e.ArticleID && old.Action == e.Action && old.ReversesEventID.String == e.ReversesEventID {
				return nil
			}
			return feedback.ErrConflict
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if e.Action == "undo" {
			target, err := q.FeedbackEvent(ctx, sqlc.FeedbackEventParams{TenantID: string(tenant), ID: e.ReversesEventID})
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if target.SnapshotID != e.SnapshotID || target.ArticleID != e.ArticleID || target.Action == "undo" {
				return feedback.ErrConflict
			}
			_, err = q.FeedbackReversal(ctx, sqlc.FeedbackReversalParams{TenantID: string(tenant), ReversesEventID: sql.NullString{String: e.ReversesEventID, Valid: true}})
			if err == nil {
				return feedback.ErrConflict
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		n, err := q.CreateFeedbackEvent(ctx, sqlc.CreateFeedbackEventParams{TenantID: string(tenant), ID: e.EventID, SnapshotID: e.SnapshotID, ArticleID: e.ArticleID, FeedID: snap.FeedID, Action: e.Action, ReversesEventID: sql.NullString{String: e.ReversesEventID, Valid: e.ReversesEventID != ""}, CreatedAt: now.UnixMicro()})
		if err != nil {
			return err
		}
		if n != 1 {
			return feedback.ErrConflict
		}
		n, err = q.AdvanceFeedbackRevision(ctx, sqlc.AdvanceFeedbackRevisionParams{TenantID: string(tenant), ID: snap.FeedID})
		if err != nil {
			return err
		}
		if n != 1 {
			return feedback.ErrConflict
		}
		return nil
	})
}
