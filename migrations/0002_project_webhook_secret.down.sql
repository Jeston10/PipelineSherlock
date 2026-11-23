-- Reverse of 0002_project_webhook_secret.up.sql.

BEGIN;

ALTER TABLE projects DROP COLUMN IF EXISTS webhook_secret_hash;

COMMIT;
