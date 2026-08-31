-- name: FeedPreferences :one
SELECT preferences FROM feeds WHERE tenant_id=? AND id=? AND enabled=1;

-- name: ConfigureFeedPreferences :execrows
UPDATE feeds SET preferences=? WHERE tenant_id=? AND id=? AND enabled=1;

-- name: RetrievalSources :many
SELECT CAST(json_extract(r.value,'$.source_id') AS TEXT) AS source_id,
CAST(coalesce(s.title,'') AS TEXT) AS title,
CAST(CASE WHEN fs.source_id IS NOT NULL AND s.enabled=1 THEN 1 ELSE 0 END AS INTEGER) AS allowed,
p.last_success,CAST(coalesce(p.interval_us,0) AS INTEGER) AS interval_us
FROM feeds fd JOIN json_each(fd.preferences,'$.sources') r
LEFT JOIN feed_sources fs ON fs.tenant_id=fd.tenant_id AND fs.feed_id=fd.id AND fs.source_id=json_extract(r.value,'$.source_id')
LEFT JOIN sources s ON s.tenant_id=fs.tenant_id AND s.id=fs.source_id
LEFT JOIN poll_sources p ON p.tenant_id=s.tenant_id AND p.source_id=s.id
WHERE fd.tenant_id=? AND fd.id=? AND fd.enabled=1 ORDER BY source_id LIMIT 101;

-- name: RetrievalSourceAllowed :one
SELECT count(*) FROM feeds fd JOIN json_each(fd.preferences,'$.sources') r
JOIN feed_sources fs ON fs.tenant_id=fd.tenant_id AND fs.feed_id=fd.id AND fs.source_id=json_extract(r.value,'$.source_id')
JOIN sources s ON s.tenant_id=fs.tenant_id AND s.id=fs.source_id
WHERE fd.tenant_id=? AND fd.id=? AND fs.source_id=? AND fd.enabled=1 AND s.enabled=1;

-- name: SnapshotDetails :exec
UPDATE query_snapshots SET details=? WHERE tenant_id=? AND id=?;
