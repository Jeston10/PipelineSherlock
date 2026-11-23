-- Reverts 0003: drop the processing attempt counter.

BEGIN;

ALTER TABLE webhook_events DROP COLUMN attempts;

COMMIT;
