-- name: CleanAssertionReplays :exec
DELETE FROM assertion_replays WHERE expires_at < ?;
-- name: CountAssertionReplays :one
SELECT count(*) FROM assertion_replays;
-- name: ConsumeAssertion :execrows
INSERT INTO assertion_replays(issuer,id,expires_at) VALUES(?,?,?) ON CONFLICT DO NOTHING;
-- name: EnsureCustomerWorkspace :exec
INSERT INTO customer_workspaces(tenant_id,created_at) VALUES(?,?) ON CONFLICT DO NOTHING;
-- name: CountCustomerWorkspaces :one
SELECT count(*) FROM customer_workspaces WHERE state<>'deleted';
-- name: ListCustomerWorkspaces :many
SELECT tenant_id FROM customer_workspaces WHERE state='active' ORDER BY tenant_id LIMIT 5;
-- name: RequireCustomerWorkspace :one
SELECT tenant_id FROM customer_workspaces WHERE tenant_id=? AND state='active';
-- name: CustomerWorkspaceState :one
SELECT state FROM customer_workspaces WHERE tenant_id=?;
-- name: CountCustomerKeys :one
SELECT count(*) FROM api_keys k JOIN customer_key_expiry e ON e.key_id=k.id
WHERE k.tenant_id=? AND k.revoked_at IS NULL AND e.expires_at>?;
-- name: CreateCustomerKeyExpiry :exec
INSERT INTO customer_key_expiry(key_id,expires_at) VALUES(?,?);
-- name: CustomerKeyExpiry :one
SELECT expires_at FROM customer_key_expiry WHERE key_id=?;
-- name: ListCustomerKeys :many
SELECT k.id,k.scopes,k.created_at,k.last_used_at,k.revoked_at,e.expires_at,
(k.revoked_at IS NULL AND e.expires_at>sqlc.arg(now)) AS active
FROM api_keys k JOIN customer_key_expiry e ON e.key_id=k.id
WHERE k.tenant_id=sqlc.arg(tenant_id) ORDER BY active DESC,k.created_at DESC,k.id LIMIT 100;
-- name: ListCustomerFeeds :many
SELECT id,title FROM feeds WHERE tenant_id=? AND enabled=1 ORDER BY id LIMIT 100;
-- name: CleanRequestBudgets :exec
DELETE FROM request_budgets WHERE window < ?;
-- name: ConsumeRequestBudget :execrows
INSERT INTO request_budgets(tenant_id,window,used) VALUES(?,?,1)
ON CONFLICT(tenant_id,window) DO UPDATE SET used=used+1 WHERE used<60;
