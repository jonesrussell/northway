-- name: SetCollectionMode :exec
UPDATE poll_sources SET mode='html',preview_allowed=sqlc.arg(preview_allowed),robots_until=sqlc.arg(robots_until) WHERE tenant_id=sqlc.arg(tenant_id) AND source_id=sqlc.arg(source_id);
-- name: HoldCollectionHost :exec
INSERT INTO collection_hosts(host,next_at) VALUES(sqlc.arg(host),sqlc.arg(next_at)) ON CONFLICT(host) DO UPDATE SET next_at=max(next_at,excluded.next_at);
-- name: CollectionHostDue :one
SELECT CAST(coalesce((SELECT next_at FROM collection_hosts WHERE host=sqlc.arg(host)),0) AS INTEGER);
-- name: CollectionItem :one
SELECT revision,fingerprint,payload FROM collection_items WHERE tenant_id=sqlc.arg(tenant_id) AND id=sqlc.arg(id);
-- name: PutCollectionItem :exec
INSERT INTO collection_items(tenant_id,source_id,id,revision,fingerprint,payload) VALUES(sqlc.arg(tenant_id),sqlc.arg(source_id),sqlc.arg(id),sqlc.arg(revision),sqlc.arg(fingerprint),sqlc.arg(payload)) ON CONFLICT(tenant_id,id) DO UPDATE SET revision=excluded.revision,fingerprint=excluded.fingerprint,payload=excluded.payload;
-- name: AppendCollectionEvent :exec
INSERT INTO collection_events(tenant_id,source_id,item_id,revision,payload) VALUES(sqlc.arg(tenant_id),sqlc.arg(source_id),sqlc.arg(item_id),sqlc.arg(revision),sqlc.arg(payload));
-- name: CollectionSourceItems :many
SELECT payload FROM collection_items WHERE tenant_id=sqlc.arg(tenant_id) AND source_id=sqlc.arg(source_id) ORDER BY id LIMIT 5001;
-- name: CollectionSourceCount :one
SELECT count(*) FROM collection_items WHERE tenant_id=sqlc.arg(tenant_id) AND source_id=sqlc.arg(source_id);
-- name: CollectionVersionCount :one
SELECT count(*) FROM collection_events WHERE tenant_id=sqlc.arg(tenant_id) AND source_id=sqlc.arg(source_id);
-- name: CollectionEvents :many
SELECT sequence,payload FROM collection_events WHERE tenant_id=sqlc.arg(tenant_id) AND sequence>sqlc.arg(after_cursor) ORDER BY sequence LIMIT 100;

-- name: DeleteCollectionEvents :exec
DELETE FROM collection_events WHERE tenant_id=?;
-- name: DeleteCollectionItems :exec
DELETE FROM collection_items WHERE tenant_id=?;

-- name: CollectionSeedMode :one
SELECT mode FROM poll_sources WHERE tenant_id=sqlc.arg(tenant_id) AND source_id=sqlc.arg(source_id);
-- name: CollectionStatus :one
SELECT CAST((SELECT count(*) FROM poll_sources ps WHERE ps.tenant_id=sqlc.arg(tenant_id) AND ps.mode='html') AS INTEGER) AS seeds,
CAST((SELECT count(*) FROM poll_sources ps WHERE ps.tenant_id=sqlc.arg(tenant_id) AND ps.mode='html' AND approved=1 AND enabled=1) AS INTEGER) AS enabled,
CAST((SELECT count(*) FROM collection_items ci WHERE ci.tenant_id=sqlc.arg(tenant_id)) AS INTEGER) AS items,
CAST((SELECT count(*) FROM collection_events ce WHERE ce.tenant_id=sqlc.arg(tenant_id)) AS INTEGER) AS revisions;
