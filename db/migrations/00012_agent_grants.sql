-- +goose Up
CREATE TABLE agent_grants(
 id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL REFERENCES tenants(id),
 digest BLOB NOT NULL CHECK(length(digest)=32),
 scopes INTEGER NOT NULL CHECK(scopes BETWEEN 1 AND 7),
 created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,
 revoked_at INTEGER,last_used_at INTEGER,
 label TEXT NOT NULL CHECK(length(label) BETWEEN 1 AND 128),
 CHECK(expires_at>created_at AND expires_at-created_at<=86400000000)
) STRICT;
CREATE INDEX agent_grant_tenant ON agent_grants(tenant_id);
