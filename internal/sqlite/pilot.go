package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
)

type PilotSource struct {
	ID, URL, Title string
	PublisherGroup string
	Categories     []string
	FeedIDs        []string
	Interval       time.Duration
	MaxBytes       int64
}
type PilotFeed struct {
	ID, Title    string
	Categories   []string
	PublisherCap int
	UseContext   bool
}

// ProvisionPilot atomically creates an exact reviewed profile and its poll policies.
// Existing rows must match byte-for-byte; this never adopts or rewrites other data.
func (s *Store) ProvisionPilot(ctx context.Context, p identity.Principal, sources []PilotSource, feeds []PilotFeed) error {
	tenant, err := p.RequireOperator()
	if err != nil {
		return err
	}
	if len(sources) == 0 || len(sources) > ingest.MaxSources || len(feeds) == 0 || len(feeds) > 20 {
		return ingest.ErrInvalid
	}
	feedSet, sourceSet := map[string]bool{}, map[string]bool{}
	for _, f := range feeds {
		if len(f.Categories) == 0 || identity.ValidateID(f.ID) != nil || !text(f.Title, 512, false) || feedSet[f.ID] || (feed.Preferences{Categories: f.Categories, Sources: []feed.SourceRule{{SourceID: "00000000-0000-4000-8000-000000000000", PublisherGroup: "validation", Categories: f.Categories[:1]}}, Exclude: []string{}, UseContext: f.UseContext, PublisherCap: f.PublisherCap}).Validate() != nil {
			return ingest.ErrInvalid
		}
		feedSet[f.ID] = true
	}
	for _, v := range sources {
		if identity.ValidateID(v.ID) != nil || sourceSet[v.ID] || !pollURL(v.URL) || !text(v.Title, 512, false) || v.Interval < time.Hour || v.Interval > 7*24*time.Hour || v.MaxBytes < 1024 || v.MaxBytes > ingest.MaxResponseBytes || len(v.FeedIDs) == 0 {
			return ingest.ErrInvalid
		}
		if !text(v.PublisherGroup, 64, false) || len(v.Categories) != 1 || !feed.Category(v.Categories[0]) {
			return ingest.ErrInvalid
		}
		sourceSet[v.ID] = true
		seen := map[string]bool{}
		for _, id := range v.FeedIDs {
			if !feedSet[id] || seen[id] {
				return ingest.ErrInvalid
			}
			seen[id] = true
		}
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		now := s.queryTime()
		if !validTimestamp(now) {
			return ingest.ErrInvalid
		}
		for _, f := range feeds {
			if err := q.EnsurePilotFeed(ctx, sqlc.EnsurePilotFeedParams{TenantID: string(tenant), ID: f.ID, Title: f.Title}); err != nil {
				return err
			}
			row, err := q.PilotFeedConfig(ctx, sqlc.PilotFeedConfigParams{TenantID: string(tenant), ID: f.ID})
			if err != nil {
				return err
			}
			if row.Title != f.Title || row.Enabled != 1 {
				return errors.New("existing pilot feed conflicts with reviewed profile")
			}
		}
		for _, v := range sources {
			if err := q.EnsurePilotSource(ctx, sqlc.EnsurePilotSourceParams{TenantID: string(tenant), ID: v.ID, Url: v.URL, Title: v.Title}); err != nil {
				return err
			}
			row, err := q.PilotSourceConfig(ctx, sqlc.PilotSourceConfigParams{TenantID: string(tenant), ID: v.ID})
			if err != nil {
				return err
			}
			if row.Url != v.URL || row.Title != v.Title || row.Enabled != 1 {
				return errors.New("existing pilot source conflicts with reviewed profile")
			}
			for _, feedID := range v.FeedIDs {
				if err := q.EnsurePilotFeedSource(ctx, sqlc.EnsurePilotFeedSourceParams{TenantID: string(tenant), FeedID: feedID, SourceID: v.ID}); err != nil {
					return err
				}
			}
			poll, err := q.PilotPollConfig(ctx, sqlc.PilotPollConfigParams{TenantID: string(tenant), SourceID: v.ID})
			if errors.Is(err, sql.ErrNoRows) {
				if err = q.ConfigurePoll(ctx, sqlc.ConfigurePollParams{TenantID: string(tenant), SourceID: v.ID, ApprovedUrl: v.URL, Approved: 1, Enabled: 1, IntervalUs: v.Interval.Microseconds(), MaxBytes: v.MaxBytes, NextAt: now.UnixMicro()}); err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else if poll.ApprovedUrl != v.URL || poll.Approved != 1 || poll.Enabled != 1 || poll.IntervalUs != v.Interval.Microseconds() || poll.MaxBytes != v.MaxBytes {
				return errors.New("existing poll policy conflicts with reviewed profile")
			}
		}
		for _, f := range feeds {
			pref := feed.Preferences{Categories: f.Categories, Sources: []feed.SourceRule{}, Exclude: []string{}, UseContext: f.UseContext, PublisherCap: f.PublisherCap}
			for _, v := range sources {
				for _, feedID := range v.FeedIDs {
					if feedID == f.ID {
						pref.Sources = append(pref.Sources, feed.SourceRule{SourceID: v.ID, PublisherGroup: v.PublisherGroup, Categories: v.Categories})
					}
				}
			}
			if pref.Validate() != nil {
				return ingest.ErrInvalid
			}
			encoded, err := json.Marshal(pref)
			if err != nil {
				return err
			}
			current, err := q.PilotFeedConfig(ctx, sqlc.PilotFeedConfigParams{TenantID: string(tenant), ID: f.ID})
			if err != nil {
				return err
			}
			if current.Preferences == string(encoded) {
				continue
			}
			if current.Preferences != "" {
				return errors.New("existing feed preferences conflict with reviewed profile")
			}
			n, err := q.ConfigureFeedPreferences(ctx, sqlc.ConfigureFeedPreferencesParams{Preferences: string(encoded), TenantID: string(tenant), ID: f.ID})
			if err != nil {
				return err
			}
			if n != 1 {
				return ErrNotFound
			}
		}
		return nil
	})
}
