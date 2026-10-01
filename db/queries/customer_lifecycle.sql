-- name: SuspendCustomer :execrows
UPDATE customer_workspaces SET state='suspended' WHERE tenant_id=? AND state='active';
-- name: DeleteCustomerMarker :exec
UPDATE customer_workspaces SET state='deleted' WHERE tenant_id=?;
-- name: DeleteCustomerKeyExpiries :exec
DELETE FROM customer_key_expiry WHERE key_id IN (SELECT id FROM api_keys WHERE tenant_id=?);
-- name: DeleteCustomerKeys :exec
DELETE FROM api_keys WHERE tenant_id=?;
-- name: DeleteCustomerFeedback :exec
DELETE FROM feedback_events WHERE tenant_id=?;
-- name: DeleteCustomerWork :exec
DELETE FROM query_work WHERE tenant_id=?;
-- name: DeleteCustomerSnapshots :exec
DELETE FROM query_snapshots WHERE tenant_id=?;
-- name: DeleteCustomerPollAttempts :exec
DELETE FROM poll_attempts WHERE tenant_id=?;
-- name: PreserveErasedAcquisitionUsage :exec
INSERT INTO erased_acquisition_usage(charged_at,charged_bytes)
SELECT max(charged_at,lease_until),charged_bytes FROM poll_attempts WHERE tenant_id=?;
-- name: ExpireErasedAcquisitionUsage :exec
DELETE FROM erased_acquisition_usage WHERE charged_at<=?;
-- name: DeleteCustomerPollSources :exec
DELETE FROM poll_sources WHERE tenant_id=?;
-- name: DeleteCustomerPollCursors :exec
DELETE FROM poll_cursors WHERE tenant_id=?;
-- name: DeleteCustomerFeedSources :exec
DELETE FROM feed_sources WHERE tenant_id=?;
-- name: DeleteCustomerFeeds :exec
DELETE FROM feeds WHERE tenant_id=?;
-- name: DeleteCustomerArticles :exec
DELETE FROM articles WHERE tenant_id=?;
-- name: DeleteCustomerSources :exec
DELETE FROM sources WHERE tenant_id=?;
-- name: DeleteCustomerBudgets :exec
DELETE FROM budgets WHERE tenant_id=?;
-- name: DeleteCustomerRequests :exec
DELETE FROM request_budgets WHERE tenant_id=?;
-- name: ExportCustomerFeeds :many
SELECT id,title,enabled FROM feeds WHERE tenant_id=? ORDER BY id LIMIT 100 OFFSET ?;
-- name: ExportCustomerSources :many
SELECT id,url,title,enabled FROM sources WHERE tenant_id=? ORDER BY id LIMIT 100 OFFSET ?;
-- name: ExportCustomerSnapshots :many
SELECT id,feed_id,mode,generated_at,expires_at,items FROM query_snapshots WHERE tenant_id=? ORDER BY id LIMIT 10 OFFSET ?;
-- name: ExportCustomerFeedback :many
SELECT id,snapshot_id,article_id,feed_id,action,reverses_event_id,created_at FROM feedback_events WHERE tenant_id=? ORDER BY id LIMIT 100 OFFSET ?;
-- name: ExportCustomerArticles :many
SELECT id,source_id,url,title,published_at,observed_at FROM articles WHERE tenant_id=? ORDER BY id LIMIT 100 OFFSET ?;
