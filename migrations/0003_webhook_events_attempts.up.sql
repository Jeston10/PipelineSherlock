-- Milestone 1 slice 2: processing attempt counter on the durable inbox.
-- The background worker increments this each time it tries to reconcile an
-- event so retries can be bounded and failures recorded.

BEGIN;

ALTER TABLE webhook_events ADD COLUMN attempts integer NOT NULL DEFAULT 0;

COMMIT;
