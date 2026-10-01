-- +goose Up
CREATE EXTENSION IF NOT EXISTS unaccent;
CREATE TEXT SEARCH CONFIGURATION northway_search (COPY = pg_catalog.simple);
ALTER TEXT SEARCH CONFIGURATION northway_search ALTER MAPPING FOR hword, hword_part, word WITH unaccent, simple;
-- +goose StatementBegin
CREATE FUNCTION northway_vector(value text) RETURNS tsvector LANGUAGE sql IMMUTABLE AS $$ SELECT to_tsvector('northway_search', value) $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION northway_match(value text) RETURNS tsquery LANGUAGE sql STABLE AS $$ SELECT to_tsquery('northway_search', replace(replace(replace(replace(replace(value, 'title : (', ''), '" OR "', ' | '), '" AND "', ' & '), '"', ''), ')', '')) $$;
-- +goose StatementEnd
ALTER TABLE articles ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (northway_vector(title || ' ' || body)) STORED;
ALTER TABLE articles ADD COLUMN title_vector tsvector GENERATED ALWAYS AS (northway_vector(title)) STORED;
CREATE INDEX article_title_search ON articles USING gin(title_vector);
CREATE INDEX article_search ON articles USING gin(search_vector);
