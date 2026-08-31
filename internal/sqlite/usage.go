package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"github.com/jonesrussell/northway/internal/usage"
)

func (s *Store) SetBudget(ctx context.Context, principal identity.Principal, limitMicros int64) error {
	tenant, err := principal.RequireOperator()
	if err != nil {
		return err
	}
	if limitMicros < 0 {
		return query.ErrInvalid
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		n, err := q.SetBudget(ctx, sqlc.SetBudgetParams{TenantID: string(tenant), LimitMicros: limitMicros})
		if err != nil {
			return err
		}
		if n != 1 {
			return usage.ErrLimit
		}
		return nil
	})
}

func (s *Store) GetBudget(ctx context.Context, principal identity.Principal) (usage.Budget, error) {
	tenant, err := principal.RequireOperator()
	if err != nil {
		return usage.Budget{}, err
	}
	v, err := sqlc.New(s.readers).GetBudget(ctx, string(tenant))
	if errors.Is(err, sql.ErrNoRows) {
		return usage.Budget{}, nil
	}
	return usage.Budget{LimitMicros: v.LimitMicros, SpentMicros: v.SpentMicros, HeldMicros: v.HeldMicros}, err
}

func changed(n int64, err error) error {
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	return nil
}

// These are tenant-bound operator seams, not public feed-management endpoints.
func (s *Store) SetSourceEnabled(ctx context.Context, principal identity.Principal, id string, enabled bool) error {
	tenant, err := access(principal, true, id)
	if err != nil {
		return err
	}
	var flag int64
	if enabled {
		flag = 1
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		return changed(q.SetSourceEnabled(ctx, sqlc.SetSourceEnabledParams{TenantID: string(tenant), ID: id, Enabled: flag}))
	})
}

func (s *Store) SetFeedEnabled(ctx context.Context, principal identity.Principal, id string, enabled bool) error {
	tenant, err := access(principal, true, id)
	if err != nil {
		return err
	}
	var flag int64
	if enabled {
		flag = 1
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		return changed(q.SetFeedEnabled(ctx, sqlc.SetFeedEnabledParams{TenantID: string(tenant), ID: id, Enabled: flag}))
	})
}

func (s *Store) DetachSource(ctx context.Context, principal identity.Principal, feedID, sourceID string) error {
	tenant, err := access(principal, true, feedID, sourceID)
	if err != nil {
		return err
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		return changed(q.DetachSource(ctx, sqlc.DetachSourceParams{TenantID: string(tenant), FeedID: feedID, SourceID: sourceID}))
	})
}
