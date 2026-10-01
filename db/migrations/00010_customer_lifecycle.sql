-- +goose Up
ALTER TABLE customer_workspaces ADD COLUMN state TEXT NOT NULL DEFAULT 'active' CHECK(state IN ('active','suspended','deleted'));
-- Non-identifying charges survive customer erasure until the acquisition window expires.
CREATE TABLE erased_acquisition_usage(charged_at INTEGER NOT NULL, charged_bytes INTEGER NOT NULL CHECK(charged_bytes>=0)) STRICT;
