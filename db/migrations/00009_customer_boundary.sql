-- +goose Up
CREATE TABLE assertion_replays (
    issuer TEXT NOT NULL CHECK(length(issuer) BETWEEN 1 AND 256),
    id TEXT NOT NULL CHECK(length(id)=36),
    expires_at INTEGER NOT NULL,
    PRIMARY KEY(issuer,id)
) STRICT;
CREATE INDEX assertion_replays_expiry ON assertion_replays(expires_at);
CREATE TABLE customer_workspaces (
    tenant_id TEXT PRIMARY KEY NOT NULL REFERENCES tenants(id),
    created_at INTEGER NOT NULL
) STRICT;
CREATE TABLE customer_key_expiry (
    key_id TEXT PRIMARY KEY NOT NULL REFERENCES api_keys(id),
    expires_at INTEGER NOT NULL
) STRICT;
CREATE TABLE request_budgets (
    tenant_id TEXT NOT NULL,
    window INTEGER NOT NULL,
    used INTEGER NOT NULL CHECK(used BETWEEN 1 AND 60),
    PRIMARY KEY(tenant_id,window)
) STRICT;
