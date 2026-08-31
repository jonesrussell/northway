-- name: QueryScope :one
SELECT f.revision,t.corpus_revision,t.entitlement_revision FROM feeds f
JOIN tenants t ON t.id=f.tenant_id WHERE f.tenant_id=? AND f.id=? AND f.enabled=1;
-- name: QueryWorkByKey :one
SELECT * FROM query_work WHERE tenant_id=? AND key_hash=?;
-- name: QueryWorkByID :one
SELECT * FROM query_work WHERE tenant_id=? AND id=?;
-- name: CreateQueryWork :exec
INSERT INTO query_work(tenant_id,id,key_hash,request_hash,feed_id,feed_revision,corpus_revision,entitlement_revision,ranker_version,item_limit,since_at,created_at,lease_until,retain_until,cache_ttl,work_state,spend_state,reserved_micros,actual_micros,snapshot_id)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?);
-- name: UpdateQueryWork :exec
UPDATE query_work SET work_state=?,spend_state=?,actual_micros=?,snapshot_id=? WHERE tenant_id=? AND id=?;
-- name: ExpiredQueryWork :many
SELECT * FROM query_work WHERE tenant_id=? AND work_state='pending' AND lease_until<=? ORDER BY lease_until,id LIMIT 100;
-- name: FindQueryCache :one
SELECT * FROM query_snapshots WHERE tenant_id=? AND feed_id=? AND request_hash=? AND feed_revision=? AND corpus_revision=? AND entitlement_revision=? AND ranker_version=? AND expires_at>? ORDER BY generated_at DESC,id LIMIT 1;
-- name: GetQuerySnapshot :one
SELECT * FROM query_snapshots WHERE tenant_id=? AND id=? AND retain_until>?;
-- name: RetainQuerySnapshot :exec
UPDATE query_snapshots SET retain_until=max(retain_until,sqlc.arg(retain_until)) WHERE tenant_id=sqlc.arg(tenant_id) AND id=sqlc.arg(id);
-- name: CreateQuerySnapshot :exec
INSERT INTO query_snapshots(tenant_id,id,feed_id,request_hash,feed_revision,corpus_revision,entitlement_revision,ranker_version,mode,generated_at,expires_at,retain_until,items)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?);
-- name: QueryArticle :one
SELECT a.id,a.source_id,a.content_hash,a.title,a.url,a.observed_at FROM articles a
JOIN feed_sources f ON f.tenant_id=a.tenant_id AND f.source_id=a.source_id
JOIN sources s ON s.tenant_id=a.tenant_id AND s.id=a.source_id
WHERE a.tenant_id=? AND f.feed_id=? AND a.id=? AND s.enabled=1;
-- name: QuerySourceAllowed :one
SELECT count(*) FROM feed_sources f JOIN sources s ON s.tenant_id=f.tenant_id AND s.id=f.source_id
WHERE f.tenant_id=? AND f.feed_id=? AND f.source_id=? AND s.enabled=1;
-- name: GetBudget :one
SELECT * FROM budgets WHERE tenant_id=?;
-- name: SetBudget :execrows
INSERT INTO budgets(tenant_id,limit_micros) VALUES(?,?) ON CONFLICT(tenant_id) DO UPDATE SET limit_micros=excluded.limit_micros
WHERE budgets.spent_micros<=excluded.limit_micros AND budgets.held_micros<=excluded.limit_micros-budgets.spent_micros;
-- name: ReserveBudget :execrows
UPDATE budgets SET held_micros=held_micros+sqlc.arg(amount) WHERE tenant_id=sqlc.arg(tenant_id) AND sqlc.arg(amount)<=limit_micros-spent_micros-held_micros;
-- name: SettleBudget :execrows
UPDATE budgets SET held_micros=held_micros-sqlc.arg(reserved),spent_micros=spent_micros+sqlc.arg(actual) WHERE tenant_id=sqlc.arg(tenant_id) AND held_micros>=sqlc.arg(reserved);
-- name: SetSourceEnabled :execrows
UPDATE sources SET enabled=? WHERE tenant_id=? AND id=?;
-- name: SetFeedEnabled :execrows
UPDATE feeds SET enabled=? WHERE tenant_id=? AND id=?;
-- name: DetachSource :execrows
DELETE FROM feed_sources WHERE tenant_id=? AND feed_id=? AND source_id=?;
