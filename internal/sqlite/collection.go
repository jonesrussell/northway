package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"time"
)

func validObservation(o ingest.Observation, account string, preview bool) bool {
	if o.Version != 1 || o.AccountURL != account || o.EvidenceURL != account || o.SourceID != "" || o.ID != "" || o.Revision != 0 || o.ObservedAt != 0 || !link(o.OriginalURL) || !text(o.Title, 512, false) || !text(o.Description, 2048, true) || o.Method != "html-metadata-v1" || o.State != "available" {
		return false
	}
	if o.Kind == "profile" && o.OriginalURL != account {
		return false
	}
	if o.PreviewURL != "" && (!preview || !link(o.PreviewURL)) {
		return false
	}
	switch o.Kind {
	case "profile", "image", "video", "link":
	default:
		return false
	}
	switch o.Display {
	case "link":
		return o.EmbedURL == ""
	case "image":
		return o.Kind == "image" && preview && o.PreviewURL != "" && o.EmbedURL == ""
	case "embed":
		return o.Kind == "video" && ingest.SafeEmbed(o.EmbedURL)
	default:
		return false
	}
}
func putCollection(ctx context.Context, q *sqlc.Queries, tenant, source, account string, preview bool, r ingest.Result, now time.Time) error {
	incoming := r.Observations
	if r.Status == 304 {
		if len(incoming) != 0 {
			return ingest.ErrInvalid
		}
		return nil
	}
	if r.Status == 404 || r.Status == 410 {
		if len(incoming) != 0 {
			return ingest.ErrInvalid
		}
		rows, e := q.CollectionSourceItems(ctx, sqlc.CollectionSourceItemsParams{TenantID: tenant, SourceID: source})
		if e != nil {
			return e
		}
		for _, raw := range rows {
			var o ingest.Observation
			if json.Unmarshal([]byte(raw), &o) != nil {
				return ingest.ErrInvalid
			}
			o.State = "removed"
			incoming = append(incoming, o)
		}
	} else if r.Status != 200 {
		return ingest.ErrInvalid
	}
	seen := map[string]bool{}
	for _, o := range incoming {
		if r.Status == 200 && !validObservation(o, account, preview) {
			return ingest.ErrInvalid
		}
		id := itemIDFor(source, o.Kind+"\x00"+o.OriginalURL)
		if seen[id] {
			return ingest.ErrInvalid
		}
		seen[id] = true
		h := sha256.Sum256(o.Fingerprint())
		hash := hex.EncodeToString(h[:])
		old, e := q.CollectionItem(ctx, sqlc.CollectionItemParams{TenantID: tenant, ID: id})
		revision := int64(1)
		if e == nil {
			if old.Fingerprint == hash {
				continue
			}
			revision = old.Revision + 1
		} else if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		o.ID = id
		o.SourceID = source
		o.Revision = revision
		o.ObservedAt = now.Unix()
		raw, e := json.Marshal(o)
		if e != nil || len(raw) > 16384 {
			return ingest.ErrInvalid
		}
		if e = q.PutCollectionItem(ctx, sqlc.PutCollectionItemParams{TenantID: tenant, SourceID: source, ID: id, Revision: revision, Fingerprint: hash, Payload: string(raw)}); e != nil {
			return e
		}
		if e = q.AppendCollectionEvent(ctx, sqlc.AppendCollectionEventParams{TenantID: tenant, SourceID: source, ItemID: id, Revision: revision, Payload: string(raw)}); e != nil {
			return e
		}
	}
	n, e := q.CollectionSourceCount(ctx, sqlc.CollectionSourceCountParams{TenantID: tenant, SourceID: source})
	if e != nil {
		return e
	}
	v, e := q.CollectionVersionCount(ctx, sqlc.CollectionVersionCountParams{TenantID: tenant, SourceID: source})
	if e != nil {
		return e
	}
	// Terminal removal is finite: at most one tombstone per stored item.
	if n > 5000 || (v > 10000 && r.Status != 404 && r.Status != 410) {
		return ingest.ErrCorpusFull
	}
	return nil
}

// CollectionBatch is a local operator export seam. It cannot provision tenants,
// enable acquisition, or use personal query snapshots as a delivery queue.
func (s *Store) CollectionBatch(ctx context.Context, p identity.Principal, after int64) (ingest.Batch, error) {
	tenant, e := p.RequireOperator()
	if e != nil {
		return ingest.Batch{}, e
	}
	if after < 0 {
		return ingest.Batch{}, ingest.ErrInvalid
	}
	if e = s.RequireTenant(ctx, p); e != nil {
		return ingest.Batch{}, e
	}
	rows, e := sqlc.New(s.readers).CollectionEvents(ctx, sqlc.CollectionEventsParams{TenantID: string(tenant), AfterCursor: after})
	if e != nil {
		return ingest.Batch{}, e
	}
	b := ingest.Batch{Version: 1, After: after, Next: after, Events: []ingest.Event{}}
	for _, row := range rows {
		var o ingest.Observation
		if e = json.Unmarshal([]byte(row.Payload), &o); e != nil {
			return ingest.Batch{}, e
		}
		b.Events = append(b.Events, ingest.Event{Cursor: row.Sequence, Observation: o})
		b.Next = row.Sequence
	}
	return b, nil
}

// AddCollectionSeed is retry-safe operator management. It never grants rights,
// enables polling, changes an existing source, or accepts a scraped policy.
func (s *Store) AddCollectionSeed(ctx context.Context, p identity.Principal, v ingest.CollectionSeed) error {
	tenant, e := access(p, true, v.ID)
	if e != nil {
		return e
	}
	if !pollURL(v.URL) || !text(v.Title, 512, false) {
		return ingest.ErrInvalid
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		old, e := q.PollSourceURL(ctx, sqlc.PollSourceURLParams{TenantID: string(tenant), SourceID: v.ID})
		if e == nil {
			mode, e := q.CollectionSeedMode(ctx, sqlc.CollectionSeedModeParams{TenantID: string(tenant), SourceID: v.ID})
			if e != nil || old != v.URL || mode != "html" {
				return ingest.ErrInvalid
			}
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		n, e := q.OtherPollSources(ctx, sqlc.OtherPollSourcesParams{TenantID: string(tenant), SourceID: v.ID})
		if e != nil {
			return e
		}
		if n >= ingest.MaxSources {
			return ingest.ErrBudget
		}
		if e = q.CreateSource(ctx, sqlc.CreateSourceParams{TenantID: string(tenant), ID: v.ID, Url: v.URL, Title: v.Title}); e != nil {
			return e
		}
		if e = q.ConfigurePoll(ctx, sqlc.ConfigurePollParams{TenantID: string(tenant), SourceID: v.ID, ApprovedUrl: v.URL, Approved: 0, Enabled: 0, IntervalUs: time.Hour.Microseconds(), MaxBytes: ingest.MaxResponseBytes, NextAt: s.queryTime().UnixMicro()}); e != nil {
			return e
		}
		return q.SetCollectionMode(ctx, sqlc.SetCollectionModeParams{TenantID: string(tenant), SourceID: v.ID, RobotsUntil: 0, PreviewAllowed: 0})
	})
}
func (s *Store) CollectionStatus(ctx context.Context, p identity.Principal) (ingest.CollectionStatus, error) {
	tenant, e := p.RequireOperator()
	if e != nil {
		return ingest.CollectionStatus{}, e
	}
	if e = s.RequireTenant(ctx, p); e != nil {
		return ingest.CollectionStatus{}, e
	}
	v, e := sqlc.New(s.readers).CollectionStatus(ctx, string(tenant))
	return ingest.CollectionStatus{Seeds: v.Seeds, Enabled: v.Enabled, Items: v.Items, Revisions: v.Revisions}, e
}
