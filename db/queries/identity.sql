-- name: CreateAPIKey :exec
INSERT INTO api_keys(id,tenant_id,digest,scopes,created_at) VALUES(?,?,?,?,?);
-- name: LookupAPIKey :one
SELECT id,tenant_id,digest,scopes,created_at,last_used_at,revoked_at FROM api_keys WHERE id=?;
-- name: TouchAPIKey :execrows
UPDATE api_keys SET last_used_at=max(coalesce(last_used_at,created_at),?)
WHERE tenant_id=? AND id=? AND revoked_at IS NULL;
-- name: RevokeAPIKey :execrows
UPDATE api_keys SET revoked_at=coalesce(revoked_at,max(created_at,?)) WHERE tenant_id=? AND id=?;
-- name: GetFeed :one
SELECT id,title FROM feeds WHERE tenant_id=? AND id=? AND enabled=1;
