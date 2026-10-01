-- +goose Up
CREATE INDEX query_work_retention ON query_work(tenant_id,retain_until,work_state,spend_state);
CREATE INDEX query_work_snapshot ON query_work(tenant_id,snapshot_id);
CREATE INDEX query_snapshot_retention ON query_snapshots(tenant_id,retain_until);
CREATE INDEX article_version_retention ON article_versions(tenant_id,observed_at);
CREATE INDEX feedback_saved_snapshot ON feedback_events(tenant_id,snapshot_id,action);
CREATE INDEX poll_attempt_retention ON poll_attempts(tenant_id,charged_at,state);
