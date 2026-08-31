-- name: CreateTenant :exec
INSERT INTO tenants(id,created_at) VALUES(?,?);
-- name: CreateSource :exec
INSERT INTO sources(tenant_id,id,url,title) VALUES(?,?,?,?);
-- name: CreateFeed :exec
INSERT INTO feeds(tenant_id,id,title) VALUES(?,?,?);
-- name: AttachSource :exec
INSERT INTO feed_sources(tenant_id,feed_id,source_id) VALUES(?,?,?);
-- name: PutArticle :execrows
INSERT INTO articles(tenant_id,id,source_id,origin_id,url,title,body,content_hash,published_at,observed_at)
VALUES(?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(tenant_id,id) DO UPDATE SET
url=excluded.url,title=excluded.title,body=excluded.body,content_hash=excluded.content_hash,
published_at=excluded.published_at,observed_at=excluded.observed_at
WHERE articles.source_id=excluded.source_id AND articles.origin_id=excluded.origin_id;
-- name: RecordVersion :exec
INSERT INTO article_versions(tenant_id,article_id,content_hash,title,body,observed_at)
VALUES(?,?,?,?,?,?) ON CONFLICT(tenant_id,article_id,content_hash) DO NOTHING;
-- name: GetArticle :one
SELECT a.id,a.source_id,a.origin_id,a.url,a.title,a.body,a.content_hash,a.published_at,a.observed_at FROM articles a JOIN sources s ON s.tenant_id=a.tenant_id AND s.id=a.source_id WHERE a.tenant_id=? AND a.id=? AND s.enabled=1;
-- name: DeleteArticle :execrows
DELETE FROM articles WHERE tenant_id=? AND id=?;
