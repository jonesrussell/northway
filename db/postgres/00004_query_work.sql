-- +goose Up
ALTER TABLE tenants ADD COLUMN corpus_revision BIGINT NOT NULL DEFAULT 1 CHECK(corpus_revision>=1);
ALTER TABLE tenants ADD COLUMN entitlement_revision BIGINT NOT NULL DEFAULT 1 CHECK(entitlement_revision>=1);
ALTER TABLE feeds ADD COLUMN revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>=1);
ALTER TABLE feeds ADD COLUMN enabled BIGINT NOT NULL DEFAULT 1 CHECK(enabled IN (0,1));
ALTER TABLE sources ADD COLUMN enabled BIGINT NOT NULL DEFAULT 1 CHECK(enabled IN (0,1));

CREATE TABLE budgets (
    tenant_id TEXT PRIMARY KEY NOT NULL REFERENCES tenants(id),
    limit_micros BIGINT NOT NULL CHECK(limit_micros>=0),
    spent_micros BIGINT NOT NULL DEFAULT 0 CHECK(spent_micros>=0),
    held_micros BIGINT NOT NULL DEFAULT 0 CHECK(held_micros>=0),
    CHECK(spent_micros<=limit_micros AND held_micros<=limit_micros-spent_micros)
);
CREATE TABLE query_work (
    tenant_id TEXT NOT NULL,
    id TEXT NOT NULL CHECK(length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') !~ '[^0-9a-f]'),
    key_hash BYTEA NOT NULL CHECK(length(key_hash)=32),
    request_hash BYTEA NOT NULL CHECK(length(request_hash)=32),
    feed_id TEXT NOT NULL,
    feed_revision BIGINT NOT NULL CHECK(feed_revision>=1),
    corpus_revision BIGINT NOT NULL CHECK(corpus_revision>=1),
    entitlement_revision BIGINT NOT NULL CHECK(entitlement_revision>=1),
    ranker_version TEXT NOT NULL CHECK(length(ranker_version) BETWEEN 1 AND 100),
    item_limit BIGINT NOT NULL CHECK(item_limit BETWEEN 1 AND 20),
    since_at BIGINT NOT NULL CHECK(since_at>=0),
    created_at BIGINT NOT NULL CHECK(created_at>=0),
    lease_until BIGINT NOT NULL CHECK(lease_until>created_at),
    retain_until BIGINT NOT NULL CHECK(retain_until>=created_at+86400000000),
    cache_ttl BIGINT NOT NULL CHECK(cache_ttl BETWEEN 1000000 AND 3600000000),
    work_state TEXT NOT NULL CHECK(work_state IN ('pending','done','failed')),
    spend_state TEXT NOT NULL CHECK(spend_state IN ('reserved','started','uncertain','settled')),
    reserved_micros BIGINT NOT NULL CHECK(reserved_micros>=0),
    actual_micros BIGINT CHECK(actual_micros>=0 AND actual_micros<=reserved_micros),
    snapshot_id TEXT,
    PRIMARY KEY(tenant_id,id),
    UNIQUE(tenant_id,key_hash),
    FOREIGN KEY(tenant_id,feed_id) REFERENCES feeds(tenant_id,id),
    CHECK((spend_state='settled')=(actual_micros IS NOT NULL)),
    CHECK((work_state='done')=(snapshot_id IS NOT NULL))
);
CREATE INDEX query_work_expiry ON query_work(tenant_id,work_state,lease_until);
CREATE TABLE query_snapshots (
    tenant_id TEXT NOT NULL,
    id TEXT NOT NULL CHECK(length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') !~ '[^0-9a-f]'),
    feed_id TEXT NOT NULL,
    request_hash BYTEA NOT NULL CHECK(length(request_hash)=32),
    feed_revision BIGINT NOT NULL,
    corpus_revision BIGINT NOT NULL,
    entitlement_revision BIGINT NOT NULL,
    ranker_version TEXT NOT NULL,
    mode TEXT NOT NULL CHECK(mode IN ('ai','deterministic_fallback')),
    generated_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL CHECK(expires_at>generated_at),
    retain_until BIGINT NOT NULL CHECK(retain_until>=expires_at),
    items TEXT NOT NULL CHECK(octet_length(items)<=524288),
    PRIMARY KEY(tenant_id,id),
    FOREIGN KEY(tenant_id,feed_id) REFERENCES feeds(tenant_id,id)
);
ALTER TABLE query_work ADD FOREIGN KEY(tenant_id,snapshot_id) REFERENCES query_snapshots(tenant_id,id);
CREATE INDEX query_cache ON query_snapshots(tenant_id,feed_id,request_hash,feed_revision,corpus_revision,entitlement_revision,ranker_version,expires_at);

-- Revision updates share the corpus/membership transaction, including direct
-- future ingestion writes. Tenant-wide invalidation is conservative on the Pi.
-- +goose StatementBegin
CREATE FUNCTION query_article_insert_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
UPDATE tenants SET corpus_revision=corpus_revision+1 WHERE id=new.tenant_id;
RETURN NEW; END $$;
CREATE TRIGGER query_article_insert AFTER INSERT ON articles FOR EACH ROW EXECUTE FUNCTION query_article_insert_fn();
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION query_article_update_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
IF new.content_hash IS DISTINCT FROM old.content_hash OR new.url IS DISTINCT FROM old.url
 OR new.published_at IS DISTINCT FROM old.published_at OR new.observed_at IS DISTINCT FROM old.observed_at THEN
UPDATE tenants SET corpus_revision=corpus_revision+1 WHERE id=new.tenant_id;
END IF;
RETURN NEW; END $$;
CREATE TRIGGER query_article_update AFTER UPDATE ON articles FOR EACH ROW EXECUTE FUNCTION query_article_update_fn();
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION query_article_delete_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
UPDATE tenants SET corpus_revision=corpus_revision+1 WHERE id=old.tenant_id;
RETURN OLD; END $$;
CREATE TRIGGER query_article_delete AFTER DELETE ON articles FOR EACH ROW EXECUTE FUNCTION query_article_delete_fn();
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION query_membership_insert_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
UPDATE feeds SET revision=revision+1 WHERE tenant_id=new.tenant_id AND id=new.feed_id;
 UPDATE tenants SET entitlement_revision=entitlement_revision+1 WHERE id=new.tenant_id;
RETURN NEW; END $$;
CREATE TRIGGER query_membership_insert AFTER INSERT ON feed_sources FOR EACH ROW EXECUTE FUNCTION query_membership_insert_fn();
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION query_membership_delete_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
UPDATE feeds SET revision=revision+1 WHERE tenant_id=old.tenant_id AND id=old.feed_id;
 UPDATE tenants SET entitlement_revision=entitlement_revision+1 WHERE id=old.tenant_id;
RETURN OLD; END $$;
CREATE TRIGGER query_membership_delete AFTER DELETE ON feed_sources FOR EACH ROW EXECUTE FUNCTION query_membership_delete_fn();
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION query_feed_update_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
UPDATE feeds SET revision=revision+1 WHERE tenant_id=new.tenant_id AND id=new.id;
RETURN NEW; END $$;
CREATE TRIGGER query_feed_update AFTER UPDATE OF title,enabled ON feeds FOR EACH ROW EXECUTE FUNCTION query_feed_update_fn();
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION query_source_update_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
UPDATE tenants SET entitlement_revision=entitlement_revision+1 WHERE id=new.tenant_id;
RETURN NEW; END $$;
CREATE TRIGGER query_source_update AFTER UPDATE OF title,url,enabled ON sources FOR EACH ROW EXECUTE FUNCTION query_source_update_fn();
-- +goose StatementEnd
