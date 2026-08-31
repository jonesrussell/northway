-- +goose Up
ALTER TABLE tenants ADD COLUMN corpus_revision INTEGER NOT NULL DEFAULT 1 CHECK(corpus_revision>=1);
ALTER TABLE tenants ADD COLUMN entitlement_revision INTEGER NOT NULL DEFAULT 1 CHECK(entitlement_revision>=1);
ALTER TABLE feeds ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK(revision>=1);
ALTER TABLE feeds ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1));
ALTER TABLE sources ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1));

CREATE TABLE budgets (
    tenant_id TEXT PRIMARY KEY NOT NULL REFERENCES tenants(id),
    limit_micros INTEGER NOT NULL CHECK(limit_micros>=0),
    spent_micros INTEGER NOT NULL DEFAULT 0 CHECK(spent_micros>=0),
    held_micros INTEGER NOT NULL DEFAULT 0 CHECK(held_micros>=0),
    CHECK(spent_micros<=limit_micros AND held_micros<=limit_micros-spent_micros)
) STRICT;
CREATE TABLE query_work (
    tenant_id TEXT NOT NULL,
    id TEXT NOT NULL CHECK(length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'),
    key_hash BLOB NOT NULL CHECK(length(key_hash)=32),
    request_hash BLOB NOT NULL CHECK(length(request_hash)=32),
    feed_id TEXT NOT NULL,
    feed_revision INTEGER NOT NULL CHECK(feed_revision>=1),
    corpus_revision INTEGER NOT NULL CHECK(corpus_revision>=1),
    entitlement_revision INTEGER NOT NULL CHECK(entitlement_revision>=1),
    ranker_version TEXT NOT NULL CHECK(length(ranker_version) BETWEEN 1 AND 100),
    item_limit INTEGER NOT NULL CHECK(item_limit BETWEEN 1 AND 20),
    since_at INTEGER NOT NULL CHECK(since_at>=0),
    created_at INTEGER NOT NULL CHECK(created_at>=0),
    lease_until INTEGER NOT NULL CHECK(lease_until>created_at),
    retain_until INTEGER NOT NULL CHECK(retain_until>=created_at+86400000000),
    cache_ttl INTEGER NOT NULL CHECK(cache_ttl BETWEEN 1000000 AND 3600000000),
    work_state TEXT NOT NULL CHECK(work_state IN ('pending','done','failed')),
    spend_state TEXT NOT NULL CHECK(spend_state IN ('reserved','started','uncertain','settled')),
    reserved_micros INTEGER NOT NULL CHECK(reserved_micros>=0),
    actual_micros INTEGER CHECK(actual_micros>=0 AND actual_micros<=reserved_micros),
    snapshot_id TEXT,
    PRIMARY KEY(tenant_id,id),
    UNIQUE(tenant_id,key_hash),
    FOREIGN KEY(tenant_id,feed_id) REFERENCES feeds(tenant_id,id),
    FOREIGN KEY(tenant_id,snapshot_id) REFERENCES query_snapshots(tenant_id,id),
    CHECK((spend_state='settled')=(actual_micros IS NOT NULL)),
    CHECK((work_state='done')=(snapshot_id IS NOT NULL))
) STRICT;
CREATE INDEX query_work_expiry ON query_work(tenant_id,work_state,lease_until);
CREATE TABLE query_snapshots (
    tenant_id TEXT NOT NULL,
    id TEXT NOT NULL CHECK(length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'),
    feed_id TEXT NOT NULL,
    request_hash BLOB NOT NULL CHECK(length(request_hash)=32),
    feed_revision INTEGER NOT NULL,
    corpus_revision INTEGER NOT NULL,
    entitlement_revision INTEGER NOT NULL,
    ranker_version TEXT NOT NULL,
    mode TEXT NOT NULL CHECK(mode IN ('ai','deterministic_fallback')),
    generated_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL CHECK(expires_at>generated_at),
    retain_until INTEGER NOT NULL CHECK(retain_until>=expires_at),
    items TEXT NOT NULL CHECK(length(CAST(items AS BLOB))<=160000),
    PRIMARY KEY(tenant_id,id),
    FOREIGN KEY(tenant_id,feed_id) REFERENCES feeds(tenant_id,id)
) STRICT;
CREATE INDEX query_cache ON query_snapshots(tenant_id,feed_id,request_hash,feed_revision,corpus_revision,entitlement_revision,ranker_version,expires_at);

-- Revision updates share the corpus/membership transaction, including direct
-- future ingestion writes. Tenant-wide invalidation is conservative on the Pi.
-- +goose StatementBegin
CREATE TRIGGER query_article_insert AFTER INSERT ON articles BEGIN
 UPDATE tenants SET corpus_revision=corpus_revision+1 WHERE id=new.tenant_id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER query_article_update AFTER UPDATE ON articles BEGIN
 UPDATE tenants SET corpus_revision=corpus_revision+1 WHERE id=new.tenant_id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER query_article_delete AFTER DELETE ON articles BEGIN
 UPDATE tenants SET corpus_revision=corpus_revision+1 WHERE id=old.tenant_id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER query_membership_insert AFTER INSERT ON feed_sources BEGIN
 UPDATE feeds SET revision=revision+1 WHERE tenant_id=new.tenant_id AND id=new.feed_id;
 UPDATE tenants SET entitlement_revision=entitlement_revision+1 WHERE id=new.tenant_id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER query_membership_delete AFTER DELETE ON feed_sources BEGIN
 UPDATE feeds SET revision=revision+1 WHERE tenant_id=old.tenant_id AND id=old.feed_id;
 UPDATE tenants SET entitlement_revision=entitlement_revision+1 WHERE id=old.tenant_id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER query_feed_update AFTER UPDATE OF title,enabled ON feeds BEGIN
 UPDATE feeds SET revision=revision+1 WHERE tenant_id=new.tenant_id AND id=new.id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER query_source_update AFTER UPDATE OF title,url,enabled ON sources BEGIN
 UPDATE tenants SET entitlement_revision=entitlement_revision+1 WHERE id=new.tenant_id;
END;
-- +goose StatementEnd
