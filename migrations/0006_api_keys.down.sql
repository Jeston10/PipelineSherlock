-- Revert the API authentication keys table.

BEGIN;

DROP TABLE IF EXISTS api_keys;

COMMIT;
