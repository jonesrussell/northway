-- +goose Up
ALTER TABLE poll_sources ADD COLUMN mode TEXT NOT NULL DEFAULT 'feed' CHECK(mode IN ('feed','html'));
ALTER TABLE poll_sources ADD COLUMN preview_allowed INTEGER NOT NULL DEFAULT 0 CHECK(preview_allowed IN (0,1));
ALTER TABLE poll_sources ADD COLUMN robots_until INTEGER NOT NULL DEFAULT 0;
CREATE TABLE collection_items(
 tenant_id TEXT NOT NULL,source_id TEXT NOT NULL,id TEXT NOT NULL,revision INTEGER NOT NULL,
 fingerprint TEXT NOT NULL,payload TEXT NOT NULL CHECK(length(CAST(payload AS BLOB))<=16384),
 PRIMARY KEY(tenant_id,id),FOREIGN KEY(tenant_id,source_id) REFERENCES sources(tenant_id,id)
) STRICT;
CREATE TABLE collection_events(
 sequence INTEGER PRIMARY KEY AUTOINCREMENT,tenant_id TEXT NOT NULL,source_id TEXT NOT NULL,
 item_id TEXT NOT NULL,revision INTEGER NOT NULL,payload TEXT NOT NULL CHECK(length(CAST(payload AS BLOB))<=16384),
 UNIQUE(tenant_id,item_id,revision),FOREIGN KEY(tenant_id,source_id) REFERENCES sources(tenant_id,id)
) STRICT;
CREATE INDEX collection_event_tenant_cursor ON collection_events(tenant_id,sequence);
CREATE TABLE collection_hosts(host TEXT PRIMARY KEY,next_at INTEGER NOT NULL) STRICT;

CREATE UNIQUE INDEX collection_seed_url ON poll_sources(tenant_id,approved_url) WHERE mode='html';
