package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
)

func (s *Store) CreateAPIKey(ctx context.Context, principal identity.Principal, key identity.KeyRecord) error {
	tenant, err := principal.RequireOperator()
	if err != nil {
		return err
	}
	if key.TenantID != tenant || !identity.ValidKeyID(key.ID) || !key.Scopes.Valid() || !validTimestamp(key.CreatedAt) || key.Digest == [32]byte{} || key.LastUsedAt != nil || key.RevokedAt != nil {
		return errors.New("invalid key metadata")
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		return q.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{ID: key.ID, TenantID: string(tenant), Digest: key.Digest[:], Scopes: int64(key.Scopes), CreatedAt: key.CreatedAt.UnixMicro()})
	})
}

// LookupAPIKey is the sole cross-tenant credential lookup. It exposes no corpus
// data and is consumed only by identity.Service, never a public lookup endpoint.
func (s *Store) LookupAPIKey(ctx context.Context, id string) (identity.KeyRecord, error) {
	if !identity.ValidKeyID(id) {
		return identity.KeyRecord{}, identity.ErrUnauthorized
	}
	v, err := sqlc.New(s.readers).LookupAPIKey(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.KeyRecord{}, identity.ErrUnauthorized
	}
	if err != nil {
		return identity.KeyRecord{}, err
	}
	if len(v.Digest) != 32 || v.Scopes < 1 || v.Scopes > 3 {
		return identity.KeyRecord{}, identity.ErrUnauthorized
	}
	key := identity.KeyRecord{ID: v.ID, TenantID: identity.TenantID(v.TenantID), Scopes: identity.Scopes(v.Scopes), CreatedAt: time.UnixMicro(v.CreatedAt).UTC()}
	copy(key.Digest[:], v.Digest)
	if v.LastUsedAt.Valid {
		value := time.UnixMicro(v.LastUsedAt.Int64).UTC()
		key.LastUsedAt = &value
	}
	if v.RevokedAt.Valid {
		value := time.UnixMicro(v.RevokedAt.Int64).UTC()
		key.RevokedAt = &value
	}
	return key, nil
}

func (s *Store) TouchAPIKey(ctx context.Context, tenant identity.TenantID, id string, now time.Time) (bool, error) {
	if tenant.Validate() != nil || !identity.ValidKeyID(id) || !validTimestamp(now) {
		return false, identity.ErrUnauthorized
	}
	var affected int64
	err := s.write(ctx, func(q *sqlc.Queries) error {
		var err error
		affected, err = q.TouchAPIKey(ctx, sqlc.TouchAPIKeyParams{MAX: now.UnixMicro(), TenantID: string(tenant), ID: id})
		return err
	})
	return affected == 1, err
}

func (s *Store) RevokeAPIKey(ctx context.Context, principal identity.Principal, id string) error {
	tenant, err := principal.RequireOperator()
	if err != nil {
		return err
	}
	if !identity.ValidKeyID(id) {
		return ErrNotFound
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		n, err := q.RevokeAPIKey(ctx, sqlc.RevokeAPIKeyParams{MAX: time.Now().UTC().UnixMicro(), TenantID: string(tenant), ID: id})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *Store) GetFeed(ctx context.Context, principal identity.Principal, id string) (feed.Feed, error) {
	tenant, err := access(principal, false, id)
	if err != nil {
		return feed.Feed{}, err
	}
	v, err := sqlc.New(s.readers).GetFeed(ctx, sqlc.GetFeedParams{TenantID: string(tenant), ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return feed.Feed{}, ErrNotFound
	}
	return feed.Feed{ID: v.ID, Title: v.Title}, err
}
