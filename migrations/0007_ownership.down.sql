-- Revert per-caller ownership.

BEGIN;

DROP INDEX IF EXISTS projects_owner_idx;
ALTER TABLE projects DROP COLUMN IF EXISTS owner_id;
ALTER TABLE api_keys DROP COLUMN IF EXISTS owner_id;

COMMIT;
