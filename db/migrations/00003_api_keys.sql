-- +goose Up
CREATE TABLE api_keys (
    id TEXT PRIMARY KEY NOT NULL CHECK(length(id)=32 AND id NOT GLOB '*[^0-9a-f]*'),
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    digest BLOB NOT NULL CHECK(length(digest)=32),
    scopes INTEGER NOT NULL CHECK(scopes BETWEEN 1 AND 3),
    created_at INTEGER NOT NULL CHECK(created_at>=0),
    last_used_at INTEGER CHECK(last_used_at>=created_at),
    revoked_at INTEGER CHECK(revoked_at>=created_at),
    UNIQUE(tenant_id,id)
) STRICT;
