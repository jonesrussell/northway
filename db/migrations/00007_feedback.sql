-- +goose Up
-- Events survive corpus changes. Membership is checked against immutable
-- snapshots, not a foreign key to an article which retention may remove.
CREATE TABLE feedback_events (
 tenant_id TEXT NOT NULL,
 id TEXT NOT NULL CHECK(length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'),
 snapshot_id TEXT NOT NULL,
 article_id TEXT NOT NULL,
 feed_id TEXT NOT NULL,
 action TEXT NOT NULL CHECK(action IN ('save','dismiss','less_like_this','undo')),
 reverses_event_id TEXT,
 created_at INTEGER NOT NULL CHECK(created_at>=0),
 PRIMARY KEY(tenant_id,id),
 FOREIGN KEY(tenant_id,feed_id) REFERENCES feeds(tenant_id,id),
 FOREIGN KEY(tenant_id,reverses_event_id) REFERENCES feedback_events(tenant_id,id),
 CHECK((action='undo')=(reverses_event_id IS NOT NULL)),
 CHECK(reverses_event_id IS NULL OR reverses_event_id<>id)
) STRICT;
CREATE UNIQUE INDEX feedback_single_reversal ON feedback_events(tenant_id,reverses_event_id) WHERE reverses_event_id IS NOT NULL;
-- Revision change is performed by the adapter in the same transaction, with
-- affected-row checks. Identical replay neither inserts nor changes revision.
