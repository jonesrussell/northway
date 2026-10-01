package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/identity"
)

// ensurePublicWorkspace commits workspace creation and private memberships
// together. Register installation and acquisition are separate operator actions.
func (s *Store) ensurePublicWorkspace(ctx context.Context, tenant identity.TenantID) error {
	return s.publicWrite(ctx, func(tx *sql.Tx) error {
		var state string
		e := tx.QueryRowContext(ctx, `SELECT state FROM customer_workspaces WHERE tenant_id=$1`, string(tenant)).Scan(&state)
		if e == nil {
			if state != "active" {
				return identity.ErrForbidden
			}
			return s.provisionPublicCatalogue(ctx, tx, tenant)
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		var count int64
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM customer_workspaces`).Scan(&count); e != nil {
			return e
		}
		if count >= 5 {
			return identity.ErrForbidden
		}
		var exists bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tenants WHERE id=$1)`, string(tenant)).Scan(&exists); e != nil {
			return e
		}
		if exists {
			return identity.ErrForbidden
		}
		now := s.queryTime().UnixMicro()
		if _, e = tx.ExecContext(ctx, `INSERT INTO tenants(id,created_at) VALUES($1,$2)`, string(tenant), now); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO customer_workspaces(tenant_id,created_at) VALUES($1,$2)`, string(tenant), now); e != nil {
			return e
		}
		return s.provisionPublicCatalogue(ctx, tx, tenant)
	})
}

func PublicFeedID(topic string) string { return itemIDFor("northcloud.public.feed.v1", topic) }

func (s *Store) provisionPublicCatalogue(ctx context.Context, tx *sql.Tx, tenant identity.TenantID) error {
	rows, e := tx.QueryContext(ctx, `SELECT id,publisher,topic FROM public_sources WHERE onboarding=1 AND enabled=1 AND review_state='approved' AND rights_basis='reviewed_metadata' ORDER BY topic,id LIMIT 101`)
	if e != nil {
		return e
	}
	byTopic := map[string][]feed.SourceRule{}
	order := []string{}
	for rows.Next() {
		var source, publisher, topic string
		if e = rows.Scan(&source, &publisher, &topic); e != nil {
			rows.Close()
			return e
		}
		if _, ok := byTopic[topic]; !ok {
			order = append(order, topic)
		}
		byTopic[topic] = append(byTopic[topic], feed.SourceRule{SourceID: source, PublisherGroup: itemIDFor("northcloud.public.publisher.v1", publisher), Categories: []string{topic}})
	}
	if e = errors.Join(rows.Err(), rows.Close()); e != nil {
		return e
	}
	for _, topic := range order {
		pref := feed.Preferences{Categories: []string{topic}, Sources: byTopic[topic], PublisherCap: 2}
		if pref.Validate() != nil {
			return identity.ErrUnavailable
		}
		raw, e := json.Marshal(pref)
		if e != nil {
			return e
		}
		id := PublicFeedID(topic)
		result, e := tx.ExecContext(ctx, `INSERT INTO feeds(tenant_id,id,title,preferences) VALUES($1,$2,$3,$4) ON CONFLICT(tenant_id,id) DO NOTHING`, string(tenant), id, "Shared "+topic+" news", string(raw))
		if e != nil {
			return e
		}
		count, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if count == 0 {
			continue
		} // Preserve saved preferences on replay.
		for _, src := range pref.Sources {
			if _, e = tx.ExecContext(ctx, `INSERT INTO workspace_public_subscriptions(tenant_id,feed_id,source_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, string(tenant), id, src.SourceID); e != nil {
				return e
			}
		}
	}
	return nil
}
