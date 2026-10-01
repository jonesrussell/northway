package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"time"
)

func (s *Store) ConsumeAssertion(ctx context.Context, issuer, id string, tenant identity.TenantID, expires, now time.Time) error {
	if issuer == "" || len(issuer) > 256 || identity.ValidateID(id) != nil || tenant.Validate() != nil || !validTimestamp(now) || !validTimestamp(expires) || expires.Before(now) || expires.After(now.Add(70*time.Second)) {
		return identity.ErrUnauthorized
	}
	return s.writeOperational(ctx, func(q *sqlc.Queries) error {
		if err := q.CleanAssertionReplays(ctx, now.Unix()); err != nil {
			return err
		}
		count, err := q.CountAssertionReplays(ctx)
		if err != nil {
			return err
		}
		if count >= 4096 {
			return identity.ErrUnavailable
		}
		n, err := q.ConsumeAssertion(ctx, sqlc.ConsumeAssertionParams{Issuer: issuer, ID: id, ExpiresAt: expires.Unix()})
		if err != nil {
			return err
		}
		if n != 1 {
			return identity.ErrUnauthorized
		}
		return nil
	})
}

func (s *Store) EnsureWorkspace(ctx context.Context, p identity.Principal) error {
	tenant, err := p.RequireManagement()
	if err != nil {
		return err
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		_, err := q.RequireCustomerWorkspace(ctx, string(tenant))
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		n, err := q.TenantExists(ctx, string(tenant))
		if err != nil {
			return err
		}
		if n != 0 {
			return identity.ErrForbidden
		} // Existing pilot requires explicit migration, never automatic adoption.
		now := time.Now().UTC().UnixMicro()
		if err := q.CreateTenant(ctx, sqlc.CreateTenantParams{ID: string(tenant), CreatedAt: now}); err != nil {
			return err
		}
		return q.EnsureCustomerWorkspace(ctx, sqlc.EnsureCustomerWorkspaceParams{TenantID: string(tenant), CreatedAt: now})
	})
}

func requireWorkspace(ctx context.Context, q *sqlc.Queries, tenant identity.TenantID) error {
	_, err := q.RequireCustomerWorkspace(ctx, string(tenant))
	if errors.Is(err, sql.ErrNoRows) {
		return identity.ErrNotFound
	}
	return err
}

func (s *Store) IssueCustomerKey(ctx context.Context, p identity.Principal, scopes identity.Scopes) (identity.KeyMetadata, identity.Secret, error) {
	tenant, err := p.RequireManagement()
	if err != nil {
		return identity.KeyMetadata{}, identity.Secret{}, err
	}
	k, secret, err := identity.GenerateManagedKey(p, scopes)
	if err != nil {
		return identity.KeyMetadata{}, identity.Secret{}, err
	}
	expires := k.CreatedAt.Add(30 * 24 * time.Hour)
	err = s.write(ctx, func(q *sqlc.Queries) error {
		if err := requireWorkspace(ctx, q, tenant); err != nil {
			return err
		}
		n, err := q.CountCustomerKeys(ctx, sqlc.CountCustomerKeysParams{TenantID: string(tenant), ExpiresAt: k.CreatedAt.UnixMicro()})
		if err != nil {
			return err
		}
		if n >= 5 {
			return identity.ErrForbidden
		}
		if err := q.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{ID: k.ID, TenantID: string(tenant), Digest: k.Digest[:], Scopes: int64(k.Scopes), CreatedAt: k.CreatedAt.UnixMicro()}); err != nil {
			return err
		}
		return q.CreateCustomerKeyExpiry(ctx, sqlc.CreateCustomerKeyExpiryParams{KeyID: k.ID, ExpiresAt: expires.UnixMicro()})
	})
	if err != nil {
		return identity.KeyMetadata{}, identity.Secret{}, err
	}
	return identity.KeyMetadata{ID: k.ID, Scopes: scopes.Names(), CreatedAt: k.CreatedAt, ExpiresAt: expires}, secret, nil
}

func (s *Store) ListCustomerKeys(ctx context.Context, p identity.Principal) ([]identity.KeyMetadata, error) {
	tenant, err := p.RequireManagement()
	if err != nil {
		return nil, err
	}
	q := sqlc.New(s.readers)
	if err := requireWorkspace(ctx, q, tenant); err != nil {
		return nil, err
	}
	rows, err := q.ListCustomerKeys(ctx, sqlc.ListCustomerKeysParams{TenantID: string(tenant), Now: time.Now().UTC().UnixMicro()})
	if err != nil {
		return nil, err
	}
	out := make([]identity.KeyMetadata, 0, len(rows))
	for _, r := range rows {
		m := identity.KeyMetadata{ID: r.ID, Scopes: identity.Scopes(r.Scopes).Names(), CreatedAt: time.UnixMicro(r.CreatedAt).UTC(), ExpiresAt: time.UnixMicro(r.ExpiresAt).UTC()}
		if r.LastUsedAt.Valid {
			v := time.UnixMicro(r.LastUsedAt.Int64).UTC()
			m.LastUsedAt = &v
		}
		if r.RevokedAt.Valid {
			v := time.UnixMicro(r.RevokedAt.Int64).UTC()
			m.RevokedAt = &v
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *Store) RevokeCustomerKey(ctx context.Context, p identity.Principal, id string) error {
	tenant, err := p.RequireManagement()
	if err != nil {
		return err
	}
	if !identity.ValidKeyID(id) {
		return identity.ErrNotFound
	}
	return s.writeOperational(ctx, func(q *sqlc.Queries) error {
		if err := requireWorkspace(ctx, q, tenant); err != nil {
			return err
		}
		// Only externally issued customer keys are manageable here, never pilot/operator keys.
		if _, err := q.CustomerKeyExpiry(ctx, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return identity.ErrNotFound
			}
			return err
		}
		n, err := q.RevokeAPIKey(ctx, sqlc.RevokeAPIKeyParams{TenantID: string(tenant), ID: id, MAX: time.Now().UTC().UnixMicro()})
		if err != nil {
			return err
		}
		if n == 0 {
			return identity.ErrNotFound
		}
		return nil
	})
}

func (s *Store) ListCustomerFeeds(ctx context.Context, p identity.Principal) ([]identity.FeedMetadata, error) {
	tenant, err := p.Require(identity.FeedsRead)
	if err != nil {
		return nil, err
	}
	rows, err := sqlc.New(s.readers).ListCustomerFeeds(ctx, string(tenant))
	if err != nil {
		return nil, err
	}
	out := make([]identity.FeedMetadata, 0, len(rows))
	for _, r := range rows {
		out = append(out, identity.FeedMetadata{ID: r.ID, Title: r.Title})
	}
	return out, nil
}

// TakeRequestBudget is shared by browser assertions and API keys for the same
// tenant. Durable fixed-minute limit: 60 authenticated requests, across restart.
func (s *Store) TakeRequestBudget(ctx context.Context, p identity.Principal, now time.Time) (bool, error) {
	tenant, err := p.Require(identity.FeedsRead)
	if err != nil {
		tenant, err = p.Require(identity.FeedbackWrite)
	}
	if err != nil {
		return false, err
	}
	var n int64
	err = s.writeOperational(ctx, func(q *sqlc.Queries) error {
		if err := q.CleanRequestBudgets(ctx, now.Unix()/60-1); err != nil {
			return err
		}
		var err error
		n, err = q.ConsumeRequestBudget(ctx, sqlc.ConsumeRequestBudgetParams{TenantID: string(tenant), Window: now.Unix() / 60})
		return err
	})
	return n == 1, err
}
