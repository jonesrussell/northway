-- +goose Up
-- Collection remains opt-in even for pre-existing enabled sources.
CREATE TABLE poll_sources (
 tenant_id TEXT NOT NULL,
 source_id TEXT NOT NULL,
 approved_url TEXT NOT NULL,
 approved BIGINT NOT NULL DEFAULT 0 CHECK(approved IN (0,1)),
 enabled BIGINT NOT NULL DEFAULT 0 CHECK(enabled IN (0,1)),
 interval_us BIGINT NOT NULL CHECK(interval_us BETWEEN 3600000000 AND 604800000000),
 max_bytes BIGINT NOT NULL CHECK(max_bytes BETWEEN 1024 AND 2097152),
 next_at BIGINT NOT NULL CHECK(next_at>=0),
 etag TEXT NOT NULL DEFAULT '' CHECK(octet_length(etag)<=1024),
 modified TEXT NOT NULL DEFAULT '' CHECK(octet_length(modified)<=1024),
 last_success BIGINT CHECK(last_success>=0),
 last_attempt BIGINT CHECK(last_attempt>=0),
 last_status BIGINT NOT NULL DEFAULT 0 CHECK(last_status BETWEEN 0 AND 599),
 last_error TEXT NOT NULL DEFAULT '',
 claim_id TEXT,
 PRIMARY KEY(tenant_id,source_id),
 FOREIGN KEY(tenant_id,source_id) REFERENCES sources(tenant_id,id)
);
CREATE TABLE poll_attempts (
 id TEXT PRIMARY KEY NOT NULL,
 tenant_id TEXT NOT NULL,
 source_id TEXT NOT NULL,
 started_at BIGINT NOT NULL CHECK(started_at>=0),
 lease_until BIGINT NOT NULL CHECK(lease_until>started_at),
 charged_at BIGINT NOT NULL CHECK(charged_at>=started_at),
 charged_bytes BIGINT NOT NULL CHECK(charged_bytes BETWEEN 0 AND 2097152),
 reserved_bytes BIGINT NOT NULL CHECK(reserved_bytes BETWEEN 1024 AND 2097152),
 state TEXT NOT NULL CHECK(state IN ('pending','done','abandoned')),
 FOREIGN KEY(tenant_id,source_id) REFERENCES sources(tenant_id,id)
);
CREATE INDEX poll_attempt_window ON poll_attempts(charged_at);
CREATE TABLE poll_cursors (
 tenant_id TEXT PRIMARY KEY NOT NULL REFERENCES tenants(id),
 source_id TEXT NOT NULL DEFAULT ''
);
