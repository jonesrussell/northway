-- name: PilotPollConfig :one
SELECT approved_url,approved,enabled,interval_us,max_bytes FROM poll_sources WHERE tenant_id=sqlc.arg(tenant_id) AND source_id=sqlc.arg(source_id);

-- name: PollSourceURL :one
SELECT url FROM sources WHERE tenant_id=sqlc.arg(tenant_id) AND id=sqlc.arg(source_id);

-- name: OtherPollSources :one
SELECT count(*) FROM poll_sources WHERE NOT (tenant_id=sqlc.arg(tenant_id) AND source_id=sqlc.arg(source_id));

-- name: ConfigurePoll :exec
INSERT INTO poll_sources(tenant_id,source_id,approved_url,approved,enabled,interval_us,max_bytes,next_at)
VALUES(sqlc.arg(tenant_id),sqlc.arg(source_id),sqlc.arg(approved_url),sqlc.arg(approved),sqlc.arg(enabled),sqlc.arg(interval_us),sqlc.arg(max_bytes),sqlc.arg(next_at)) ON CONFLICT(tenant_id,source_id) DO UPDATE SET
approved_url=excluded.approved_url,approved=excluded.approved,enabled=excluded.enabled,
interval_us=excluded.interval_us,max_bytes=excluded.max_bytes,
next_at=max(poll_sources.next_at,excluded.next_at),etag='',modified='',claim_id=NULL;

-- name: AbandonPollSources :exec
UPDATE poll_sources SET claim_id=NULL,last_error='abandoned'
WHERE claim_id IN (SELECT id FROM poll_attempts WHERE state='pending' AND lease_until<=sqlc.arg(now_at));

-- name: AbandonPollAttempts :exec
UPDATE poll_attempts SET state='abandoned',charged_at=lease_until WHERE state='pending' AND lease_until<=sqlc.arg(now_at);

-- name: ExpirePollAttempts :exec
DELETE FROM poll_attempts WHERE state!='pending' AND charged_at<=sqlc.arg(before_at);

-- name: ActivePollAttempts :one
SELECT count(*) FROM poll_attempts WHERE state='pending';

-- name: PollCursor :one
SELECT source_id FROM poll_cursors WHERE tenant_id=sqlc.arg(tenant_id);

-- name: NextPollSources :many
SELECT ps.source_id,ps.approved_url,ps.etag,ps.modified,ps.max_bytes,ps.interval_us
FROM poll_sources ps JOIN sources s ON s.tenant_id=ps.tenant_id AND s.id=ps.source_id
WHERE ps.tenant_id=sqlc.arg(tenant_id) AND ps.enabled=1 AND ps.approved=1 AND s.enabled=1 AND s.url=ps.approved_url AND ps.next_at<=sqlc.arg(now_at)
ORDER BY ps.source_id LIMIT 100;

-- name: PollWindow :one
SELECT count(*) AS attempts, CAST(coalesce(sum(charged_bytes),0) AS INTEGER) AS used FROM poll_attempts;

-- name: InsertPollAttempt :exec
INSERT INTO poll_attempts(id,tenant_id,source_id,started_at,lease_until,charged_at,charged_bytes,reserved_bytes,state) VALUES(sqlc.arg(id),sqlc.arg(tenant_id),sqlc.arg(source_id),sqlc.arg(started_at),sqlc.arg(lease_until),sqlc.arg(charged_at),sqlc.arg(charged_bytes),sqlc.arg(reserved_bytes),'pending');

-- name: MarkPollStarted :exec
UPDATE poll_sources SET claim_id=sqlc.arg(claim_id),last_attempt=sqlc.arg(last_attempt),next_at=sqlc.arg(next_at),last_error='pending' WHERE tenant_id=sqlc.arg(tenant_id) AND source_id=sqlc.arg(source_id);

-- name: AdvancePollCursor :exec
INSERT INTO poll_cursors(tenant_id,source_id) VALUES(sqlc.arg(tenant_id),sqlc.arg(source_id)) ON CONFLICT(tenant_id) DO UPDATE SET source_id=excluded.source_id;

-- name: PendingPoll :one
SELECT a.source_id,a.reserved_bytes,a.lease_until,ps.etag,ps.modified
FROM poll_attempts a JOIN poll_sources ps ON ps.tenant_id=a.tenant_id AND ps.source_id=a.source_id AND ps.claim_id=a.id
JOIN sources s ON s.tenant_id=ps.tenant_id AND s.id=ps.source_id
WHERE a.tenant_id=sqlc.arg(tenant_id) AND a.id=sqlc.arg(id) AND a.state='pending' AND ps.enabled=1 AND ps.approved=1 AND s.enabled=1 AND s.url=ps.approved_url;

-- name: SourceItemCount :one
SELECT count(*) FROM articles WHERE tenant_id=sqlc.arg(tenant_id) AND source_id=sqlc.arg(source_id);

-- name: SourceVersionCount :one
SELECT count(*) FROM article_versions v JOIN articles a ON a.tenant_id=v.tenant_id AND a.id=v.article_id WHERE a.tenant_id=sqlc.arg(tenant_id) AND a.source_id=sqlc.arg(source_id);

-- name: SettlePollAttempt :exec
UPDATE poll_attempts SET state='done',charged_bytes=sqlc.arg(charged_bytes),charged_at=sqlc.arg(charged_at) WHERE tenant_id=sqlc.arg(tenant_id) AND id=sqlc.arg(id);

-- name: MarkPollFailure :exec
UPDATE poll_sources SET claim_id=NULL,last_error=sqlc.arg(last_error),last_status=sqlc.arg(last_status),next_at=max(next_at,sqlc.arg(hold_until)) WHERE tenant_id=sqlc.arg(tenant_id) AND source_id=sqlc.arg(source_id);

-- name: MarkPollSuccess :exec
UPDATE poll_sources SET claim_id=NULL,last_success=sqlc.arg(last_success),last_status=sqlc.arg(last_status),last_error='',etag=sqlc.arg(etag),modified=sqlc.arg(modified),next_at=max(next_at,sqlc.arg(hold_until)) WHERE tenant_id=sqlc.arg(tenant_id) AND source_id=sqlc.arg(source_id);

-- name: PollItemIdentity :one
SELECT source_id,origin_id FROM articles WHERE tenant_id=sqlc.arg(tenant_id) AND id=sqlc.arg(id);

-- name: PutPollItem :execrows
INSERT INTO articles(tenant_id,id,source_id,origin_id,url,title,body,content_hash,published_at,observed_at)
VALUES(sqlc.arg(tenant_id),sqlc.arg(id),sqlc.arg(source_id),sqlc.arg(origin_id),sqlc.arg(url),sqlc.arg(title),'',sqlc.arg(content_hash),sqlc.arg(published_at),sqlc.arg(observed_at)) ON CONFLICT(tenant_id,id) DO UPDATE SET url=excluded.url,title=excluded.title,body='',content_hash=excluded.content_hash,published_at=excluded.published_at,observed_at=excluded.observed_at
WHERE articles.content_hash!=excluded.content_hash OR articles.url!=excluded.url OR articles.published_at IS NOT excluded.published_at OR articles.body!='';

-- name: PollItemByOrigin :one
SELECT id FROM articles WHERE tenant_id=sqlc.arg(tenant_id) AND source_id=sqlc.arg(source_id) AND origin_id=sqlc.arg(origin_id);

-- name: ResetPollSchedule :execrows
UPDATE poll_sources SET next_at=max(CAST(sqlc.arg(next_at) AS INTEGER),coalesce(last_attempt+interval_us,0)),
last_error=CASE WHEN claim_id IS NOT NULL THEN 'reset' ELSE last_error END,claim_id=NULL
WHERE tenant_id=sqlc.arg(tenant_id) AND source_id=sqlc.arg(source_id);
