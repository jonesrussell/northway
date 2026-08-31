package sqlite

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"slices"
)

func queryID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (s *Store) queryTime() time.Time {
	if s.clock != nil {
		return s.clock().UTC().Truncate(time.Microsecond)
	}
	return time.Now().UTC().Truncate(time.Microsecond)
}

func queryScope(ctx context.Context, q *sqlc.Queries, tenant, feed string) (sqlc.QueryScopeRow, error) {
	v, err := q.QueryScope(ctx, sqlc.QueryScopeParams{TenantID: tenant, ID: feed})
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return v, err
}

func sameScope(w sqlc.QueryWork, scope sqlc.QueryScopeRow) bool {
	return w.FeedRevision == scope.Revision && w.CorpusRevision == scope.CorpusRevision && w.EntitlementRevision == scope.EntitlementRevision
}

// Twenty items fit within 512 KiB, including sixfold JSON escaping of bounded
// title, URL, source and explanation fields plus timestamps and identifiers.
// Keep the migration CHECK aligned; the worst-case round-trip test covers both.
const snapshotJSONLimit = 512 * 1024

// BeginQuery atomically claims a key, reuses an authorized cache entry or holds
// a worst-case budget. Insufficient/unconfigured budget permits deterministic
// work only. A replay never returns WorkID, so it cannot authorize another call.
func (s *Store) BeginQuery(ctx context.Context, principal identity.Principal, key string, request query.Request, policy query.Policy) (query.Claim, error) {
	tenant, err := access(principal, false, request.FeedID)
	if err != nil {
		return query.Claim{}, err
	}
	digest, err := request.Digest()
	if err != nil || !query.ValidKey(key) || policy.Validate() != nil {
		return query.Claim{}, query.ErrInvalid
	}
	keyHash := sha256.Sum256([]byte("POST /v1/feed-queries\x00" + key))
	var claim query.Claim
	err = s.write(ctx, func(q *sqlc.Queries) error {
		now := s.queryTime()
		if !validTimestamp(now) || !validTimestamp(now.Add(25*time.Hour)) {
			return query.ErrUnavailable
		}
		scope, err := queryScope(ctx, q, string(tenant), request.FeedID)
		if err != nil {
			return err
		}
		old, err := q.QueryWorkByKey(ctx, sqlc.QueryWorkByKeyParams{TenantID: string(tenant), KeyHash: keyHash[:]})
		if err == nil {
			if !bytes.Equal(old.RequestHash, digest[:]) {
				return query.ErrConflict
			}
			if old.RetainUntil <= now.UnixMicro() {
				return query.ErrUnavailable
			}
			switch old.WorkState {
			case "done":
				snapshot, err := loadSnapshot(ctx, q, string(tenant), old.SnapshotID.String, now)
				if err != nil {
					return err
				}
				claim.Snapshot = &snapshot
				return nil
			case "pending":
				if old.LeaseUntil > now.UnixMicro() {
					return query.ErrInProgress
				}
			}
			return query.ErrUnavailable
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		retain := now.Add(24 * time.Hour).UnixMicro()
		w := sqlc.CreateQueryWorkParams{TenantID: string(tenant), ID: queryID(), KeyHash: keyHash[:], RequestHash: digest[:], FeedID: request.FeedID, FeedRevision: scope.Revision, CorpusRevision: scope.CorpusRevision, EntitlementRevision: scope.EntitlementRevision, RankerVersion: policy.RankerVersion, ItemLimit: int64(request.Limit), SinceAt: max(0, now.Add(-time.Duration(request.MaxAgeHours)*time.Hour).UnixMicro()), CreatedAt: now.UnixMicro(), LeaseUntil: now.Add(policy.Lease).UnixMicro(), RetainUntil: retain, CacheTtl: policy.CacheTTL.Microseconds(), WorkState: "pending", SpendState: "reserved"}
		cached, err := q.FindQueryCache(ctx, sqlc.FindQueryCacheParams{TenantID: w.TenantID, FeedID: w.FeedID, RequestHash: w.RequestHash, FeedRevision: w.FeedRevision, CorpusRevision: w.CorpusRevision, EntitlementRevision: w.EntitlementRevision, RankerVersion: w.RankerVersion, ExpiresAt: now.UnixMicro()})
		if err == nil {
			snapshot, err := loadSnapshot(ctx, q, w.TenantID, cached.ID, now)
			if err != nil {
				return err
			}
			if err := q.RetainQuerySnapshot(ctx, sqlc.RetainQuerySnapshotParams{TenantID: w.TenantID, ID: cached.ID, RetainUntil: retain}); err != nil {
				return err
			}
			w.WorkState, w.SpendState = "done", "settled"
			w.ActualMicros = sql.NullInt64{Valid: true}
			w.SnapshotID = sql.NullString{String: cached.ID, Valid: true}
			claim.Snapshot = &snapshot
		} else {
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if policy.WorstCaseMicros > 0 {
				n, err := q.ReserveBudget(ctx, sqlc.ReserveBudgetParams{TenantID: w.TenantID, Amount: policy.WorstCaseMicros})
				if err != nil {
					return err
				}
				if n == 1 {
					w.ReservedMicros = policy.WorstCaseMicros
				}
			}
			claim.WorkID, claim.ProviderAllowed = w.ID, w.ReservedMicros > 0
		}
		return q.CreateQueryWork(ctx, w)
	})
	if err != nil {
		return query.Claim{}, err
	}
	return claim, nil
}

func queryWork(ctx context.Context, q *sqlc.Queries, tenant, id string) (sqlc.QueryWork, error) {
	w, err := q.QueryWorkByID(ctx, sqlc.QueryWorkByIDParams{TenantID: tenant, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return w, err
}

func updateWork(ctx context.Context, q *sqlc.Queries, w sqlc.QueryWork) error {
	return q.UpdateQueryWork(ctx, sqlc.UpdateQueryWorkParams{TenantID: w.TenantID, ID: w.ID, WorkState: w.WorkState, SpendState: w.SpendState, ActualMicros: w.ActualMicros, SnapshotID: w.SnapshotID})
}

// StartProvider must commit successfully BEFORE the sole provider attempt. An
// error/ambiguous commit never authorizes a call. Repeating this method fails;
// it cannot be used to retry a timeout. No provider code runs inside storage.
func (s *Store) StartProvider(ctx context.Context, principal identity.Principal, id string) error {
	tenant, err := access(principal, false, id)
	if err != nil {
		return err
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		w, err := queryWork(ctx, q, string(tenant), id)
		if err != nil {
			return err
		}
		if w.WorkState != "pending" || w.SpendState != "reserved" || w.ReservedMicros == 0 || w.LeaseUntil <= s.queryTime().UnixMicro() {
			return query.ErrConflict
		}
		scope, err := queryScope(ctx, q, w.TenantID, w.FeedID)
		if err != nil {
			return err
		}
		if !sameScope(w, scope) {
			return query.ErrConflict
		}
		w.SpendState = "started"
		return updateWork(ctx, q, w)
	})
}

func settle(ctx context.Context, q *sqlc.Queries, w *sqlc.QueryWork, value query.Settlement) error {
	if !value.Known {
		if value.ActualMicros != 0 || (w.SpendState != "started" && w.SpendState != "uncertain") {
			return query.ErrInvalid
		}
		w.SpendState = "uncertain"
		return nil
	}
	if value.ActualMicros < 0 || value.ActualMicros > w.ReservedMicros || (w.SpendState == "reserved" && value.ActualMicros != 0) || w.SpendState == "settled" {
		return query.ErrConflict
	}
	if w.ReservedMicros > 0 {
		n, err := q.SettleBudget(ctx, sqlc.SettleBudgetParams{TenantID: w.TenantID, Reserved: w.ReservedMicros, Actual: value.ActualMicros})
		if err != nil {
			return err
		}
		if n != 1 {
			return query.ErrConflict
		}
	}
	w.SpendState = "settled"
	w.ActualMicros = sql.NullInt64{Int64: value.ActualMicros, Valid: true}
	return nil
}

// CompleteQuery commits immutable, bounded selected-item evidence and usage
// together. It rejects stale or invented candidates. Explanation grounding is
// the later ranker's responsibility; storage never trusts caller titles/URLs.
func (s *Store) CompleteQuery(ctx context.Context, principal identity.Principal, id, mode string, selections []query.Selection, cost query.Settlement) (query.Snapshot, error) {
	return s.completeQuery(ctx, principal, id, mode, selections, cost, nil)
}
func (s *Store) completeQuery(ctx context.Context, principal identity.Principal, id, mode string, selections []query.Selection, cost query.Settlement, retrieval *query.Retrieval) (query.Snapshot, error) {
	tenant, err := access(principal, false, id)
	if err != nil {
		return query.Snapshot{}, err
	}
	if (mode != "ai" && mode != "deterministic_fallback") || len(selections) > 20 {
		return query.Snapshot{}, query.ErrInvalid
	}
	seen := make(map[string]bool, len(selections))
	for _, selection := range selections {
		if selection.Validate() != nil || seen[selection.ArticleID] {
			return query.Snapshot{}, query.ErrInvalid
		}
		seen[selection.ArticleID] = true
	}
	var snapshot query.Snapshot
	err = s.write(ctx, func(q *sqlc.Queries) error {
		now := s.queryTime()
		w, err := queryWork(ctx, q, string(tenant), id)
		if err != nil {
			return err
		}
		if w.WorkState != "pending" || w.LeaseUntil <= now.UnixMicro() || int64(len(selections)) > w.ItemLimit || (mode == "ai" && w.SpendState != "started") {
			return query.ErrConflict
		}
		scope, err := queryScope(ctx, q, w.TenantID, w.FeedID)
		if err != nil {
			return err
		}
		// Unrelated arrivals must not discard an already-paid result. Selected
		// versions and current access are checked below. Keep the ORIGINAL corpus
		// revision on the snapshot: it must not cache-hit against unseen arrivals.
		if w.FeedRevision != scope.Revision || w.EntitlementRevision != scope.EntitlementRevision {
			return query.ErrConflict
		}
		var details *query.Details
		var pref feed.Preferences
		if retrieval != nil {
			if w.RankerVersion != query.DeterministicVersion || !sameScope(w, scope) || w.SpendState != "reserved" || w.ReservedMicros != 0 {
				return query.ErrConflict
			}
			details, pref, err = retrievalDetails(ctx, q, w, retrieval, now)
			if err != nil {
				return err
			}
		}
		items := make([]query.Item, 0, len(selections))
		for _, selection := range selections {
			a, err := q.QueryArticle(ctx, sqlc.QueryArticleParams{TenantID: w.TenantID, FeedID: w.FeedID, ID: selection.ArticleID})
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			effective := a.ObservedAt
			if a.PublishedAt.Valid {
				effective = a.PublishedAt.Int64
			}
			if a.ContentHash != selection.ContentHash || effective < w.SinceAt || effective > w.CreatedAt || a.ObservedAt > w.CreatedAt {
				return query.ErrConflict
			}
			item := query.Item{ArticleID: a.ID, SourceID: a.SourceID, ContentHash: a.ContentHash, Title: a.Title, URL: a.Url, Explanation: selection.Explanation}
			if retrieval != nil {
				allowed := false
				for _, rule := range pref.Sources {
					if rule.SourceID == a.SourceID && slices.Contains(rule.Categories, selection.Category) {
						allowed = true
						break
					}
				}
				if !allowed || !slices.Contains(retrieval.Categories, selection.Category) {
					return query.ErrConflict
				}
				item.SourceName = a.SourceName
				item.ObservedAt = time.UnixMicro(a.ObservedAt).UTC()
				item.Category = selection.Category
				if a.PublishedAt.Valid {
					t := time.UnixMicro(a.PublishedAt.Int64).UTC()
					item.PublishedAt = &t
				}
			}
			items = append(items, item)
		}
		encoded, err := json.Marshal(items)
		if err != nil || len(encoded) > snapshotJSONLimit {
			return query.ErrInvalid
		}
		if err := settle(ctx, q, &w, cost); err != nil {
			return err
		}
		expires := now.Add(time.Duration(w.CacheTtl) * time.Microsecond)
		snapshot = query.Snapshot{ID: queryID(), FeedID: w.FeedID, RankerVersion: w.RankerVersion, Mode: mode, FeedRevision: w.FeedRevision, GeneratedAt: now, ExpiresAt: expires, Items: items, Details: details}
		if err := q.CreateQuerySnapshot(ctx, sqlc.CreateQuerySnapshotParams{TenantID: w.TenantID, ID: snapshot.ID, FeedID: w.FeedID, RequestHash: w.RequestHash, FeedRevision: w.FeedRevision, CorpusRevision: w.CorpusRevision, EntitlementRevision: w.EntitlementRevision, RankerVersion: w.RankerVersion, Mode: mode, GeneratedAt: now.UnixMicro(), ExpiresAt: expires.UnixMicro(), RetainUntil: max(w.RetainUntil, expires.UnixMicro()), Items: string(encoded)}); err != nil {
			return err
		}
		if details != nil {
			data, err := json.Marshal(details)
			if err != nil || len(data) > 65536 {
				return query.ErrInvalid
			}
			n, err := q.SnapshotDetails(ctx, sqlc.SnapshotDetailsParams{Details: string(data), TenantID: w.TenantID, ID: snapshot.ID})
			if err != nil {
				return err
			}
			if n != 1 {
				return query.ErrConflict
			}
		}
		w.WorkState = "done"
		w.SnapshotID = sql.NullString{String: snapshot.ID, Valid: true}
		return updateWork(ctx, q, w)
	})
	if err != nil {
		return query.Snapshot{}, err
	}
	return snapshot, nil
}

func loadSnapshot(ctx context.Context, q *sqlc.Queries, tenant, id string, now time.Time) (query.Snapshot, error) {
	v, err := q.GetQuerySnapshot(ctx, sqlc.GetQuerySnapshotParams{TenantID: tenant, ID: id, RetainUntil: now.UnixMicro()})
	if errors.Is(err, sql.ErrNoRows) {
		return query.Snapshot{}, ErrNotFound
	}
	if err != nil {
		return query.Snapshot{}, err
	}
	if _, err := queryScope(ctx, q, tenant, v.FeedID); err != nil {
		return query.Snapshot{}, err
	}
	var items []query.Item
	if err := json.Unmarshal([]byte(v.Items), &items); err != nil || len(items) > 20 {
		return query.Snapshot{}, query.ErrUnavailable
	}
	snapshot := query.Snapshot{ID: v.ID, FeedID: v.FeedID, FeedRevision: v.FeedRevision, RankerVersion: v.RankerVersion, Mode: v.Mode, GeneratedAt: time.UnixMicro(v.GeneratedAt).UTC(), ExpiresAt: time.UnixMicro(v.ExpiresAt).UTC(), Items: make([]query.Item, 0, len(items))}
	if v.Details != "" {
		snapshot.Details = &query.Details{}
		if json.Unmarshal([]byte(v.Details), snapshot.Details) != nil || len(snapshot.Details.Sources) < 1 || len(snapshot.Details.Sources) > 100 {
			return query.Snapshot{}, query.ErrUnavailable
		}
		for i, source := range snapshot.Details.Sources {
			n, err := q.RetrievalSourceAllowed(ctx, sqlc.RetrievalSourceAllowedParams{TenantID: tenant, ID: v.FeedID, SourceID: source.SourceID})
			if err != nil {
				return query.Snapshot{}, err
			}
			if n == 0 {
				snapshot.Details.Sources[i].Allowed = false
				if source.Allowed {
					snapshot.Suppressed = true
				}
			}
		}
	}
	for _, item := range items {
		n, err := q.QuerySourceAllowed(ctx, sqlc.QuerySourceAllowedParams{TenantID: tenant, FeedID: v.FeedID, SourceID: item.SourceID})
		if err != nil {
			return query.Snapshot{}, err
		}
		if snapshot.Details != nil && n > 0 {
			n, err = q.RetrievalSourceAllowed(ctx, sqlc.RetrievalSourceAllowedParams{TenantID: tenant, ID: v.FeedID, SourceID: item.SourceID})
			if err != nil {
				return query.Snapshot{}, err
			}
		}
		if n == 0 {
			snapshot.Suppressed = true
			continue
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	return snapshot, nil
}

// GetSnapshot rechecks current feed/source access in the same short transaction
// as the read. Revocation suppresses items without reranking retained evidence.
func (s *Store) GetSnapshot(ctx context.Context, principal identity.Principal, id string) (query.Snapshot, error) {
	tenant, err := access(principal, false, id)
	if err != nil {
		return query.Snapshot{}, err
	}
	var snapshot query.Snapshot
	err = s.write(ctx, func(q *sqlc.Queries) error {
		var err error
		snapshot, err = loadSnapshot(ctx, q, string(tenant), id, s.queryTime())
		return err
	})
	if err != nil {
		return query.Snapshot{}, err
	}
	return snapshot, nil
}

func failWork(ctx context.Context, q *sqlc.Queries, w *sqlc.QueryWork) error {
	if w.WorkState != "pending" {
		return query.ErrConflict
	}
	value := query.Settlement{Known: w.SpendState == "reserved"}
	if err := settle(ctx, q, w, value); err != nil {
		return err
	}
	w.WorkState = "failed"
	return updateWork(ctx, q, *w)
}

// FailQuery never treats a started provider attempt as free. A caller may use
// it after cancellation with a fresh bounded context; otherwise recovery owns it.
func (s *Store) FailQuery(ctx context.Context, principal identity.Principal, id string) error {
	tenant, err := access(principal, false, id)
	if err != nil {
		return err
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		w, err := queryWork(ctx, q, string(tenant), id)
		if err != nil {
			return err
		}
		if w.WorkState == "failed" {
			return nil
		}
		return failWork(ctx, q, &w)
	})
}

// RecoverQueries processes at most 100 expired claims, fences stale workers and
// preserves uncertain holds. The future scheduler/operator must call it; opening
// a database never starts jobs. Idempotency tombstones are not deleted here.
func (s *Store) RecoverQueries(ctx context.Context, principal identity.Principal) (int, error) {
	tenant, err := principal.RequireOperator()
	if err != nil {
		return 0, err
	}
	count := 0
	err = s.write(ctx, func(q *sqlc.Queries) error {
		rows, err := q.ExpiredQueryWork(ctx, sqlc.ExpiredQueryWorkParams{TenantID: string(tenant), LeaseUntil: s.queryTime().UnixMicro()})
		if err != nil {
			return err
		}
		for _, w := range rows {
			if err := failWork(ctx, q, &w); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ReconcileQuery requires affirmative provider/operator evidence, not elapsed
// time. Repeating identical settlement is safe; contradictory evidence conflicts.
func (s *Store) ReconcileQuery(ctx context.Context, principal identity.Principal, id string, actualMicros int64) error {
	tenant, err := access(principal, true, id)
	if err != nil {
		return err
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		w, err := queryWork(ctx, q, string(tenant), id)
		if err != nil {
			return err
		}
		if w.SpendState == "settled" {
			if w.ActualMicros.Int64 == actualMicros {
				return nil
			}
			return query.ErrConflict
		}
		if w.SpendState != "uncertain" {
			return query.ErrConflict
		}
		if err := settle(ctx, q, &w, query.Settlement{Known: true, ActualMicros: actualMicros}); err != nil {
			return err
		}
		return updateWork(ctx, q, w)
	})
}
