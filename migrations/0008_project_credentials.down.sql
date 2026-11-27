-- Reverts migration 0008: drops the per-project encrypted credential store.

BEGIN;

DROP TABLE IF EXISTS project_credentials;

COMMIT;
