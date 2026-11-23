-- Milestone 1: per-project webhook secret.
-- Stores only a hash of the webhook token, never the plaintext. A NULL value
-- means no secret has been set yet and webhook delivery for the project is
-- rejected until one is.

BEGIN;

ALTER TABLE projects ADD COLUMN webhook_secret_hash text;

COMMIT;
