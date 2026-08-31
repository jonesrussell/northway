-- name: FeedbackEvent :one
SELECT * FROM feedback_events WHERE tenant_id=? AND id=?;
-- name: FeedbackReversal :one
SELECT id FROM feedback_events WHERE tenant_id=? AND reverses_event_id=?;
-- name: CreateFeedbackEvent :execrows
INSERT INTO feedback_events(tenant_id,id,snapshot_id,article_id,feed_id,action,reverses_event_id,created_at) VALUES(?,?,?,?,?,?,?,?);
-- name: AdvanceFeedbackRevision :execrows
UPDATE feeds SET revision=revision+1 WHERE tenant_id=? AND id=? AND enabled=1;
