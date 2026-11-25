-- Revert the merge-request write-back idempotency constraint.

BEGIN;

ALTER TABLE comments
    DROP CONSTRAINT IF EXISTS comments_fingerprint_mr_unique;

COMMIT;
