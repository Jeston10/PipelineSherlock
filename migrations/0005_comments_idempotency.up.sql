-- Make merge-request write-back idempotent: at most one comment per
-- (fingerprint, merge request). The write-back path inserts with
-- ON CONFLICT DO NOTHING so a reprocessed event never posts a duplicate note.
-- Reversible; see 0005_comments_idempotency.down.sql.

BEGIN;

ALTER TABLE comments
    ADD CONSTRAINT comments_fingerprint_mr_unique UNIQUE (fingerprint_id, merge_request_iid);

COMMIT;
