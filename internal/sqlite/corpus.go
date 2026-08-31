package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jonesrussell/northway/internal/article"
	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/source"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
)

var ErrNotFound = errors.New("object not found in tenant scope")

func access(principal identity.Principal, write bool, ids ...string) (identity.TenantID, error) {
	var tenant identity.TenantID
	var err error
	if write {
		tenant, err = principal.RequireOperator()
	} else {
		tenant, err = principal.Require(identity.FeedsRead)
	}
	if err != nil {
		return "", err
	}
	for _, id := range ids {
		if err := identity.ValidateID(id); err != nil {
			if write {
				return "", err
			}
			return "", ErrNotFound
		}
	}
	return tenant, nil
}

func text(value string, max int, empty bool) bool {
	return (empty || strings.TrimSpace(value) != "") && len(value) <= max && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func link(value string) bool {
	if !text(value, 2048, false) {
		return false
	}
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}

// validTimestamp bounds the UTC instant before UnixMicro can overflow and keeps
// stored timestamps representable by the API's RFC3339 format. Sub-microsecond
// precision is intentionally discarded when converting to database integers.
func validTimestamp(value time.Time) bool {
	year := value.UTC().Year()
	return year >= 1970 && year <= 9999
}

// CreateTenant is an operator-only provisioning seam, not a public endpoint.
func (s *Store) CreateTenant(ctx context.Context, tenant identity.TenantID) error {
	if err := tenant.Validate(); err != nil {
		return err
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		return q.CreateTenant(ctx, sqlc.CreateTenantParams{ID: string(tenant), CreatedAt: time.Now().UTC().UnixMicro()})
	})
}

func (s *Store) CreateSource(ctx context.Context, principal identity.Principal, v source.Source) error {
	tenant, err := access(principal, true, v.ID)
	if err != nil {
		return err
	}
	if !link(v.URL) || !text(v.Title, 512, false) {
		return errors.New("invalid source metadata")
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		return q.CreateSource(ctx, sqlc.CreateSourceParams{TenantID: string(tenant), ID: v.ID, Url: v.URL, Title: v.Title})
	})
}

func (s *Store) CreateFeed(ctx context.Context, principal identity.Principal, v feed.Feed) error {
	tenant, err := access(principal, true, v.ID)
	if err != nil {
		return err
	}
	if !text(v.Title, 512, false) {
		return errors.New("invalid feed title")
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		return q.CreateFeed(ctx, sqlc.CreateFeedParams{TenantID: string(tenant), ID: v.ID, Title: v.Title})
	})
}

func (s *Store) AttachSource(ctx context.Context, principal identity.Principal, feedID, sourceID string) error {
	tenant, err := access(principal, true, feedID, sourceID)
	if err != nil {
		return err
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		return q.AttachSource(ctx, sqlc.AttachSourceParams{TenantID: string(tenant), FeedID: feedID, SourceID: sourceID})
	})
}

// PutArticle atomically updates the current item, records its content version and
// updates FTS through triggers. Source/origin identity cannot change on update.
// Lease fencing and poll success advancement are implemented with ingestion #12.
func (s *Store) PutArticle(ctx context.Context, principal identity.Principal, v article.Article) error {
	tenant, err := access(principal, true, v.ID, v.SourceID)
	if err != nil {
		return err
	}
	if !link(v.URL) || !text(v.Title, 512, false) || !text(v.Body, 65536, true) || !text(v.OriginID, 2048, false) || !validTimestamp(v.ObservedAt) {
		return errors.New("invalid article metadata")
	}
	var published sql.NullInt64
	if v.PublishedAt != nil {
		if !validTimestamp(*v.PublishedAt) {
			return errors.New("invalid publication timestamp")
		}
		published = sql.NullInt64{Int64: v.PublishedAt.UTC().UnixMicro(), Valid: true}
	}
	hash := sha256.Sum256([]byte(v.Title + "\x00" + v.Body))
	hashText := hex.EncodeToString(hash[:])
	return s.write(ctx, func(q *sqlc.Queries) error {
		n, err := q.PutArticle(ctx, sqlc.PutArticleParams{TenantID: string(tenant), ID: v.ID, SourceID: v.SourceID, OriginID: v.OriginID, Url: v.URL, Title: v.Title, Body: v.Body, ContentHash: hashText, PublishedAt: published, ObservedAt: v.ObservedAt.UTC().UnixMicro()})
		if err != nil {
			return err
		}
		if n != 1 {
			return errors.New("article identity cannot change")
		}
		return q.RecordVersion(ctx, sqlc.RecordVersionParams{TenantID: string(tenant), ArticleID: v.ID, ContentHash: hashText, Title: v.Title, Body: v.Body, ObservedAt: v.ObservedAt.UTC().UnixMicro()})
	})
}

func (s *Store) GetArticle(ctx context.Context, principal identity.Principal, id string) (article.Article, error) {
	tenant, err := access(principal, false, id)
	if err != nil {
		return article.Article{}, err
	}
	v, err := sqlc.New(s.readers).GetArticle(ctx, sqlc.GetArticleParams{TenantID: string(tenant), ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return article.Article{}, ErrNotFound
	}
	if err != nil {
		return article.Article{}, err
	}
	return fromRow(v), nil
}

func fromRow(v sqlc.GetArticleRow) article.Article {
	a := article.Article{ID: v.ID, SourceID: v.SourceID, OriginID: v.OriginID, URL: v.Url, Title: v.Title, Body: v.Body, ContentHash: v.ContentHash, ObservedAt: time.UnixMicro(v.ObservedAt).UTC()}
	if v.PublishedAt.Valid {
		t := time.UnixMicro(v.PublishedAt.Int64).UTC()
		a.PublishedAt = &t
	}
	return a
}

func (s *Store) DeleteArticle(ctx context.Context, principal identity.Principal, id string) error {
	tenant, err := access(principal, true, id)
	if err != nil {
		return err
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		n, err := q.DeleteArticle(ctx, sqlc.DeleteArticleParams{TenantID: string(tenant), ID: id})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func matchExpression(input string) (string, error) {
	if !text(input, 256, false) {
		return "", errors.New("invalid search text")
	}
	words := strings.FieldsFunc(input, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	if len(words) == 0 || len(words) > 8 {
		return "", errors.New("search requires 1..8 literal terms")
	}
	for i, w := range words {
		if len(w) > 64 {
			return "", errors.New("search term too long")
		}
		words[i] = `"` + w + `"`
	}
	return strings.Join(words, " AND "), nil
}

// Search returns bounded, tenant/feed-scoped storage candidates only. This is
// not the contextual query/ranking API. SQL and MATCH syntax are bounded apart.
func (s *Store) Search(ctx context.Context, principal identity.Principal, feedID, terms string, since time.Time, limit int) ([]article.Article, error) {
	tenant, err := access(principal, false, feedID)
	if err != nil {
		return nil, err
	}
	if !validTimestamp(since) || limit < 1 || limit > 50 {
		return nil, errors.New("invalid search bounds")
	}
	expression, err := matchExpression(terms)
	if err != nil {
		return nil, err
	}
	// FTS5's virtual-table MATCH is kept here, outside sqlc's relational schema
	// parser. The real-file isolation/update/delete tests exercise this exact SQL.
	rows, err := s.readers.QueryContext(ctx, `SELECT a.id,a.source_id,a.origin_id,a.url,a.title,a.body,a.content_hash,a.published_at,a.observed_at
FROM article_fts JOIN articles a ON a.rowid=article_fts.rowid
JOIN feed_sources f ON f.tenant_id=a.tenant_id AND f.source_id=a.source_id
JOIN sources src ON src.tenant_id=a.tenant_id AND src.id=a.source_id AND src.enabled=1
JOIN feeds fd ON fd.tenant_id=f.tenant_id AND fd.id=f.feed_id AND fd.enabled=1
WHERE article_fts MATCH ? AND a.tenant_id=? AND f.tenant_id=? AND f.feed_id=? AND a.observed_at>=?
ORDER BY a.observed_at DESC,a.id LIMIT ?`, expression, string(tenant), string(tenant), feedID, since.UTC().UnixMicro(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]article.Article, 0)
	for rows.Next() {
		var v sqlc.GetArticleRow
		if err := rows.Scan(&v.ID, &v.SourceID, &v.OriginID, &v.Url, &v.Title, &v.Body, &v.ContentHash, &v.PublishedAt, &v.ObservedAt); err != nil {
			return nil, err
		}
		items = append(items, fromRow(v))
	}
	return items, rows.Err()
}
