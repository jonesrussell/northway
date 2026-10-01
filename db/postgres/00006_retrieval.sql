-- +goose Up
ALTER TABLE feeds ADD COLUMN preferences TEXT NOT NULL DEFAULT '' CHECK(octet_length(preferences)<=65536);
ALTER TABLE query_snapshots ADD COLUMN details TEXT NOT NULL DEFAULT '' CHECK(octet_length(details)<=65536);
CREATE INDEX article_effective_age ON articles(tenant_id,source_id,coalesce(published_at,observed_at) DESC,id);
-- +goose StatementBegin
CREATE FUNCTION query_preferences_update_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
IF new.preferences IS DISTINCT FROM old.preferences THEN
UPDATE feeds SET revision=revision+1 WHERE tenant_id=new.tenant_id AND id=new.id;
END IF;
RETURN NEW; END $$;
CREATE TRIGGER query_preferences_update AFTER UPDATE OF preferences ON feeds FOR EACH ROW EXECUTE FUNCTION query_preferences_update_fn();
-- +goose StatementEnd
