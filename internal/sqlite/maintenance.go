package sqlite

import (
	"context"
	"errors"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
)

const (
	queryRetention  = 7 * 24 * time.Hour
	corpusRetention = 90 * 24 * time.Hour
	pollRetention   = 24 * time.Hour
)

// MaintenanceReport contains bounded operational counts only. It never
// includes tenant, source, article, request, or provider data.
type MaintenanceReport struct {
	QueryWork       int64
	Snapshots       int64
	ArticleVersions int64
	Articles        int64
	PollAttempts    int64
	Unreconciled    int64
	WALBusy         bool
}

// Maintain removes only records whose published retention window is complete.
// Uncertain provider charges and user feedback are evidence and are preserved.
func (s *Store) Maintain(ctx context.Context, principal identity.Principal) (MaintenanceReport, error) {
	tenant, err := principal.RequireOperator()
	if err != nil {
		return MaintenanceReport{}, err
	}
	now := s.queryTime()
	if !validTimestamp(now) {
		return MaintenanceReport{}, errors.New("invalid maintenance time")
	}
	var report MaintenanceReport
	err = s.writeOperational(ctx, func(q *sqlc.Queries) error {
		var err error
		report.QueryWork, err = q.DeleteExpiredQueryWork(ctx, sqlc.DeleteExpiredQueryWorkParams{TargetTenant: string(tenant), BeforeAt: now.UnixMicro()})
		if err != nil {
			return err
		}
		report.Snapshots, err = q.DeleteExpiredQuerySnapshots(ctx, sqlc.DeleteExpiredQuerySnapshotsParams{TargetTenant: string(tenant), BeforeAt: now.UnixMicro()})
		if err != nil {
			return err
		}
		report.ArticleVersions, err = q.DeleteSupersededArticleVersions(ctx, sqlc.DeleteSupersededArticleVersionsParams{TargetTenant: string(tenant), BeforeAt: now.Add(-corpusRetention).UnixMicro()})
		if err != nil {
			return err
		}
		report.Articles, err = q.DeleteExpiredArticles(ctx, sqlc.DeleteExpiredArticlesParams{TargetTenant: string(tenant), BeforeAt: now.Add(-corpusRetention).UnixMicro()})
		if err != nil {
			return err
		}
		report.PollAttempts, err = q.DeleteExpiredPollAttempts(ctx, sqlc.DeleteExpiredPollAttemptsParams{TargetTenant: string(tenant), BeforeAt: now.Add(-pollRetention).UnixMicro()})
		if err != nil {
			return err
		}
		report.Unreconciled, err = q.UnreconciledQueryWork(ctx, string(tenant))
		return err
	})
	if err != nil {
		return MaintenanceReport{}, err
	}
	var busy, logPages, checkpointed int
	if err := s.checkpoint(ctx, &busy, &logPages, &checkpointed); err != nil {
		return report, err
	}
	report.WALBusy = busy != 0
	return report, nil
}

func (s *Store) checkpoint(ctx context.Context, busy, logPages, checkpointed *int) error {
	select {
	case s.writeGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.writeGate }()
	return s.writer.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(busy, logPages, checkpointed)
}

// PollHealthy checks persisted source freshness and lease state. The caller
// receives one boolean; no publisher URL, response, or error context escapes.
func (s *Store) PollHealthy(ctx context.Context, principal identity.Principal) (bool, error) {
	tenant, err := principal.RequireOperator()
	if err != nil {
		return false, err
	}
	now := s.queryTime()
	if !validTimestamp(now) {
		return false, errors.New("invalid poll health time")
	}
	count, err := sqlc.New(s.readers).UnhealthyPollSources(ctx, sqlc.UnhealthyPollSourcesParams{TargetTenant: string(tenant), NowAt: now.UnixMicro()})
	return count == 0, err
}
