package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
)

// PublicDefinition is metadata policy, not workspace ownership. These local
// operator methods have no HTTP route, startup hook or production schedule.
type PublicDefinition struct {
	URL, Title, Publisher, Topic, Language, Provenance, ReviewState, RightsBasis string
	Enabled                                                                      bool
	Interval                                                                     time.Duration
	MaxBytes                                                                     int64
	Onboarding                                                                   bool
}

func canonicalPublicURL(raw string) string {
	u, e := url.Parse(raw)
	if e != nil {
		return raw
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port := u.Port(); port != "" && port != "443" {
		host = net.JoinHostPort(u.Hostname(), port)
	}
	u.Host = host
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String()
}
func PublicSourceID(canonicalURL string) string {
	return itemIDFor("northcloud.public.source.v1", canonicalPublicURL(canonicalURL))
}

// publicWrite has the same database-wide admission lock as private writes. It
// commits before any caller can fetch and never retries a failed transaction.
func (s *Store) publicWrite(ctx context.Context, fn func(*sql.Tx) error) error {
	if !s.postgres {
		return errors.New("shared acquisition requires PostgreSQL")
	}
	if _, e := s.guard.ExecContext(ctx, "SELECT 1"); e != nil {
		return errors.New("PostgreSQL ownership connection lost")
	}
	tx, e := s.writer.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(762314202)"); e != nil {
		return e
	}
	var used int64
	if e = tx.QueryRowContext(ctx, "SELECT pg_database_size(current_database())").Scan(&used); e != nil {
		return e
	}
	if used > storageLimitBytes-(16<<20) {
		return ErrStoragePressure
	}
	if e = fn(tx); e != nil {
		return e
	}
	return tx.Commit()
}

func (s *Store) ConfigurePublicSource(ctx context.Context, v PublicDefinition) (string, error) {
	v.URL = canonicalPublicURL(v.URL)
	if !pollURL(v.URL) || !text(v.Title, 512, false) || !text(v.Publisher, 256, false) || !text(v.Topic, 128, false) || !text(v.Language, 32, false) || !text(v.Provenance, 2048, false) || v.Interval < 24*time.Hour || v.Interval > 7*24*time.Hour || v.MaxBytes < 1024 || v.MaxBytes > ingest.MaxResponseBytes {
		return "", ingest.ErrInvalid
	}
	if v.ReviewState != "pending" && v.ReviewState != "approved" && v.ReviewState != "excluded" {
		return "", ingest.ErrInvalid
	}
	if v.RightsBasis != "not_assessed" && v.RightsBasis != "reviewed_metadata" || v.Enabled && (v.ReviewState != "approved" || v.RightsBasis != "reviewed_metadata") {
		return "", ingest.ErrInvalid
	}
	id := PublicSourceID(v.URL)
	e := s.publicWrite(ctx, func(tx *sql.Tx) error {
		var count int64
		if e := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM poll_sources)+(SELECT count(*) FROM public_poll_sources WHERE source_id!=$1)`, id).Scan(&count); e != nil {
			return e
		}
		if count >= ingest.MaxSources {
			return ingest.ErrBudget
		}
		onboarding := 0
		if v.Onboarding {
			onboarding = 1
		}
		enabled := 0
		if v.Enabled {
			enabled = 1
		}
		result, e := tx.ExecContext(ctx, `INSERT INTO public_sources(id,canonical_url,title,publisher,topic,language,provenance,review_state,rights_basis,enabled,onboarding) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT(id) DO UPDATE SET title=excluded.title,publisher=excluded.publisher,topic=excluded.topic,language=excluded.language,provenance=excluded.provenance,review_state=excluded.review_state,rights_basis=excluded.rights_basis,enabled=excluded.enabled,onboarding=excluded.onboarding,policy_revision=public_sources.policy_revision+1
WHERE (public_sources.title,public_sources.publisher,public_sources.topic,public_sources.language,public_sources.provenance,public_sources.review_state,public_sources.rights_basis,public_sources.enabled,public_sources.onboarding) IS DISTINCT FROM (excluded.title,excluded.publisher,excluded.topic,excluded.language,excluded.provenance,excluded.review_state,excluded.rights_basis,excluded.enabled,excluded.onboarding)`, id, v.URL, v.Title, v.Publisher, v.Topic, v.Language, v.Provenance, v.ReviewState, v.RightsBasis, enabled, onboarding)
		if e != nil {
			return e
		}
		changed, e := result.RowsAffected()
		if e != nil {
			return e
		}
		// Reconfiguration fences prior work without refunding its reservation.
		_, e = tx.ExecContext(ctx, `INSERT INTO public_poll_sources(source_id,interval_us,max_bytes,next_at) VALUES($1,$2,$3,$4)
ON CONFLICT(source_id) DO UPDATE SET interval_us=excluded.interval_us,max_bytes=excluded.max_bytes,next_at=greatest(public_poll_sources.next_at,excluded.next_at),claim_id=NULL WHERE public_poll_sources.interval_us!=excluded.interval_us OR public_poll_sources.max_bytes!=excluded.max_bytes OR $5::bigint>0`, id, v.Interval.Microseconds(), v.MaxBytes, s.queryTime().UnixMicro(), changed)
		return e
	})
	return id, e
}

// SubscribePublicSource creates only a private membership. No source, polling
// job, article, grant or key is copied. Active workspace and feed are required.
func (s *Store) SubscribePublicSource(ctx context.Context, p identity.Principal, feed, source string, enabled bool) error {
	tenant, e := access(p, true, feed, source)
	if e != nil {
		return e
	}
	return s.publicWrite(ctx, func(tx *sql.Tx) error {
		if !enabled {
			_, e := tx.ExecContext(ctx, `DELETE FROM workspace_public_subscriptions WHERE tenant_id=$1 AND feed_id=$2 AND source_id=$3`, string(tenant), feed, source)
			return e
		}
		var allowed bool
		e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM customer_workspaces w JOIN feeds f ON f.tenant_id=w.tenant_id CROSS JOIN public_sources s WHERE w.tenant_id=$1 AND w.state='active' AND f.id=$2 AND f.enabled=1 AND s.id=$3 AND s.enabled=1 AND s.review_state='approved' AND s.rights_basis='reviewed_metadata')`, string(tenant), feed, source).Scan(&allowed)
		if e != nil {
			return e
		}
		if !allowed {
			return ErrNotFound
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO workspace_public_subscriptions(tenant_id,feed_id,source_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, string(tenant), feed, source)
		return e
	})
}

// ClaimPublicPoll has no tenant argument: public acquisition works at zero
// workspaces. Admission is serial across public AND private acquisition.
func (s *Store) ClaimPublicPoll(ctx context.Context) (ingest.Claim, error) {
	var c ingest.Claim
	var outcome error
	e := s.publicWrite(ctx, func(tx *sql.Tx) error {
		now := s.queryTime()
		if !validTimestamp(now) || !validTimestamp(now.Add(7*24*time.Hour)) {
			return ingest.ErrInvalid
		}
		at := now.UnixMicro()
		before := now.Add(-24 * time.Hour).UnixMicro()
		for _, q := range []string{
			`UPDATE public_poll_sources SET claim_id=NULL,last_error='abandoned' WHERE claim_id IN (SELECT id FROM public_poll_attempts WHERE state='pending' AND lease_until<=$1)`,
			`UPDATE public_poll_attempts SET state='abandoned',charged_at=lease_until WHERE state='pending' AND lease_until<=$1`,
			`UPDATE poll_sources SET claim_id=NULL,last_error='abandoned' WHERE claim_id IN (SELECT id FROM poll_attempts WHERE state='pending' AND lease_until<=$1)`,
			`UPDATE poll_attempts SET state='abandoned',charged_at=lease_until WHERE state='pending' AND lease_until<=$1`,
		} {
			if _, e := tx.ExecContext(ctx, q, at); e != nil {
				return e
			}
		}
		var active int64
		if e := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM poll_attempts WHERE state='pending')+(SELECT count(*) FROM public_poll_attempts WHERE state='pending')`).Scan(&active); e != nil {
			return e
		}
		if active > 0 {
			outcome = ingest.ErrBusy
			return nil
		}
		var interval int64
		e := tx.QueryRowContext(ctx, `SELECT p.source_id,s.canonical_url,p.etag,p.modified,p.max_bytes,p.interval_us FROM public_poll_sources p JOIN public_sources s ON s.id=p.source_id
WHERE s.enabled=1 AND s.review_state='approved' AND s.rights_basis='reviewed_metadata' AND p.next_at<=$1
AND NOT EXISTS(SELECT 1 FROM collection_hosts h WHERE h.host=split_part(split_part(s.canonical_url,'/',3),':',1) AND h.next_at>$1)
ORDER BY p.last_attempt NULLS FIRST,p.source_id LIMIT 1 FOR UPDATE OF p SKIP LOCKED`, at).Scan(&c.SourceID, &c.URL, &c.ETag, &c.LastModified, &c.MaxBytes, &interval)
		if errors.Is(e, sql.ErrNoRows) {
			outcome = ingest.ErrIdle
			return nil
		}
		if e != nil {
			return e
		}
		var attempts, used int64
		e = tx.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(charged_bytes),0)::bigint FROM (
SELECT charged_bytes FROM poll_attempts WHERE state='pending' OR charged_at>$1 UNION ALL SELECT charged_bytes FROM public_poll_attempts WHERE state='pending' OR charged_at>$1 UNION ALL SELECT charged_bytes FROM erased_acquisition_usage WHERE charged_at>$1) accounting`, before).Scan(&attempts, &used)
		if e != nil {
			return e
		}
		if attempts >= ingest.DailyAttempts || c.MaxBytes > ingest.DailyBytes-used {
			outcome = ingest.ErrBudget
			return nil
		}
		c.ID = queryID()
		c.Until = now.Add(ingest.LeaseDuration)
		c.Mode = "feed"
		if _, e = tx.ExecContext(ctx, `INSERT INTO public_poll_attempts(id,source_id,started_at,lease_until,charged_at,charged_bytes,reserved_bytes,state) VALUES($1,$2,$3,$4,$3,$5,$5,'pending')`, c.ID, c.SourceID, at, c.Until.UnixMicro(), c.MaxBytes); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE public_poll_sources SET claim_id=$1,last_attempt=$2,next_at=$2+interval_us,last_error='pending' WHERE source_id=$3`, c.ID, at, c.SourceID); e != nil {
			return e
		}
		u, _ := url.Parse(c.URL)
		_, e = tx.ExecContext(ctx, `INSERT INTO collection_hosts(host,next_at) VALUES($1,$2) ON CONFLICT(host) DO UPDATE SET next_at=greatest(collection_hosts.next_at,excluded.next_at)`, u.Hostname(), c.Until.UnixMicro())
		return e
	})
	if e != nil {
		return ingest.Claim{}, e
	}
	if outcome != nil {
		return ingest.Claim{}, outcome
	}
	return c, nil
}

func (s *Store) FinishPublicPoll(ctx context.Context, id string, r ingest.Result) error {
	if identity.ValidateID(id) != nil || !validResult(r) || len(r.Observations) != 0 || r.Failure == "" && r.Status != 200 && r.Status != 304 {
		return ingest.ErrInvalid
	}
	return s.publicWrite(ctx, func(tx *sql.Tx) error {
		now := s.queryTime()
		var source, etag, modified, rawURL string
		var reserved, until int64
		e := tx.QueryRowContext(ctx, `SELECT a.source_id,a.reserved_bytes,a.lease_until,p.etag,p.modified,s.canonical_url FROM public_poll_attempts a JOIN public_poll_sources p ON p.source_id=a.source_id AND p.claim_id=a.id JOIN public_sources s ON s.id=a.source_id WHERE a.id=$1 AND a.state='pending' AND s.enabled=1 AND s.review_state='approved' AND s.rights_basis='reviewed_metadata'`, id).Scan(&source, &reserved, &until, &etag, &modified, &rawURL)
		if errors.Is(e, sql.ErrNoRows) || e == nil && until <= now.UnixMicro() {
			return ingest.ErrLease
		}
		if e != nil {
			return e
		}
		if r.Bytes > reserved || r.Status == 304 && etag == "" && modified == "" {
			return ingest.ErrInvalid
		}
		if r.Failure == "" && r.Status == 200 {
			for _, v := range r.Items {
				articleID := itemIDFor("northcloud.public.article.v1:"+source, v.OriginID)
				var published sql.NullInt64
				if v.PublishedAt != nil {
					published = sql.NullInt64{Int64: v.PublishedAt.UnixMicro(), Valid: true}
				}
				sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%v:%d", v.Title, v.URL, published.Valid, published.Int64)))
				hash := hex.EncodeToString(sum[:])
				result, e := tx.ExecContext(ctx, `INSERT INTO public_articles(id,source_id,origin_id,url,title,content_hash,published_at,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT(source_id,origin_id) DO UPDATE SET url=excluded.url,title=excluded.title,content_hash=excluded.content_hash,published_at=excluded.published_at,observed_at=excluded.observed_at WHERE public_articles.content_hash!=excluded.content_hash`, articleID, source, v.OriginID, v.URL, v.Title, hash, published, now.UnixMicro())
				if e != nil {
					return e
				}
				n, e := result.RowsAffected()
				if e != nil {
					return e
				}
				if n > 0 {
					if _, e = tx.ExecContext(ctx, `INSERT INTO public_article_versions(article_id,content_hash,title,url,published_at,observed_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, articleID, hash, v.Title, v.URL, published, now.UnixMicro()); e != nil {
						return e
					}
				}
			}
			var items, versions int64
			if e := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM public_articles WHERE source_id=$1),(SELECT count(*) FROM public_article_versions v JOIN public_articles a ON a.id=v.article_id WHERE a.source_id=$1)`, source).Scan(&items, &versions); e != nil {
				return e
			}
			if items > ingest.MaxSourceItems || versions > ingest.MaxSourceVersions {
				return ingest.ErrCorpusFull
			}
		}
		charged := reserved
		if r.Failure == "" {
			charged = r.Bytes
		}
		if _, e = tx.ExecContext(ctx, `UPDATE public_poll_attempts SET state='done',charged_bytes=$1,charged_at=$2 WHERE id=$3`, charged, now.UnixMicro(), id); e != nil {
			return e
		}
		hold := int64(0)
		if !r.NotBefore.IsZero() {
			hold = r.NotBefore.UnixMicro()
		}
		if r.Status == 304 {
			if r.ETag == "" {
				r.ETag = etag
			}
			if r.LastModified == "" {
				r.LastModified = modified
			}
		}
		if r.Failure != "" {
			r.ETag = etag
			r.LastModified = modified
		}
		_, e = tx.ExecContext(ctx, `UPDATE public_poll_sources SET claim_id=NULL,last_status=$1,last_error=$2,etag=$3,modified=$4,next_at=greatest(next_at,$5),last_success=CASE WHEN $2='' THEN $6 ELSE last_success END WHERE source_id=$7`, r.Status, r.Failure, r.ETag, r.LastModified, hold, now.UnixMicro(), source)
		if e != nil {
			return e
		}
		u, _ := url.Parse(rawURL)
		_, e = tx.ExecContext(ctx, `UPDATE collection_hosts SET next_at=greatest($2::bigint,$3::bigint) WHERE host=$1`, u.Hostname(), now.Add(10*time.Second).UnixMicro(), hold)
		return e
	})
}

// RunPublicPollOnce reuses the guarded native Fetcher contract. It is an
// explicit local seam, not connected to CustomerPublisher or a startup job.
func (s *Store) RunPublicPollOnce(ctx context.Context, f ingest.Fetcher) (ingest.Result, error) {
	c, e := s.ClaimPublicPoll(ctx)
	if e != nil {
		return ingest.Result{}, e
	}
	fetchCtx, cancel := context.WithTimeout(ctx, ingest.FetchTimeout)
	r := f.Fetch(fetchCtx, c)
	cancel()
	if ctx.Err() != nil {
		r = ingest.Result{Failure: "cancelled", Status: r.Status}
	}
	finishCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer done()
	e = s.FinishPublicPoll(finishCtx, c.ID, r)
	if errors.Is(e, ingest.ErrCorpusFull) {
		e = errors.Join(e, s.FinishPublicPoll(finishCtx, c.ID, ingest.Result{Failure: "corpus_full", Status: r.Status}))
	}
	if e != nil {
		return ingest.Result{}, e
	}
	if r.Failure != "" {
		return r, ingest.ErrFetch
	}
	return r, nil
}
