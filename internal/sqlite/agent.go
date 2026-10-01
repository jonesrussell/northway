package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"time"
)

func (s *Store) CreateAgentGrant(ctx context.Context, p identity.Principal, g identity.AgentGrant) error {
	tenant, e := p.RequireOperator()
	if e != nil {
		return e
	}
	now := time.Now().UTC()
	if g.TenantID != tenant || !identity.ValidKeyID(g.ID) || !g.Scopes.Valid() || g.Digest == [32]byte{} || g.RevokedAt != nil || !validTimestamp(g.CreatedAt) || !g.ExpiresAt.After(now) || g.CreatedAt.After(now) || g.ExpiresAt.Sub(g.CreatedAt) > 24*time.Hour || g.ExpiresAt.Before(g.CreatedAt) || !identity.ValidAgentLabel(g.Label) {
		return identity.ErrForbidden
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		n, e := q.AgentGrantCount(ctx, string(tenant))
		if e != nil {
			return e
		}
		if n >= 100 {
			return identity.ErrForbidden
		}
		return q.CreateAgentGrant(ctx, sqlc.CreateAgentGrantParams{ID: g.ID, TenantID: string(tenant), Digest: g.Digest[:], Scopes: int64(g.Scopes), CreatedAt: g.CreatedAt.UnixMicro(), ExpiresAt: g.ExpiresAt.UnixMicro(), Label: g.Label})
	})
}
func (s *Store) LookupAgentGrant(ctx context.Context, id string) (identity.AgentGrant, error) {
	if !identity.ValidKeyID(id) {
		return identity.AgentGrant{}, identity.ErrUnauthorized
	}
	row, e := s.queries(s.readers).LookupAgentGrant(ctx, id)
	if errors.Is(e, sql.ErrNoRows) {
		return identity.AgentGrant{}, identity.ErrUnauthorized
	}
	if e != nil {
		return identity.AgentGrant{}, e
	}
	if len(row.Digest) != 32 {
		return identity.AgentGrant{}, identity.ErrUnauthorized
	}
	g := identity.AgentGrant{ID: row.ID, TenantID: identity.TenantID(row.TenantID), Scopes: identity.CollectionScopes(row.Scopes), CreatedAt: time.UnixMicro(row.CreatedAt).UTC(), ExpiresAt: time.UnixMicro(row.ExpiresAt).UTC(), Label: row.Label}
	copy(g.Digest[:], row.Digest)
	if row.RevokedAt.Valid {
		at := time.UnixMicro(row.RevokedAt.Int64).UTC()
		g.RevokedAt = &at
	}
	return g, nil
}
func (s *Store) TouchAgentGrant(ctx context.Context, tenant identity.TenantID, id string, now time.Time) (bool, error) {
	if tenant.Validate() != nil || !identity.ValidKeyID(id) || !validTimestamp(now) {
		return false, identity.ErrUnauthorized
	}
	var n int64
	e := s.writeOperational(ctx, func(q *sqlc.Queries) error {
		var e error
		n, e = q.TouchAgentGrant(ctx, sqlc.TouchAgentGrantParams{MAX: now.UnixMicro(), TenantID: string(tenant), ID: id, CreatedAt: now.UnixMicro(), ExpiresAt: now.UnixMicro()})
		return e
	})
	return n == 1, e
}
func (s *Store) RevokeAgentGrant(ctx context.Context, p identity.Principal, id string) error {
	tenant, e := p.RequireOperator()
	if e != nil {
		return e
	}
	if !identity.ValidKeyID(id) {
		return identity.ErrNotFound
	}
	return s.writeOperational(ctx, func(q *sqlc.Queries) error {
		n, e := q.RevokeAgentGrant(ctx, sqlc.RevokeAgentGrantParams{MAX: time.Now().UTC().UnixMicro(), TenantID: string(tenant), ID: id})
		if e != nil {
			return e
		}
		if n != 1 {
			return identity.ErrNotFound
		}
		return nil
	})
}

func (s *Store) AgentGrantIssueReady(ctx context.Context, p identity.Principal) error {
	tenant, e := p.RequireOperator()
	if e != nil {
		return e
	}
	if e = s.RequireTenant(ctx, p); e != nil {
		return e
	}
	q := s.queries(s.readers)
	state, e := q.CustomerWorkspaceState(ctx, string(tenant))
	if e == nil && state != "active" {
		return identity.ErrForbidden
	}
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	count, e := q.AgentGrantCount(ctx, string(tenant))
	if e != nil {
		return e
	}
	if count >= 100 {
		return identity.ErrForbidden
	}
	return nil
}
