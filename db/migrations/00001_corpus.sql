-- +goose Up
CREATE TABLE tenants (
    id TEXT PRIMARY KEY NOT NULL CHECK (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'),
    created_at INTEGER NOT NULL CHECK(created_at >= 0)
) STRICT;
CREATE TABLE sources (
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    id TEXT NOT NULL CHECK (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'),
    url TEXT NOT NULL CHECK(length(url) BETWEEN 1 AND 2048),
    title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 512),
    PRIMARY KEY(tenant_id,id),
    UNIQUE(tenant_id,url)
) STRICT;
CREATE TABLE articles (
    rowid INTEGER PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    id TEXT NOT NULL CHECK (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'),
    source_id TEXT NOT NULL,
    origin_id TEXT NOT NULL CHECK(length(origin_id) BETWEEN 1 AND 2048),
    url TEXT NOT NULL CHECK(length(url) BETWEEN 1 AND 2048),
    title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 512),
    body TEXT NOT NULL CHECK(length(CAST(body AS BLOB)) <= 65536),
    content_hash TEXT NOT NULL CHECK(length(content_hash)=64 AND content_hash NOT GLOB '*[^0-9a-f]*'),
    published_at INTEGER CHECK(published_at >= 0),
    observed_at INTEGER NOT NULL CHECK(observed_at >= 0),
    UNIQUE(tenant_id,id),
    UNIQUE(tenant_id,source_id,origin_id),
    FOREIGN KEY(tenant_id,source_id) REFERENCES sources(tenant_id,id)
) STRICT;
CREATE INDEX articles_by_tenant_observed ON articles(tenant_id,observed_at,id);
CREATE TABLE article_versions (
    tenant_id TEXT NOT NULL,
    article_id TEXT NOT NULL,
    content_hash TEXT NOT NULL CHECK(length(content_hash)=64 AND content_hash NOT GLOB '*[^0-9a-f]*'),
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    observed_at INTEGER NOT NULL CHECK(observed_at >= 0),
    PRIMARY KEY(tenant_id,article_id,content_hash),
    FOREIGN KEY(tenant_id,article_id) REFERENCES articles(tenant_id,id) ON DELETE CASCADE
) STRICT;
CREATE TABLE feeds (
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    id TEXT NOT NULL CHECK (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'),
    title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 512),
    PRIMARY KEY(tenant_id,id)
) STRICT;
CREATE TABLE feed_sources (
    tenant_id TEXT NOT NULL,
    feed_id TEXT NOT NULL,
    source_id TEXT NOT NULL,
    PRIMARY KEY(tenant_id,feed_id,source_id),
    FOREIGN KEY(tenant_id,feed_id) REFERENCES feeds(tenant_id,id) ON DELETE CASCADE,
    FOREIGN KEY(tenant_id,source_id) REFERENCES sources(tenant_id,id)
) STRICT;
