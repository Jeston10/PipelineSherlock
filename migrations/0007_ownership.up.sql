-- Milestone 6 slice 2: per-caller ownership.
-- Associates every API key and every project with an owner (tenant) so a caller
-- only sees and mutates projects it owns. Existing rows default to '' (no owner),
-- which no authenticated key can match (keys require a non-empty owner), so any
-- legacy data stays private. This migration is reversible; see the down file.

BEGIN;

ALTER TABLE api_keys ADD COLUMN owner_id text NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN owner_id text NOT NULL DEFAULT '';

CREATE INDEX projects_owner_idx ON projects (owner_id);

COMMIT;
