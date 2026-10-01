-- +goose Up
CREATE TABLE assertion_replays (
    issuer TEXT NOT NULL CHECK(length(issuer) BETWEEN 1 AND 256),
    id TEXT NOT NULL CHECK(length(id)=36),
    expires_at BIGINT NOT NULL,
    PRIMARY KEY(issuer,id)
);
CREATE INDEX assertion_replays_expiry ON assertion_replays(expires_at);
CREATE TABLE customer_workspaces (
    tenant_id TEXT PRIMARY KEY NOT NULL REFERENCES tenants(id),
    created_at BIGINT NOT NULL
);
CREATE TABLE customer_key_expiry (
    key_id TEXT PRIMARY KEY NOT NULL REFERENCES api_keys(id),
    expires_at BIGINT NOT NULL
);
CREATE TABLE request_budgets (
    tenant_id TEXT NOT NULL,
    budget_window BIGINT NOT NULL,
    used BIGINT NOT NULL CHECK(used BETWEEN 1 AND 60),
    PRIMARY KEY(tenant_id,budget_window)
);
