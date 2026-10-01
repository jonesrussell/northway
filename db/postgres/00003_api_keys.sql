-- +goose Up
CREATE TABLE api_keys (
    id TEXT PRIMARY KEY NOT NULL CHECK(length(id)=32 AND id !~ '[^0-9a-f]'),
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    digest BYTEA NOT NULL CHECK(length(digest)=32),
    scopes BIGINT NOT NULL CHECK(scopes BETWEEN 1 AND 3),
    created_at BIGINT NOT NULL CHECK(created_at>=0),
    last_used_at BIGINT CHECK(last_used_at>=created_at),
    revoked_at BIGINT CHECK(revoked_at>=created_at),
    UNIQUE(tenant_id,id)
);
