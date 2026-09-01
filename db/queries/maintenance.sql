-- name: DeleteExpiredQueryWork :execrows
DELETE FROM query_work WHERE rowid IN (
  SELECT rowid FROM query_work
  WHERE query_work.tenant_id=sqlc.arg(target_tenant)
    AND query_work.retain_until<=sqlc.arg(before_at)
    AND work_state IN ('done','failed')
    AND spend_state='settled'
  ORDER BY retain_until,rowid LIMIT 1000
);

-- name: DeleteExpiredQuerySnapshots :execrows
DELETE FROM query_snapshots WHERE rowid IN (
  SELECT query_snapshots.rowid FROM query_snapshots
  WHERE query_snapshots.tenant_id=sqlc.arg(target_tenant)
    AND query_snapshots.retain_until<=sqlc.arg(before_at)
    AND NOT EXISTS (
      SELECT 1 FROM query_work
      WHERE query_work.tenant_id=query_snapshots.tenant_id
        AND query_work.snapshot_id=query_snapshots.id
    )
	AND NOT EXISTS (
	  SELECT 1 FROM feedback_events saved
	  WHERE saved.tenant_id=query_snapshots.tenant_id
	    AND saved.snapshot_id=query_snapshots.id
	    AND saved.action='save'
	    AND NOT EXISTS (
	      SELECT 1 FROM feedback_events reversal
	      WHERE reversal.tenant_id=saved.tenant_id
	        AND reversal.reverses_event_id=saved.id
	    )
	)
  ORDER BY retain_until,query_snapshots.rowid LIMIT 1000
);

-- name: DeleteSupersededArticleVersions :execrows
DELETE FROM article_versions WHERE rowid IN (
  SELECT article_versions.rowid FROM article_versions
  WHERE article_versions.tenant_id=sqlc.arg(target_tenant)
    AND article_versions.observed_at<sqlc.arg(before_at)
    AND NOT EXISTS (
      SELECT 1 FROM articles
      WHERE articles.tenant_id=article_versions.tenant_id
        AND articles.id=article_versions.article_id
        AND articles.content_hash=article_versions.content_hash
    )
  ORDER BY observed_at,article_versions.rowid LIMIT 1000
);

-- name: DeleteExpiredArticles :execrows
DELETE FROM articles WHERE rowid IN (
  SELECT rowid FROM articles
  WHERE articles.tenant_id=sqlc.arg(target_tenant)
    AND articles.observed_at<sqlc.arg(before_at)
  ORDER BY observed_at,rowid LIMIT 1000
);

-- name: DeleteExpiredPollAttempts :execrows
DELETE FROM poll_attempts WHERE rowid IN (
  SELECT rowid FROM poll_attempts
  WHERE poll_attempts.tenant_id=sqlc.arg(target_tenant)
    AND poll_attempts.state!='pending'
    AND poll_attempts.charged_at<=sqlc.arg(before_at)
  ORDER BY charged_at,rowid LIMIT 1000
);

-- name: UnreconciledQueryWork :one
SELECT count(*) FROM query_work
WHERE query_work.tenant_id=sqlc.arg(target_tenant) AND query_work.spend_state='uncertain';

-- name: UnhealthyPollSources :one
SELECT count(*) FROM poll_sources ps
LEFT JOIN poll_attempts pa ON pa.id=ps.claim_id AND pa.tenant_id=ps.tenant_id
WHERE ps.tenant_id=sqlc.arg(target_tenant)
  AND ps.approved=1
  AND ps.enabled=1
  AND (
    ps.last_success IS NULL
    OR ps.last_success>CAST(sqlc.arg(now_at) AS INTEGER)
    OR ps.last_success<CAST(sqlc.arg(now_at) AS INTEGER)-(2*ps.interval_us)
    OR (ps.last_error!='' AND ps.last_error!='pending')
    OR (ps.claim_id IS NOT NULL AND (pa.id IS NULL OR pa.state!='pending' OR pa.lease_until<=CAST(sqlc.arg(now_at) AS INTEGER)))
  );
