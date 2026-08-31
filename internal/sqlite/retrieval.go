package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"slices"
	"time"
)

// ConfigureFeedPreferences selects only previously attached tenant sources. It
// grants no collection approval and is deliberately an operator-only seam.
func (s *Store) ConfigureFeedPreferences(ctx context.Context, p identity.Principal, id string, v feed.Preferences) error {
	tenant, err := access(p, true, id)
	if err != nil {
		return err
	}
	if err = v.Validate(); err != nil {
		return query.ErrInvalid
	}
	encoded, err := json.Marshal(v)
	if err != nil || len(encoded) > 65536 {
		return query.ErrInvalid
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		if _, err := queryScope(ctx, q, string(tenant), id); err != nil {
			return err
		}
		for _, src := range v.Sources {
			n, err := q.QuerySourceAllowed(ctx, sqlc.QuerySourceAllowedParams{TenantID: string(tenant), FeedID: id, SourceID: src.SourceID})
			if err != nil {
				return err
			}
			if n != 1 {
				return ErrNotFound
			}
		}
		n, err := q.ConfigureFeedPreferences(ctx, sqlc.ConfigureFeedPreferencesParams{Preferences: string(encoded), TenantID: string(tenant), ID: id})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrNotFound
		}
		return nil
	})
}
func preferences(ctx context.Context, q *sqlc.Queries, tenant, id string) (feed.Preferences, error) {
	raw, err := q.FeedPreferences(ctx, sqlc.FeedPreferencesParams{TenantID: tenant, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	if err != nil {
		return feed.Preferences{}, err
	}
	var p feed.Preferences
	if json.Unmarshal([]byte(raw), &p) != nil || p.Validate() != nil {
		return p, query.ErrUnavailable
	}
	return p, nil
}

// RetrieveCandidates uses one consistent read transaction. FTS virtual-table
// SQL stays in the adapter, as with Search; sqlc owns the relational queries.
// All filters run before the per-category cap. No body leaves this operation.
func (s *Store) RetrieveCandidates(ctx context.Context, p identity.Principal, id string, r query.Request) (query.Corpus, error) {
	tenant, err := access(p, false, id, r.FeedID)
	if err != nil {
		return query.Corpus{}, err
	}
	digest, err := r.Digest()
	if err != nil {
		return query.Corpus{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, query.RetrievalTimeout)
	defer cancel()
	tx, err := s.readers.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return query.Corpus{}, err
	}
	defer tx.Rollback()
	q := sqlc.New(tx)
	w, err := queryWork(ctx, q, string(tenant), id)
	if err != nil {
		return query.Corpus{}, err
	}
	scope, err := queryScope(ctx, q, string(tenant), r.FeedID)
	if err != nil {
		return query.Corpus{}, err
	}
	if w.WorkState != "pending" || w.LeaseUntil <= s.queryTime().UnixMicro() || !bytes.Equal(w.RequestHash, digest[:]) || w.FeedID != r.FeedID || w.RankerVersion != query.DeterministicVersion || !sameScope(w, scope) {
		return query.Corpus{}, query.ErrConflict
	}
	pref, err := preferences(ctx, q, string(tenant), r.FeedID)
	if err != nil {
		return query.Corpus{}, err
	}
	corpus := query.Corpus{Preferences: pref, Terms: query.Terms(r.Context, pref.UseContext)}
	include, exclude := query.LiteralMatch(corpus.Terms), query.LiteralMatch(pref.Exclude)
	for _, cat := range pref.Categories {
		ids := []string{}
		groups := map[string]string{}
		for _, rule := range pref.Sources {
			if slices.Contains(rule.Categories, cat) {
				ids = append(ids, rule.SourceID)
				groups[rule.SourceID] = rule.PublisherGroup
			}
		}
		encoded, _ := json.Marshal(ids)
		// Independent subqueries keep MATCH syntax literal and avoid relying on
		// global BM25 statistics that could change ranking across tenant boundaries.
		statement := `SELECT a.id,a.source_id,a.origin_id,a.url,a.title,a.content_hash,a.published_at,a.observed_at
FROM articles a JOIN feed_sources fs ON fs.tenant_id=a.tenant_id AND fs.source_id=a.source_id
JOIN sources s ON s.tenant_id=a.tenant_id AND s.id=a.source_id AND s.enabled=1
WHERE a.tenant_id=? AND fs.feed_id=? AND a.source_id IN (SELECT value FROM json_each(?))
AND coalesce(a.published_at,a.observed_at)>=? AND coalesce(a.published_at,a.observed_at)<=? AND a.observed_at<=?`
		args := []any{string(tenant), r.FeedID, string(encoded), w.SinceAt, w.CreatedAt, w.CreatedAt}
		if include != "" {
			statement += ` AND a.rowid IN (SELECT rowid FROM article_fts WHERE article_fts MATCH ?)`
			args = append(args, include)
		}
		if exclude != "" {
			statement += ` AND a.rowid NOT IN (SELECT rowid FROM article_fts WHERE article_fts MATCH ?)`
			args = append(args, exclude)
		}
		statement += ` ORDER BY coalesce(a.published_at,a.observed_at) DESC,a.id LIMIT ?`
		args = append(args, query.CandidatesPerCategory+1)
		rows, err := tx.QueryContext(ctx, statement, args...)
		if err != nil {
			return query.Corpus{}, err
		}
		count := 0
		for rows.Next() {
			var c query.Candidate
			var published sql.NullInt64
			var observed int64
			a := &c.Article
			if err = rows.Scan(&a.ID, &a.SourceID, &a.OriginID, &a.URL, &a.Title, &a.ContentHash, &published, &observed); err != nil {
				rows.Close()
				return query.Corpus{}, err
			}
			count++
			if count > query.CandidatesPerCategory {
				corpus.Truncated = true
				break
			}
			a.ObservedAt = time.UnixMicro(observed).UTC()
			if published.Valid {
				t := time.UnixMicro(published.Int64).UTC()
				a.PublishedAt = &t
			}
			c.Category = cat
			c.PublisherGroup = groups[a.SourceID]
			corpus.Candidates = append(corpus.Candidates, c)
		}
		err = errors.Join(rows.Err(), rows.Close())
		if err != nil {
			return query.Corpus{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return query.Corpus{}, err
	}
	return corpus, nil
}

func (s *Store) CompleteRetrieval(ctx context.Context, p identity.Principal, id string, r query.Retrieval) (query.Snapshot, error) {
	return s.completeQuery(ctx, p, id, "deterministic_fallback", r.Selections, query.Settlement{Known: true}, &r)
}

func retrievalDetails(ctx context.Context, q *sqlc.Queries, w sqlc.QueryWork, r *query.Retrieval) (*query.Details, feed.Preferences, error) {
	pref, err := preferences(ctx, q, w.TenantID, w.FeedID)
	if err != nil {
		return nil, pref, err
	}
	cats := pref.Categories
	if len(cats) > 1 && int64(len(cats)) > w.ItemLimit {
		cats = cats[:w.ItemLimit]
	}
	if !slices.Equal(cats, r.Categories) || len(r.Warnings) > 10 {
		return nil, pref, query.ErrInvalid
	}
	for _, v := range r.Warnings {
		if !text(v, 500, false) {
			return nil, pref, query.ErrInvalid
		}
	}
	sources, err := q.RetrievalSources(ctx, sqlc.RetrievalSourcesParams{TenantID: w.TenantID, ID: w.FeedID})
	if err != nil {
		return nil, pref, err
	}
	if len(sources) != len(pref.Sources) {
		return nil, pref, query.ErrConflict
	}
	detail := &query.Details{Categories: slices.Clone(cats), Warnings: slices.Clone(r.Warnings), Sources: []query.SourceHealth{}}
	for _, src := range sources {
		health := query.SourceHealth{SourceID: src.SourceID, Allowed: src.Allowed == 1}
		if src.LastSuccess.Valid && src.IntervalUs > 0 && src.LastSuccess.Int64 <= w.CreatedAt {
			until := time.UnixMicro(src.LastSuccess.Int64).UTC().Add(2 * time.Duration(src.IntervalUs) * time.Microsecond)
			if validTimestamp(until) {
				health.CurrentUntil = &until
			}
		}
		detail.Sources = append(detail.Sources, health)
	}
	return detail, pref, nil
}

var _ query.RetrievalStore = (*Store)(nil)
