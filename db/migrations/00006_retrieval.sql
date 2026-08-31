-- +goose Up
ALTER TABLE feeds ADD COLUMN preferences TEXT NOT NULL DEFAULT '' CHECK(length(CAST(preferences AS BLOB))<=65536);
ALTER TABLE query_snapshots ADD COLUMN details TEXT NOT NULL DEFAULT '' CHECK(length(CAST(details AS BLOB))<=65536);
CREATE INDEX article_effective_age ON articles(tenant_id,source_id,coalesce(published_at,observed_at) DESC,id);
-- +goose StatementBegin
CREATE TRIGGER query_preferences_update AFTER UPDATE OF preferences ON feeds
WHEN new.preferences IS NOT old.preferences BEGIN
 UPDATE feeds SET revision=revision+1 WHERE tenant_id=new.tenant_id AND id=new.id;
END;
-- +goose StatementEnd
